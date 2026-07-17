# Data Models Map

## User

`models/user.go`

- `User` -> `users`
- 关键字段: `ID`, `Username`, `Password`, `Email`, `Phone`, `IsAdmin`, `Status`, `CreatedAt`, `UpdatedAt`
- 注意: 当前密码字段为明文语义, 没有哈希封装。

## Agent

`models/agent.go`

- `AgentProperties` -> `agent_properties`
  - 关键字段: `ID`, `Name`, `Description`, `Config`, `IsEnabled`, `ApiKey`, `ApiSecret`, `ApiFlowId`, `CreatedAt`, `UpdatedAt`
  - `ApiKey`/`ApiSecret`/`ApiFlowId` 用于讯飞星辰工作流鉴权和指定工作流。
  - 启动时全量加载到 `AgentPropertiesLoader` 的 sync.Map 缓存。
- `AgentConversation` -> `agent_conversations`
- `AgentMessage` -> MongoDB `agent_messages`
  - 关键字段含 `ResponseTime`(int64, 毫秒, assistant 消息专用) 和 `ErrorMessage`(string, 出错时记录)
- `AgentFileAsset` -> `agent_file_assets`

关键关系:

- `AgentConversation.SessionID` 是唯一业务会话 ID。
- `AgentMessage.MongoID` 映射 Mongo `_id`, HTTP 响应通过 `message_id` 暴露。
- `AgentMessage.SessionID` + `AgentMessage.UserID` 用于历史查询。Agent 侧不压缩。
- `AgentMessage.Sequence` 用于会话内消息顺序。

## AI

`models/ai.go`

- `AiProperties` -> `ai_properties`
- `AiConversation` -> `ai_conversations`
- `AiMessage` -> MongoDB `ai_messages`

关键关系:

- `AiConversation.UserID` 当前存 JWT `username`。
- `AiMessage.MongoID` 映射 Mongo `_id`, HTTP 响应通过 `message_id` 暴露。
- `AiMessage.SessionID` + `AiMessage.UserID` 用于历史查询和 AI memory 压缩。
- `AiMessage.Sequence` 用于消息顺序。
- `AiMessage` 的 GORM 字段保留兼容旧表结构, 当前 AI 消息读写走 MongoDB。
- `AiProperties.ApiKey` 和 `AiProperties.ApiSecret` 当前是普通字符串字段。
- 默认 DeepSeek 模型配置不建表, 读取 `config.AppConfig.AI.DeepSeek`; `ai.deepseek.api_key` 留空时不会启用 config fallback。

## Interview

`models/interview.go`

- `InterviewSession` -> `interview_sessions`
- `InterviewRecord` -> `interview_records`

当前注意点:

- `InterviewSessionFacade.PageConversations` 查询 `AgentConversation`, 不查询 `InterviewSession`。
- `InterviewSessionFacade.GetConversationHistory` 查询 `AgentMessage`, 不查询面试专属消息表。

## Compressed Context

`models/compressed_context.go`

- Mongo 集合: `compressed_contexts`
- AI 压缩快照 `_id` 固定为 `ai:{sessionId}`, 用于覆盖写入同一 AI 会话的最新压缩快照。
- 关键字段: `_id`, `session_id`, `memory_scope`, `compressed_content`, `index`, `total_token_count`, `message_count`, `created_at`, `updated_at`
- AI 压缩快照额外写入 `memory_scope=ai`, 用于排查共享集合中的来源。
- AI Redis key: 压缩摘要为 `memory:ai:{sessionId}:summary`, 压缩索引为 `memory:ai:{sessionId}:index`。
- 仅 AI 侧使用压缩; Agent 侧不写压缩快照、不占 Redis 压缩 key。

## Repository Clients

- MySQL: `repositories/mysql.DB`
- Redis: `repositories.RedisClient`
- MongoDB: `repositories/mongo.GetCollection(name)`
- AI provider config: `config.AppConfig.AI`, 当前预留 DeepSeek OpenAI-compatible 配置。

改任一模型字段、表名或集合名时, 同步更新对应 Skill。
