---
name: ai-meeting-memory
description: 当需求涉及 AI 或 Agent Run 上下文、压缩摘要、画像、Redis/MySQL 恢复、历史窗口、消息序列或分布式 SingleFlight 时使用。
---

# AI-Meeting Memory Skill

## 何时使用

- AI 长上下文拼接、压缩阈值、摘要恢复和清理。
- Agent Run 四层上下文、token 预算、压缩边界、画像候选/合并/确认。
- 消息 `sequence`、Redis 热数据或 SingleFlight 共享执行。
- AI 聊天入口同时读 AI Skill，Agent 编排同时读 Agent Skill，面试运行态恢复先读 Interview Skill。

旧 `AgentChatSSE` 不使用压缩；当前 Agent Run 使用压缩。两条链路不能互相推断。

## 代码地图

- AI：`services/ai/ai_memory_service.go`、`services/ai/ai_chat_service.go`。
- Agent：`services/agent/context/assembler.go`、`services/agent/context/tokens.go`、`services/agent/context/store.go`、`services/agent/context/compress.go`。
- 画像：`services/agent/context/profile_rules.go`、`services/agent/context/profile_consolidate.go`、`services/agent/context/profile_apply.go`。
- 后台生命周期：`services/agent/context/workers.go`；根编排：`services/agent/run/service.go`。
- 热数据：`repositories/redis_hot.go`；连接和全局 SingleFlight：`repositories/redis.go`。
- 持久化：`repositories/mysql/message_repository.go`、`repositories/mysql/sequence_repository.go`、`repositories/mysql/compressed_context_repository.go`。
- 模型：`models/conversation.go`、`models/agent_runtime.go`。
- 分布式去重：`pkg/singleflight/singleflight.go`。

## AI 压缩

1. Chat / ChatStream 先 `GetContext`，再保存 user、调用模型、保存 assistant，成功一轮后异步 `CompressContext`。
2. 摘要缓存为 `memory:ai:{sid}:summary` / `:index`，TTL 7 天；原文窗口查 MySQL `messages`。
3. 按格式化消息字节长度触发（默认 threshold=4096，触发偏移 500，可设范围 1024～32768），较旧的 80% 消息生成摘要，近期 20% 留原文；流式摘要失败回退本地截断。先写 Redis，再独立 goroutine upsert MySQL，并非同步持久化。
4. 持久快照为 `compressed_contexts`，实际 `session_id` 使用原始 sid，`memory_scope=ai`。旧 `ai:` 前缀 helper 未用于当前持久化路径。
5. 压缩调用 SingleFlight，key 为 `compress:ai:<sid>:<uid>`；模型 chunk 调 `writer.Write` 更新进度。
6. `/api/xunzhi/v1/ai/memory/threshold` 只调整当前进程内阈值，重启失效。

## Agent Run 上下文

- 装配顺序为 identity → active profile facts → summary → recent；各 system 段先独立截断，再给近期消息剩余预算。估算器为 CJK 1 token/字、其他字符约 4 字符/token、乘 1.15，不是精确 tokenizer；不能声称所有输入均满足严格总 token 上限。
- 摘要的 `Index` 即 covered sequence；近期只取 `sequence > Index`。Redis miss 的 MySQL 回填最多取 500 条，是恢复窗口限制。
- `PushRecentMessage` 不做 LTRIM；压缩成功后按保留 tail 重建近期列表，避免先裁掉尚未摘要覆盖的消息。
- 只压缩对话，按估算 token 触发；增量合并未覆盖的旧消息与已有摘要，保留最近约 20% 或至少 600 token。先写 MySQL 摘要，再写 Redis。
- 根装配丢弃近期消息时，run 服务会以 30s 预算同步尝试压缩并重新装配；结果记录到 `agent_runs.context_report_json`。
- 每个 user 消息异步捕获规则候选；L2 当前仅在“无候选且用户消息不足 3 条”时跳过，定义的候选阈值 5 未参与判断。合并输出 diff，正式画像 version CAS，确认与审计分别落库；已有确认接口和调试页。renderProfile 优先 active facts，无 active facts 时回退序列化旧画像 JSON。
- 候选、压缩、合并任务使用 goroutine + cancel/WaitGroup 管理，没有固定 worker/队列/并发上限。旧画像提取与 AI 压缩不在该管理范围。

## SingleFlight 实际边界

- Redis SET NX 租约选 leader，Pub/Sub 通知 follower，轮询检查进度；leader 流式 `writer.Write` 记录 `累计字节:时间戳`，停滞超过 30s 可触发接管。
- 取消信号用 Pub/Sub 与 cancel key 轮询传递；新 leader 初始占位进度减少首段输出前的误判窗口。
- Redis 故障可降级进程内合并，writer 为 no-op；非流式 Agent run 共享结果，流式 run 使用独立 Redis 幂等锁，不共享 chunk。
- 当前存在 follower 缓存命中状态与“继续等待”同值的问题；轮询命中可能被忽略。共享执行继承 leader 请求 context，断连可能影响其他等待者。
- 续租和写结果错误未完整处理，结果/进度写入缺少 ownership/fencing 保护；不响应 cancel 的执行、panic 清理及本地等待取消仍需治理。占位进度修复不等于故障接管强一致。
- Agent run 和画像合并未更新 writer，长任务可能被停滞检测误判。历史模拟压测不能证明这些业务路径可靠。

## 不变量与修改检查

- MySQL `messages` 是全量消息真相源；`session_sequences` 原子分配序列，禁止改回查询 max+1。
- 摘要与近期消息按 sequence 划界，并保留用户归属校验。
- `compressed_contexts` 唯一键仅为 session_id；`memory_scope` 不是联合唯一维度。`TotalLength` 在 AI 路径是字节数，在 Agent 路径是估算 token 数，不能混读。
- 先读 `docs/agent-knowledge/references/memory-context-flow.md`；并发和故障边界读 `docs/agent-knowledge/references/placeholder-risk-register.md`。
- 向量召回尚未实现，规划时读 `docs/agent-knowledge/references/ai-memory-evolution.md`。
- 改存储同步 `data-models.md`，改 HTTP 同步 `routes-map.md`；代码变更运行相应测试及 `scripts/knowledge-check.sh diff`，文档整理运行 `full`。
