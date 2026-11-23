# Memory Context Flow

代码核对日期：2026-11-23。普通 AI、旧 Agent Chat、Agent Run 是不同链路；面试运行态见 `interview-runtime-governance.md`。

## 存储与边界

MySQL `conversations` / `messages` 统一保存会话和全量消息，scene 区分 ai/agent/interview。消息 sequence 由 `repositories/mysql/sequence_repository.go` 的 `session_sequences` 原子分配。

`models/agent_runtime.go` 的 `CompressedContextRecord` 保存摘要、`Index`、`TotalLength`、`MemoryScope`。唯一键仅 session_id。AI 和 Agent 当前都用原始 session_id，AI 设置 memory_scope=ai；旧 `ai:` 前缀 helper 不在持久化调用路径。`TotalLength` 在 AI 路径存字节数，在 Agent 路径存估算 token 数。

摘要覆盖 sequence ≤ Index，原文窗口取 sequence > Index。改变索引含义时必须一起核对压缩输入、缓存重建、MySQL 恢复和装配，避免摘要重复或未覆盖消息丢失。

## 普通 AI

代码：`services/ai/ai_chat_service.go`、`services/ai/ai_memory_service.go`。

1. 校验会话归属，读取摘要和近期原文窗口。
2. 保存用户消息，调用 configured AI client，保存完整 assistant 回复并更新会话计数。
3. 成功一轮后独立 goroutine 触发压缩；阈值按 `len(content)` 字节长度判断。
4. 较旧的 80% 消息生成摘要，近期 20% 保留原文。压缩调用 OpenAI-compatible SSE，chunk 更新 SingleFlight writer；失败回退本地截断摘要。
5. 先写 Redis 的 `memory:ai:{sid}:summary` / `:index`（TTL 7 天），再独立 goroutine upsert MySQL；缓存 miss 从持久快照恢复并异步回填 Redis。Redis 成功到 MySQL 落库之间存在失败窗口。

阈值接口只改当前实例内存配置。SSE 中断时 user 已保存，未完整的 assistant 不会保存。模型摘要有损，不等于原文精准检索。

## Agent Run

代码：`services/agent/run/service.go`、`services/agent/context/assembler.go`、`services/agent/context/store.go`、`services/agent/context/compress.go`。

1. 用户消息先保存 MySQL，再更新 Redis 近期列表，异步捕获画像规则候选。
2. 加载 identity、active profile、summary 及未覆盖近期消息；Redis miss 从 MySQL 回填，近期回填最多 500 条。
3. 分别按各段上限截断 system 内容，再分配 recent 剩余预算；近期消息从新往旧选择，最新单条过大才截断。估算用 CJK 1/字、其他字符约 1/4、乘 1.15。
4. 根装配发生近期消息丢弃时，以 30s 预算同步尝试 preflight 压缩，再装配。根报告写入 `agent_runs.context_report_json`。
5. ReAct / Plan 执行后保存 assistant，异步判断压缩及画像合并。
6. 压缩只处理未覆盖旧消息并合并已有摘要，按 token 触发，保留约 20% 或至少 600 token 的 tail；先更新 MySQL 快照，再写 Redis 摘要并重建 recent。

热 key：`agent:session:{sid}:recent` / `:summary` / `:summary_idx`，画像 `user:{uid}:profile`，身份 `agent:{aid}:identity`（`repositories/redis_hot.go`）。Push 不再 LTRIM，只有摘要推进边界后才重建 tail。

各 system 段独立上限与 recent 剩余预算不构成包含工具 schema、计划和所有输入的严格总预算保证。旧 `AgentChatSSE` 直接加载历史调用流式模型，不走本节装配/压缩。

## 画像合并

`services/agent/context/profile_rules.go` 只写规则候选，不调 LLM。`services/agent/context/profile_consolidate.go` 输入当前画像、pending 候选、近期对话；当前仅在无候选且 user 消息不足 3 条时退出，候选阈值 5 未使用。

`services/agent/context/profile_apply.go` 应用 add/update/supersede/ask_confirm，画像 version CAS；确认与审计分别写 profile_confirmations/profile_events。上下文优先注入 active facts，无 active facts 时回退旧画像 JSON；调试页已能确认或拒绝候选确认项。

用户级 `LastConsolidatedSeq` 与会话级 sequence 的跨会话比较仍需治理。候选、压缩、合并由 `services/agent/context/workers.go` 管理取消和等待，每任务 goroutine，无固定 worker 和并发上限；AI 压缩、旧 ExtractAsync 不在此范围。

## 共享执行限制

AI 压缩写 SingleFlight 进度；Agent 非流式 run 与画像合并未使用 writer，超过 30s 无进度可能被误判。流式 Agent run 用独立幂等锁，不共享 chunk。

`pkg/singleflight/singleflight.go` 的 follower 缓存命中状态、请求 context 继承、续租失败、结果写入 ownership、panic 清理仍有缺口，详见 `placeholder-risk-register.md`。初始占位进度只减少首段输出前的误判，不能证明旧 leader 已隔离。

## 验证重点

- 压缩前后摘要与 tail 无重复且不丢未覆盖原文，Redis miss 与热路径边界一致。
- 明确 AI 字节阈值与 Agent token 阈值；验证超长单条、system 段总量和 500 条恢复窗口。
- 故障测试需区分 leader/follower 结果，模拟不响应取消的旧 leader，检查写入所有权、续租失败及请求断连。
- 代码修改后跑相应测试和 `scripts/knowledge-check.sh diff`；锚点检查通过不代表上述语义已验证。
