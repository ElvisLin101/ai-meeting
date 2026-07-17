---
name: ai-meeting-ai
description: 当需求涉及 AI 会话、AI 消息、AI 模型配置、/ai 路由或通用 AI provider 配置时使用。
---

# AI-Meeting AI Skill

## 何时使用

读取本 Skill 的场景:

- `/ai/**` 和 `/ai-properties/**` 接口变更。
- AI 会话创建、更新、结束、删除、详情查询。
- AI 消息保存、历史查询、分页。
- AI 模型配置 CRUD、启用状态、模型选项。
- 给普通 AI Chat 接入真实模型响应。

## 代码地图

- 路由: `api/routes/routes.go` 中 `setupAiRoutes`。
- Handler: `api/handlers/ai_handler.go`。
- Service: `services/ai/ai_service.go`, `services/ai/ai_chat_service.go`, `services/ai/ai_memory_service.go`。
- MySQL 仓储: `repositories/mysql/ai_properties_repository.go`(AI 模型配置)。
- Mongo 仓储: `repositories/mongo/ai_conversation_repository.go`(会话), `repositories/mongo/ai_message_repository.go`(消息)。
- 模型客户端: `clients/ai_model_client.go`。
- DTO: `dto/ai.go`。
- 模型: `models/ai.go`。

## 核心流程

`AiConversation`

- 创建路由: `POST /api/xunzhi/v1/ai/conversations`。
- 用 UUID 生成 `session_id`, `user_id` 字段存 JWT `username`。
- 标题默认 `New AI Conversation`, 首条消息超过 50 字符时截断。
- 更新会话通过 query 参数 `messageCount` 和 `title`。
- 删除会话会删除 `ai_conversations`, 同步清理 MongoDB `ai_messages` 和 AI 压缩上下文。

`AiChat`

- 路由: `POST /api/xunzhi/v1/ai/sessions/:sessionId/chat`。
- SSE 路由: `POST /api/xunzhi/v1/ai/sessions/:sessionId/chat/stream`。
- 聊天前校验 `sessionId` 属于当前 JWT `username`。
- 请求模型前从 `AiMemoryService` 读取 AI 会话长期记忆和近期窗口。
- 用户消息和 assistant 回复都保存到 MongoDB `ai_messages`。
- 通过 OpenAI-compatible endpoint 调用模型；默认优先读取 `config.yaml` 中 `ai.deepseek` 的 DeepSeek 配置。
- 会话绑定有效 `AiID` 时优先使用 MySQL `AiProperties`, 未绑定或未找到启用模型时回退到 `ai.deepseek`。
- 模型回复后更新 `ai_conversations.message_cnt` 和 `updated_at`。
- 保存完整一轮对话后异步触发 AI 上下文压缩。
- 普通 chat 返回 JSON, 响应字段包含 `content`, `user_message_id`, `assistant_message_id`。
- SSE chat 输出 `message` 事件作为回答增量, `reasoning` 事件作为 DeepSeek `reasoning_content` 增量, 最后输出 `done` 事件携带消息 ID。

`AiMemory`

- 阈值路由: `GET /api/xunzhi/v1/ai/memory/threshold`, `PUT /api/xunzhi/v1/ai/memory/threshold`。
- Redis 摘要 key: `memory:ai:{sessionId}:summary`。
- Redis 索引 key: `memory:ai:{sessionId}:index`。
- MongoDB 压缩快照集合: `compressed_contexts`, `_id=ai:{sessionId}`。
- 原始消息集合: MongoDB `ai_messages`。
- 压缩只压旧的 80% 消息, 最新 20% 继续通过 `index` 后的 Mongo 原文窗口补齐。

`AiProperties`

- 表 `ai_properties` 保存模型名称、类型、api key、api secret、endpoint、config、启用状态。
- `/ai-properties/options` 和 `/enabled` 只返回启用模型。
- 响应 DTO 当前不返回 key/secret。

`PresetModels`（预设模型模板）

- 代码: `clients/ai_model_presets.go`。
- 预设 7 个模型: DeepSeek / 豆包 / GLM(智谱) / 通义千问 / Moonshot(Kimi) / OpenAI / 自定义。
- 每个预设含 endpoint、默认 modelType、apiKey 申请地址、配置 JSON 示例。
- `GET /ai-properties/presets` 返回预设模板列表。
- `POST /ai-properties/preset` 按模板创建配置: 用户传 provider + name + apiKey, 缺省字段用预设默认值填充。
- 自定义模型(provider=custom)需用户自行填入 endpoint + apiKey + modelType + config。
- 也支持直接 `POST /ai-properties` 完全自定义创建（不经过模板）。

`DeepSeekConfig`

- `config/config.yaml` 预留 `ai.provider=deepseek` 和 `ai.deepseek`。
- 默认 endpoint 是 `https://api.deepseek.com/chat/completions`, 默认 model 是 `deepseek-chat`。
- `api_key` 可留空；留空时不会启用 config fallback, 仍可走 MySQL `AiProperties`。
- 支持环境变量覆盖, 常用变量是 `AI_DEEPSEEK_API_KEY`。

## 修改指南

1. 接入真实模型调用时, 优先沿用 `clients/ai_model_client.go`, 不要在 handler 或 service 里直接写 HTTP 调用。
2. 保存 assistant 回复时, 保持 `sequence` 单调递增。
3. 更新消息数、标题生成和失败回滚策略要和会话表一致。
4. 如果 AI 配置影响 memory 压缩, 同步读 `ai-meeting-memory`。
5. 修改模型字段或路由后更新 references。

## 当前风险

- `AiMessage.Sequence` 仍是查询当前最大值后加一, 高并发同会话写入时仍可能重复。
- AI memory 阈值是运行时内存配置, 服务重启后恢复默认值。
- 非流式 OpenAI-compatible 响应解析只支持 `choices[].message.content` 或 `choices[].text`; 流式解析支持 `delta.content`, `delta.reasoning_content` 和 `text`。
- SSE 流式调用中途断开时, 已保存 user 消息, 但不会保存不完整 assistant 回复。
- `AiProperties.ApiKey` 和 `AiProperties.ApiSecret` 明文入库; `config.yaml` 中的 `ai.deepseek.api_key` 也要按本地密钥处理。
- `UpdateConversation` 使用 query 参数, 不是 JSON body。
