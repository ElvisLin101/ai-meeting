# Memory Context Flow

## 目标

为 AI 会话与 Agent 会话提供**可压缩、可恢复、分层**的上下文。

存储架构（已移除 MySQL）：

- MySQL 是真相源：`messages` 全量消息、`compressed_contexts` 压缩快照、`user_profiles` 画像、`agent_identities` 身份。
- Redis 是热数据：近期对话、压缩摘要、画像、身份，全部带 TTL，丢失后从 MySQL 重建。

## AI 入口链路

1. `AiMessageController.Chat`
2. `AiMessageService.Chat/ChatStream`
3. `AiMemoryService.GetCompressionThreshold()`
4. `AiMemoryService.GetContext(ctx, sessionID, username, threshold)`
5. 保存用户消息到 MySQL `messages`（`session_sequences` 分配 sequence）
6. 调模型（AiID 配置或 `config.ai.deepseek`）
7. 保存 assistant 回复到 MySQL `messages`
8. 更新 `conversations.message_count`
9. `AiMemoryService.CompressContext` 异步判断压缩

## AI GetContext

1. Redis 读 `memory:ai:{sessionId}:summary` 与 `:index`。
2. miss 时从 MySQL `compressed_contexts`（`session_id`）恢复并异步回填 Redis。
3. 从 MySQL `messages` 查 `sequence > index` 的消息（`session_id` + `user_id`，倒序）。
4. `buildContextWithWindow` 按预算取窗口，写入时恢复正序。
5. `GetContext` 只拼装，不触发压缩。

## AI/Agent CompressContext

1. 异步 goroutine 从 MySQL 读取全部消息。
2. 分布式 SingleFlight（AI: `compress:ai:{sid}:{uid}`；Agent: `compress:agent:{sid}:{uid}`）防重复压缩。
3. 低于阈值直接返回。
4. 80/20 分割，只压旧 80%。
5. 流式调模型生成摘要；`writer.Write` 刷新 SingleFlight 心跳；失败回退本地截断。
6. 写 Redis 摘要 + MySQL `compressed_contexts` 快照。

## Agent 四层上下文

`services/agent/context/assembler.go` 拼装顺序：

1. L4 `system`：Agent 自我定位（`agent_identities`，Redis 优先）
2. L3 `system`：用户画像（`user_profiles`，Redis 优先）
3. L2 `system`：压缩摘要（Redis → MySQL）
4. L1 `user/assistant`：近期全量对话（Redis List → MySQL 回填）

预算不足时优先裁剪 L1；persona 与画像永不裁剪。

- 画像提炼：`services/agent/context/profile.go`，模型调用后异步 + 节流 + SingleFlight 去重；冲突时新值覆盖旧值、version 递增。
- Agent 消息写入：`services/agent/run/service.go` 先写 MySQL 再更新 Redis 热列表。

## 修改时必查

- `repositories/mysql/message_repository.go`
- `repositories/mysql/compressed_context_repository.go`
- `repositories/mysql/user_profile_repository.go`
- `repositories/mysql/agent_identity_repository.go`
- `repositories/redis_hot.go`：热数据 key、TTL、回填
- `services/ai/ai_memory_service.go`：AI 压缩与窗口
- `services/agent/context/`：四层上下文、画像合并、压缩
- `models/conversation.go` / `models/agent_runtime.go`：表结构

## 验证建议

- 构造同一 session 的消息，检查 `/ai/history/:sessionId` 与 `/agents/conversations/:sid/messages` 的顺序和 ID。
- 超阈值后检查 Redis summary/index 与 MySQL `compressed_contexts` 均写入。
- 清空 Redis 保留 MySQL，检查读路径能回填并恢复。
- 检查压缩后 `GetContext` 不重复包含已压缩消息。
- Agent：检查画像在跨会话可见、版本递增、冲突字段被新值覆盖。
