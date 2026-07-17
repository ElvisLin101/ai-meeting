# Memory Context Flow

## 目标

为普通 AI 会话提供可压缩的历史上下文。

- AI 原始消息永久存储在 MongoDB `ai_messages`, 短期压缩摘要使用独立 Redis key, 压缩快照同样持久化到 MongoDB `compressed_contexts`。
- Agent 侧不使用压缩记忆, 不在本流程范围内。Agent 侧上下文未来由面试工作流状态机结构化状态管理。

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
2. 使用分布式 SingleFlight（key=`compress:ai:{sessionID}:{userID}`）防止全集群同一会话重复压缩, Redis 不可用时降级为本地 `localGroup`。
3. 计算消息内容长度, 低于阈值时直接返回。
4. 按 80%/20% 分割旧消息和最近消息, 只压缩旧的 80%。
5. 旧消息转成正序文本并通过 `CallConfiguredAIChatStream` 流式调用 OpenAI-compatible 模型压缩; `onChunk` 中累积完整结果并调 `writer.Write` 刷新 `progressKey` 心跳, `config.ai.deepseek` 配好 key 时默认走 DeepSeek; 流式失败回退 `fallbackAiCompressedSummary` 本地截断。
6. 将摘要写入 Redis key=`memory:ai:{sessionId}:summary`, 将索引写入 Redis key=`memory:ai:{sessionId}:index`。
7. 异步 upsert MongoDB `compressed_contexts`, 使用 `_id=ai:{sessionId}` 覆盖写入。

## 修改时必查

- `repositories/mongo/ai_message_repository.go`: AI Mongo 消息查询、分页、保存。
- `repositories/mongo/compressed_context_repository.go`: 压缩快照 Mongo upsert、恢复和删除。
- `services/ai/ai_memory_service.go`: AI Redis key、Mongo `_id`、窗口选取和压缩分割。
- `services/ai/ai_chat_service.go`: AI chat 何时读取上下文、保存消息、触发压缩。
- `models.CompressedContext`: Mongo 字段。
- `repositories.InitRedis` 和 `repositories/mongo.InitMongoDB`: 客户端可用性。

## 验证建议

- 构造同一 AI session 的用户/assistant 消息, 检查 `/ai/history/:sessionId` 顺序和 `message_id`。
- 构造超过阈值的 AI 消息, 检查 Redis `memory:ai:{sessionId}:summary` 和 `memory:ai:{sessionId}:index` 都被写入。
- 清空 Redis 保留 MongoDB, 检查 AI `GetContext` 是否能从 `_id=ai:{sessionId}` 恢复并回填 Redis。
- 检查压缩后 `GetContext` 不重复包含已经压缩的消息。
