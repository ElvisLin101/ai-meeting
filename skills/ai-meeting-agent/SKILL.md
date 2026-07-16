---
name: ai-meeting-agent
description: 当需求涉及 Agent 会话、Agent Chat、AgentProperties、AgentMessage、文件上传、场景绑定或 Agent 历史消息时使用。
---

# AI-Meeting Agent Skill

## 何时使用

读取本 Skill 的场景:

- `/agents/**` 接口变更。
- Agent 会话创建、结束、分页、历史消息查询。
- Agent Chat SSE 流式聊天、接入讯飞星辰工作流、双消息持久化。
- Agent 配置 CRUD、启动缓存、场景绑定热插拔。
- Agent 文件上传和 `AgentFileAsset` 持久化。

## 代码地图

- 路由: `api/routes/routes.go` 中 `setupAgentRoutes`。
- Handler: `api/handlers/agent_handler.go`。
- Service: `services/agent/agent_service.go`。
- 场景枚举: `services/agent/agent_scene.go`（5 个 BusinessAgentScene + 候选名称）。
- 启动缓存 + 场景解析器: `services/agent/agent_properties_loader.go`（sync.Map 内存缓存 + miss 查库 + ResolveRequired）。
- 讯飞星辰客户端: `clients/xingchen_client.go`（ChatStream + ChatSync + UploadFile）。
- MySQL 仓储: `repositories/mysql/agent_conversation_repository.go`, `repositories/mysql/agent_properties_repository.go`, `repositories/mysql/agent_file_asset_repository.go`。
- Mongo 消息仓储: `repositories/mongo/agent_message_repository.go`。
- DTO: `dto/agent.go`。
- 模型: `models/agent.go`（AgentProperties 含 ApiKey/ApiSecret/ApiFlowId；AgentMessage 含 ResponseTime/ErrorMessage）。
- 长上下文: `services/common/memory_service.go`, 需要时再读 `ai-meeting-memory`。

## 核心流程

`AgentSessionCreate`

- 路由: `POST /api/xunzhi/v1/agents/sessions`。
- Handler 从 JWT 上下文读取 `username`。
- Service `CreateConversationWithTitle` 生成 UUID 作为 `session_id`。
- 标题默认 `New Conversation`, 首条消息超过 50 字符时截断。
- 当前 `agentID` 参数硬编码为 "1", 入库 `AgentID` 固定为 1。

`AgentChat`（已实现 SSE 流式闭环）

- 路由: `POST /api/xunzhi/v1/agents/sessions/:sessionId/chat`。
- Handler 设置 SSE 响应头（`Content-Type: text/event-stream`）。
- Service `AgentChatSSE` 完整流程:
  1. 会话归属校验（`GetAgentConversationBySessionId`）。
  2. 解析智能体配置（`AgentPropertiesLoader.GetByAgentID`，先查 sync.Map 缓存 miss 查库）。
  3. 校验 apiKey/apiSecret/apiFlowId 非空。
  4. 保存用户消息到 MongoDB（`SaveAgentMessage`）。
  5. 加载历史消息 → 构建 `XingChenHistoryItem` 数组（排除最后一条当前消息）。
  6. 调用 `XingChenClient.ChatStream`（讯飞星辰工作流 SSE 流式），通过 `onChunk` 回调 `ctx.SSEvent("message", chunk)` 推送前端。
  7. 保存 assistant 回复（`SaveAgentMessageWithDetail`，含 responseTime 和 errorMessage）。
  8. 更新会话消息计数（`UpdateAgentConversationMessageCount`）。
  9. 异步触发记忆压缩（`MemoryService.CompressContext`）。
- 出错时也保存一条错误 assistant 消息（content="Sorry, an error occurred..."，errorMessage=err.Error()）。
- 最终发送 `ctx.SSEvent("end", "[DONE]")`。

`AgentHistory`

- `GET /agents/conversations/:sessionId/messages` 从 Mongo 按 `sequence ASC` 返回完整历史。
- `GET /agents/messages/history` 从 Mongo 按 `created_at DESC` 分页。
- 两个查询都必须保留用户隔离条件。

`AgentMemoryThreshold`

- `GET /agents/memory/threshold` 查询当前压缩阈值、最小值、最大值和触发偏移。
- `PUT /agents/memory/threshold` 通过 JSON `{"threshold":4096}` 修改运行时阈值。
- 阈值限制在 `1024 <= threshold <= 32768`, 且当前为内存态配置。

`AgentProperties`

- 表 `agent_properties` 保存名称、描述、配置、启用状态、apiKey、apiSecret、apiFlowId。
- `GetByPage` 当前固定 `Limit(10)`, 没有使用请求分页参数。
- 启动时 `AgentPropertiesLoader.RefreshActiveAgents` 全量加载到 sync.Map。
- `GetByAgentID` / `GetByAgentName` 先查缓存 miss 查库。
- `ResolveRequired(scene)` 按场景候选名称顺序匹配，支持热插拔。

`BusinessAgentScene`（场景枚举）

- 5 个场景: GeneralAgentChat / InterviewQuestionExtraction / InterviewAnswerEvaluation / InterviewDemeanor / InterviewQuestionAsking。
- 每个场景有默认名称 + 别名列表（如评分官: "用户答案评分官" / "面试答案评分官"）。
- `ResolveRequired` 按候选名称顺序从缓存查找，支持运营改数据库不改代码。

`AgentFileUpload`

- 路由: `POST /api/xunzhi/v1/agents/files/upload`。
- 当前保存路径为 `./uploads/` + 原始文件名, 然后写入 `agent_file_assets`。
- `XingChenClient.UploadFile` 已实现但未接入此流程（后续面试简历/照片上传使用）。
- 改动时检查目录存在、文件名净化、重名覆盖和大小限制。

## 影响检查

- 修改 Agent Chat 时, 同步检查 memory 上下文、消息计数、会话标题、错误重试和 assistant 消息落库。
- 修改 `AgentMessage.Sequence` 时, 必须读 `ai-meeting-memory`。
- 修改 Agent 消息存储时, 同时检查 `repositories/mongo/agent_message_repository.go` 和 memory Skill。
- 修改 Agent 表字段时, 更新 `docs/agent-knowledge/references/data-models.md`。
- 修改路由时, 更新 `docs/agent-knowledge/references/routes-map.md`。
- 修改场景枚举或解析器时, 更新本 Skill 的 BusinessAgentScene 部分。

## 当前风险

- `CreateConversationWithTitle` 忽略传入的 agentID, 固定写 1。
- 上传文件路径没有净化。
- 多个 Service 单例不是并发安全初始化, 当前代码只是简单懒加载。
- `AgentPropertiesLoader` 使用 sync.Map 单实例缓存, 多实例部署时配置变更需重启或手动刷新。
- `GetByPage` 固定查 10 条, 未使用分页参数。
