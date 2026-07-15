---
name: ai-meeting-agent
description: 当需求涉及 Agent 会话、Agent Chat、AgentProperties、AgentMessage、文件上传或 Agent 历史消息时使用。
---

# AI-Meeting Agent Skill

## 何时使用

读取本 Skill 的场景:

- `/agents/**` 接口变更。
- Agent 会话创建、结束、分页、历史消息查询。
- Agent Chat 收发消息、接入模型回复、接入 memory 上下文。
- Agent 配置 CRUD。
- Agent 文件上传和 `AgentFileAsset` 持久化。

## 代码地图

- 路由: `api/routes/routes.go` 中 `setupAgentRoutes`。
- Handler: `api/handlers/agent_handler.go`。
- Service: `services/agent_service.go`。
- MySQL 仓储: `repositories/mysql/agent_conversation_repository.go`, `repositories/mysql/agent_properties_repository.go`, `repositories/mysql/agent_file_asset_repository.go`。
- Mongo 消息仓储: `repositories/mongo/agent_message_repository.go`。
- DTO: `dto/agent.go`。
- 模型: `models/agent.go`。
- 长上下文: `services/memory_service.go`, 需要时再读 `ai-meeting-memory`。

## 核心流程

`AgentSessionCreate`

- 路由: `POST /api/xunzhi/v1/agents/sessions`。
- Handler 从 JWT 上下文读取 `username`。
- Service `CreateConversationWithTitle` 生成 UUID 作为 `session_id`。
- 标题默认 `New Conversation`, 首条消息超过 50 字符时截断。
- 当前 `agentID` 参数未被使用, 入库 `AgentID` 固定为 1。

`AgentChat`

- 路由: `POST /api/xunzhi/v1/agents/sessions/:sessionId/chat`。
- 当前行为: 向 Mongo 集合 `agent_messages` 保存一条 role=`user` 的 `AgentMessage`, 然后返回 `Message received`。
- 保存后会异步调用 `GetConversationHistoryWithContext`, 用 Mongo 消息拼上下文并按阈值触发压缩。
- 当前没有调用模型, 没有保存 assistant 回复, 也没有更新 `AgentConversation.message_cnt`。
- 如果要接入真实 Agent 回复, 应该先调用 memory Skill 中的上下文流程。

`AgentHistory`

- `GET /agents/conversations/:sessionId/messages` 从 Mongo 按 `sequence ASC` 返回完整历史。
- `GET /agents/messages/history` 从 Mongo 按 `created_at DESC` 分页。
- 两个查询都必须保留用户隔离条件。

`AgentMemoryThreshold`

- `GET /agents/memory/threshold` 查询当前压缩阈值、最小值、最大值和触发偏移。
- `PUT /agents/memory/threshold` 通过 JSON `{"threshold":4096}` 修改运行时阈值。
- 阈值限制在 `1024 <= threshold <= 32768`, 且当前为内存态配置。

`AgentProperties`

- 表 `agent_properties` 保存名称、描述、配置、启用状态。
- `GetByPage` 当前固定 `Limit(10)`, 没有使用请求分页参数。

`AgentFileUpload`

- 路由: `POST /api/xunzhi/v1/agents/files/upload`。
- 当前保存路径为 `./uploads/` + 原始文件名, 然后写入 `agent_file_assets`。
- 改动时检查目录存在、文件名净化、重名覆盖和大小限制。

## 影响检查

- Chat 接入真实回复时, 同步检查 memory 上下文、消息计数、会话标题、错误重试和 assistant 消息落库。
- 修改 `AgentMessage.Sequence` 时, 必须读 `ai-meeting-memory`。
- 修改 Agent 消息存储时, 同时检查 `repositories/mongo/agent_message_repository.go` 和 memory Skill。
- 修改 Agent 表字段时, 更新 `docs/agent-knowledge/references/data-models.md`。
- 修改路由时, 更新 `docs/agent-knowledge/references/routes-map.md`。

## 当前风险

- `AgentChat` 是半成品: 只落库用户消息。
- `CreateConversationWithTitle` 忽略传入的 agentID。
- 上传文件路径没有净化。
- 多个 Service 单例不是并发安全初始化, 当前代码只是简单懒加载。
