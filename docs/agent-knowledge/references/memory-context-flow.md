# Memory Context Flow

## 目标

为 Agent 和普通 AI 会话提供可压缩的历史上下文。

- Agent 原始消息永久存储在 MongoDB `agent_messages`, 短期压缩摘要在 Redis, 压缩快照持久化到 MongoDB `compressed_contexts`。
- AI 原始消息永久存储在 MongoDB `ai_messages`, 短期压缩摘要使用独立 Redis key, 压缩快照同样持久化到 MongoDB `compressed_contexts`。

## Agent 入口链路

1. `AgentMessageService.GetConversationHistoryWithContext(sessionID, userID)`
2. `MemoryService.GetCompressionThreshold()`
3. `MemoryService.GetContext(sessionID, userID, threshold)`
4. `MemoryService.GetContext` 判断 `index` 之后的上下文长度是否达到 `threshold - 500`, 达到则异步触发 `CompressContext`

当前 `AgentController.Chat` 会保存用户消息, 然后异步调用 `GetConversationHistoryWithContext` 触发上下文窗口和压缩判断；但它尚未调用模型回复链路。

## Agent GetContext 详细步骤

1. 从 Redis 读取压缩摘要 key=`sessionId` 和压缩索引 key=`sessionId + "index"`。
2. Redis 缺少摘要或索引时, 从 MongoDB `compressed_contexts` 按 `_id=sessionId` 恢复。
3. MongoDB 命中后异步同步 Redis。
4. 从 MongoDB `agent_messages` 查询 `sequence > index` 的原始消息, 条件是 `session_id` 和 `user_id`, 排序 `sequence DESC`。
5. 调用 `buildContextWithWindow` 采用倒序窗口取消息, 写入上下文时恢复正序。
6. 如果可用上下文长度达到 `threshold - 500`, 异步触发压缩。

## Agent CompressContext 详细步骤

1. 异步 goroutine 从 MongoDB 查询当前会话和用户的全部 Agent 消息。
2. 计算消息内容长度, 低于阈值时直接返回。
3. 按 80%/20% 分割旧消息和最近消息, 只压缩旧的 80%。
4. 旧消息转成正序文本并调用 `callAIForCompression`。
5. 将摘要写入 Redis key=`sessionId`, 将索引写入 Redis key=`sessionId + "index"`。
6. 异步 upsert MongoDB `compressed_contexts`, 使用 `_id=sessionId` 覆盖写入。

## AI 入口链路

1. `AiMessageController.Chat`
2. `AiMessageService.Chat(ctx, sessionID, username, content)` 或 `AiMessageService.ChatStream(ctx, sessionID, username, content, onChunk)`
3. `AiMemoryService.GetCompressionThreshold()`
4. `AiMemoryService.GetContext(ctx, sessionID, username, threshold)`
5. 保存用户消息到 MongoDB `ai_messages`
6. 通过 OpenAI-compatible endpoint 调用模型: 会话绑定有效 `AiID` 时使用 MySQL `AiProperties`, 否则优先使用 `config.ai.deepseek`
7. 保存 assistant 回复到 MongoDB `ai_messages`
8. 更新 `ai_conversations.message_cnt` 和 `updated_at`
9. 调用 `AiMemoryService.CompressContext(sessionID, username, threshold)` 异步判断是否压缩

AI Chat 同时支持普通 JSON 响应和 SSE 流式响应; 两条链路共用同一套 memory、消息落库和压缩逻辑。

## AI GetContext 详细步骤

1. 从 Redis 读取压缩摘要 key=`memory:ai:{sessionId}:summary` 和压缩索引 key=`memory:ai:{sessionId}:index`。
2. Redis 缺少摘要或索引时, 从 MongoDB `compressed_contexts` 按 `_id=ai:{sessionId}` 恢复。
3. MongoDB 命中后异步同步 Redis。
4. 从 MongoDB `ai_messages` 查询 `sequence > index` 的原始消息, 条件是 `session_id` 和 `user_id`, 排序 `sequence DESC`。
5. `AiMemoryService.buildContextWithWindow` 采用倒序窗口取消息, 写入上下文时恢复正序。
6. `GetContext` 只拼接上下文, 不触发压缩；压缩在 assistant 回复保存后触发, 避免只压缩半轮对话。

## AI CompressContext 详细步骤

1. 异步 goroutine 从 MongoDB 查询当前会话和用户的全部 AI 消息。
2. 用 `sync.Map` 防止同一 `sessionID:userID` 在当前进程内重复压缩。
3. 计算消息内容长度, 低于阈值时直接返回。
4. 按 80%/20% 分割旧消息和最近消息, 只压缩旧的 80%。
5. 旧消息转成正序文本并通过 `clients/ai_model_client.go` 调用 OpenAI-compatible 模型压缩; `config.ai.deepseek` 配好 key 时默认走 DeepSeek。
6. 将摘要写入 Redis key=`memory:ai:{sessionId}:summary`, 将索引写入 Redis key=`memory:ai:{sessionId}:index`。
7. 异步 upsert MongoDB `compressed_contexts`, 使用 `_id=ai:{sessionId}` 覆盖写入。

## 修改时必查

- `AgentMessageService.SaveMessage`: 新消息 sequence 分配。
- `repositories/mongo/agent_message_repository.go`: Agent Mongo 消息查询、分页、保存。
- `MemoryService.buildContextWithWindow`: 窗口选取和写入顺序。
- `MemoryService.CompressContext`: 压缩分割和 `compressIndex`。
- `repositories/mongo/ai_message_repository.go`: AI Mongo 消息查询、分页、保存。
- `repositories/mongo/compressed_context_repository.go`: 压缩快照 Mongo upsert、恢复和删除。
- `services/ai_memory_service.go`: AI Redis key、Mongo `_id`、窗口选取和压缩分割。
- `services/ai_chat_service.go`: AI chat 何时读取上下文、保存消息、触发压缩。
- `models.CompressedContext`: Mongo 字段。
- `repositories.InitRedis` 和 `repositories/mongo.InitMongoDB`: 客户端可用性。

## 验证建议

- 构造同一 Agent session 的 5 条消息, 检查 `sequence ASC` 历史接口顺序。
- 构造同一 AI session 的用户/assistant 消息, 检查 `/ai/history/:sessionId` 顺序和 `message_id`。
- 构造超过阈值的 AI 消息, 检查 Redis `memory:ai:{sessionId}:summary` 和 `memory:ai:{sessionId}:index` 都被写入。
- 清空 Redis 保留 MongoDB, 检查 AI `GetContext` 是否能从 `_id=ai:{sessionId}` 恢复并回填 Redis。
- 检查压缩后 `GetContext` 不重复包含已经压缩的消息。
