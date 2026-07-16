---
name: ai-meeting-memory
description: 当需求涉及 Agent 或 AI 长上下文、历史消息窗口、压缩摘要、Redis/Mongo 恢复、压缩阈值、分布式 SingleFlight 去重或 message sequence 时使用。
---

# AI-Meeting Memory Skill

## 何时使用

读取本 Skill 的场景:

- 修改 Agent 或 AI 历史上下文拼接、压缩、恢复或清理。
- 修改 `AgentMessage` 的写入顺序、查询顺序或 `sequence` 规则。
- 修改 `AiMessage` 的写入顺序、查询顺序或 `sequence` 规则。
- 接入真实 AI 摘要模型、调整压缩阈值、Redis key 或 Mongo 持久化。
- 调查上下文重复、丢失、顺序错乱或压缩不触发。
- 修改分布式 SingleFlight 去重逻辑。

不适用场景:

- AI 普通会话入口、模型调用和响应 DTO 先读 `ai-meeting-ai`。
- 面试历史接口虽然复用 `AgentMessageHistoryRespDTO`, 但业务入口先读 `ai-meeting-interview`。

## 代码地图

- Agent 记忆服务: `services/common/memory_service.go`（MemoryService + 压缩常量）。
- AI 记忆服务: `services/ai/ai_memory_service.go`（AiMemoryService）。
- 分布式 SingleFlight: `pkg/singleflight/singleflight.go`（DistributedGroup）。
- SingleFlight 初始化: `repositories/redis.go`（全局 `SingleFlight` 实例）。
- Agent 消息仓储: `repositories/mongo/agent_message_repository.go`。
- AI 消息仓储: `repositories/mongo/ai_message_repository.go`。
- 压缩上下文模型: `models/compressed_context.go`。
- AI 聊天中触发压缩: `services/ai/ai_chat_service.go`（finishAiChat 中调用 CompressContext）。
- Agent 聊天中触发压缩: `services/agent/agent_service.go`（AgentChatSSE 中调用 CompressContext）。

## 核心流程

`AgentHistoryWithContext`

- 入口: `services/agent/agent_service.go` 中 `AgentMessageService.GetConversationHistoryWithContext`。
- 流程: 取 `MemoryService` 单例, 读阈值, 调用 `GetContext`。
- Agent Chat SSE 已接入: `AgentChatSSE` 在保存 assistant 回复后异步触发 `MemoryService.CompressContext`。

`MemoryGetContext`

- 入口: `services/common/memory_service.go` 中 `MemoryService.GetContext`。
- Redis key: 压缩摘要用 `sessionId`, 压缩索引用 `sessionId + "index"`。
- Redis 摘要或索引未命中时从 Mongo 集合 `compressed_contexts` 恢复, 该集合以 `sessionId` 作为 `_id`, 然后异步同步回 Redis。
- 原始消息从 Mongo 集合 `agent_messages` 按 `session_id` 和 `user_id` 查询, 排序为 `sequence DESC`。
- 组装由 `buildContextWithWindow` 完成: Mongo 倒序取窗口, 写入上下文时恢复为正序。
- 如果 `index` 之后的上下文长度达到 `threshold - 500`, 会异步触发压缩。

`MemoryCompress`（已接入分布式 SingleFlight）

- 入口: `MemoryService.CompressContext`。
- 使用分布式 SingleFlight 去重: `repositories.SingleFlight.Do(ctx, "compress:agent:"+sessionID+":"+userID, fn)`。
  - 全集群同一 session 只压缩一次, 其他实例等结果复用。
  - Redis 不可用时自动降级为本地 singleflight（`localGroup`）。
- 异步 goroutine 从 Mongo 查询全部 `AgentMessage`, 仍按 `sequence DESC`。
- 总长度低于 `threshold - COMPRESSION_TRIGGER_OFFSET` 时不压缩。
- 当前策略用 `COMPRESSION_RATIO = 0.8` 选择旧消息压缩, 最新 20% 通过 `index` 后的 Mongo 消息补齐。
- 摘要和 index 写 Redis, 再异步 upsert 到 MongoDB。
- 压缩 AI 请求复用 `clients/ai_model_client.go`; `ai.deepseek.api_key` 已配置时默认走 DeepSeek, 否则使用第一个启用的 `AiProperties`, 未配置或失败时使用本地截断摘要兜底。

`AiMemory`

- 入口: `services/ai/ai_chat_service.go` 中 `AiMessageService.Chat` / `ChatStream`。
- 聊天链路先调用 `AiMemoryService.GetContext`, 再保存用户消息、调用模型、保存 assistant 回复, 最后触发 `AiMemoryService.CompressContext`。
- Redis key 使用命名空间, 摘要为 `memory:ai:{sessionId}:summary`, 索引为 `memory:ai:{sessionId}:index`。
- MongoDB `compressed_contexts` 中 AI 压缩快照 `_id=ai:{sessionId}`, 与 Agent 的 `_id=sessionId` 隔离。
- AI memory 压缩同样接入分布式 SingleFlight: key 为 `"compress:ai:"+sessionID+":"+userID`。
- AI memory 有独立阈值接口: `/api/xunzhi/v1/ai/memory/threshold`。

`分布式 SingleFlight 机制`

- 代码: `pkg/singleflight/singleflight.go`。
- 初始化: `repositories/redis.go` 中 `InitRedis` 创建全局 `SingleFlight *singleflight.DistributedGroup`。
- 核心方法: `Do(ctx, key, fn)` — 相同 key 只执行一次 fn, 其余等结果。
- 主节点: SET NX 抢锁 → 心跳续期 → 执行 fn → 写结果 → Pub/Sub 通知 → 释放锁。
- 从节点: 订阅 channel → 轮询检查主节点健康 → 收到通知读结果。
- AI 流式输出作心跳: 主节点每次写入进度到 Redis, 从节点检测停滞超 30s 则换主。
- 换主时写 cancelKey 通知旧主停止, 旧主 cancel context 自动断开 AI 调用。
- Redis 故障降级为本地 singleflight（`localGroup`）。

## 关键不变量

- 同一会话的 `sequence` 必须单调递增。
- 上下文压缩只能基于当前用户自己的消息。
- MongoDB `agent_messages` 是 Agent 消息永久记忆真相源。
- MongoDB `ai_messages` 是普通 AI 会话消息永久记忆真相源。
- Redis 是热缓存, MongoDB `compressed_contexts` 是压缩上下文持久恢复层。
- Redis 中只存压缩摘要和 index, 当前 Redis TTL 是 7 天。
- 如果改压缩索引规则, 必须检查是否会把已压缩消息重复拼回上下文。
- Agent 压缩快照必须以 `sessionId` 为 `_id` 覆盖写入。
- AI 压缩快照必须以 `ai:{sessionId}` 为 `_id` 覆盖写入。
- Agent 压缩 SingleFlight key 为 `"compress:agent:"+sessionID+":"+userID`。
- AI 压缩 SingleFlight key 为 `"compress:ai:"+sessionID+":"+userID`。

## 当前风险

- `SetCompressionThreshold` 是运行时内存配置, 服务重启后恢复默认值。
- AI 压缩请求默认按 OpenAI-compatible 响应解析, 当前 config fallback 是 DeepSeek; 如果接入非兼容 provider 需要改 `clients/ai_model_client.go`。
- AI 和 Agent 现在有独立 memory service, 后续修改不要混用 Redis key 或 Mongo `_id`。
- SingleFlight 的 sync.Map 防重复已替换为分布式版本, 但 `localGroup` 降级模式下仍为单进程。

## 修改指南

1. 先读 `docs/agent-knowledge/references/memory-context-flow.md`。
2. 改 `AgentMessage.Sequence` 相关逻辑时, 同时检查 `repositories/mongo/agent_message_repository.go` 和 `services/common/memory_service.go`。
3. 改 `AiMessage.Sequence` 相关逻辑时, 同时检查 `repositories/mongo/ai_message_repository.go` 和 `services/ai/ai_memory_service.go`。
4. 接入真实摘要模型时, 不要在压缩服务中硬编码 provider, 优先复用 `clients/ai_model_client.go` 和 `config.ai.deepseek` 或新增清晰的 client 边界。
5. 改 Redis key 或 Mongo 字段时, 同步更新 `models/compressed_context.go` 和 `data-models.md`。
6. 修改 SingleFlight 逻辑时, 同步检查 `pkg/singleflight/singleflight.go` 和 `repositories/redis.go`。
7. 完成后运行 `go build ./...` 和 `scripts/knowledge-check.sh diff`。
