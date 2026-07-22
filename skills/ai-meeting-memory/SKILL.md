---
name: ai-meeting-memory
description: 当需求涉及 AI 长上下文、历史消息窗口、压缩摘要、Redis/Mongo 恢复、压缩阈值、分布式 SingleFlight 去重或 message sequence 时使用。
---

# AI-Meeting Memory Skill

## 何时使用

读取本 Skill 的场景:

- 修改 AI 历史上下文拼接、压缩、恢复或清理。
- 修改 `AiMessage` 的写入顺序、查询顺序或 `sequence` 规则。
- 接入真实 AI 摘要模型、调整压缩阈值、Redis key 或 Mongo 持久化。
- 调查上下文重复、丢失、顺序错乱或压缩不触发。
- 修改分布式 SingleFlight 去重逻辑。

不适用场景:

- AI 普通会话入口、模型调用和响应 DTO 先读 `ai-meeting-ai`。
- Agent 侧不使用压缩记忆, Agent 相关需求读 `ai-meeting-agent`。
- 面试历史接口虽然复用 `AgentMessageHistoryRespDTO`, 但业务入口先读 `ai-meeting-interview`。

## 代码地图

- AI 记忆服务: `services/ai/ai_memory_service.go`（AiMemoryService + 压缩常量）。
- 分布式 SingleFlight: `pkg/singleflight/singleflight.go`（DistributedGroup）。
- SingleFlight 初始化: `repositories/redis.go`（全局 `SingleFlight` 实例）。
- Agent 消息仓储: `repositories/mongo/agent_message_repository.go`（Agent 侧不压缩, 但消息存储仍用）。
- AI 消息仓储: `repositories/mongo/ai_message_repository.go`。
- 压缩上下文模型: `models/compressed_context.go`。
- AI 聊天中触发压缩: `services/ai/ai_chat_service.go`（finishAiChat 中调用 CompressContext）。

## 核心流程

`AiMemory`

- 入口: `services/ai/ai_chat_service.go` 中 `AiMessageService.Chat` / `ChatStream`。
- 聊天链路先调用 `AiMemoryService.GetContext`, 再保存用户消息、调用模型、保存 assistant 回复, 最后触发 `AiMemoryService.CompressContext`。
- Redis key 使用命名空间, 摘要为 `memory:ai:{sessionId}:summary`, 索引为 `memory:ai:{sessionId}:index`。
- MongoDB `compressed_contexts` 中 AI 压缩快照 `_id=ai:{sessionId}`。
- AI memory 压缩接入分布式 SingleFlight: key 为 `"compress:ai:"+sessionID+":"+userID`。
- AI memory 有独立阈值接口: `/api/xunzhi/v1/ai/memory/threshold`。

`分布式 SingleFlight 机制`

- 代码: `pkg/singleflight/singleflight.go`。
- 初始化: `repositories/redis.go` 中 `InitRedis` 创建全局 `SingleFlight *singleflight.DistributedGroup`。
- 核心方法: `Do(ctx, key, fn)` — 相同 key 只执行一次 fn, 其余等结果。
- 主节点: SET NX 抢锁 → 心跳续期 → 执行 fn → 写结果 → Pub/Sub 通知 → 释放锁。
- 从节点: 订阅 channel → 轮询检查主节点健康 → 收到通知读结果。
- AI 流式输出作心跳: 主节点压缩时走 `CallConfiguredAIChatStream`, 在 `onChunk` 中调 `writer.Write` 刷新 `progressKey`(`累计字节数:时间戳`); 从节点检测 `progressKey` 停滞超 30s 则换主。`streamKey` 已移除, follower 仅靠 `progressKey` 判停滞, 不消费流内容。
- 换主时写 cancelKey 通知旧主停止, 旧主 cancel context 自动断开 AI 调用。
- Redis 故障降级为本地 singleflight（`localGroup`）, 此时 `writer.redis` 为 nil, `Write` 直接 no-op。

## 关键不变量

- 同一会话的 `sequence` 必须单调递增。
- 上下文压缩只能基于当前用户自己的消息。
- MongoDB `ai_messages` 是普通 AI 会话消息永久记忆真相源。
- Redis 是热缓存, MongoDB `compressed_contexts` 是压缩上下文持久恢复层。
- Redis 中只存压缩摘要和 index, 当前 Redis TTL 是 7 天。
- 如果改压缩索引规则, 必须检查是否会把已压缩消息重复拼回上下文。
- AI 压缩快照必须以 `ai:{sessionId}` 为 `_id` 覆盖写入。
- AI 压缩 SingleFlight key 为 `"compress:ai:"+sessionID+":"+userID`。
- 记忆/压缩能力仅属 AI 侧; Agent 侧不压缩, 未来上下文由状态机结构化状态管理。

## 当前风险

- `SetCompressionThreshold` 是运行时内存配置, 服务重启后恢复默认值。
- AI 压缩请求默认按 OpenAI-compatible SSE 流式响应解析（`CallConfiguredAIChatStream`）, 当前 config fallback 是 DeepSeek; 如果接入非兼容 provider 需要改 `clients/ai_model_client.go`。流式失败时回退本地截断 `fallbackAiCompressedSummary`。
- SingleFlight 的 sync.Map 防重复已替换为分布式版本, 但 `localGroup` 降级模式下仍为单进程。
- 当前记忆仅支持有损压缩摘要, 不支持向量召回, 早期消息的具体细节被压缩后无法精准检索回原文。后续演进方案见 `docs/agent-knowledge/references/ai-memory-evolution.md`。
- 压缩阈值用字节长度（`len()`）而非 token 数, MongoDB 字段名 `total_token_count` 实际存字节数, 中文场景下与真实 token 数偏差较大。

## 修改指南

1. 先读 `docs/agent-knowledge/references/memory-context-flow.md`。
2. 改 `AiMessage.Sequence` 相关逻辑时, 同时检查 `repositories/mongo/ai_message_repository.go` 和 `services/ai/ai_memory_service.go`。
3. 接入真实摘要模型时, 不要在压缩服务中硬编码 provider, 优先复用 `clients/ai_model_client.go` 和 `config.ai.deepseek` 或新增清晰的 client 边界。
4. 改 Redis key 或 Mongo 字段时, 同步更新 `models/compressed_context.go` 和 `data-models.md`。
5. 修改 SingleFlight 逻辑时, 同步检查 `pkg/singleflight/singleflight.go` 和 `repositories/redis.go`。
6. 规划向量召回等记忆演进时, 先读 `docs/agent-knowledge/references/ai-memory-evolution.md`。
7. 完成后运行 `go build ./...` 和 `scripts/knowledge-check.sh diff`。
