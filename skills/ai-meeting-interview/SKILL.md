---
name: ai-meeting-interview
description: 当需求涉及面试会话、面试题、作答评分、建议、雷达图、简历预览、面试记录或 /interview 路由时使用。
---

# AI-Meeting Interview Skill

## 何时使用

读取本 Skill 的场景:

- `/interview/**` 接口变更。
- 面试会话生命周期、题目抽取、答题评分、下一题、恢复会话。
- 简历评分、雷达图、表情评估。
- 面试记录保存、查询和从 Redis 归档。

## 代码地图

- 路由: `api/routes/routes.go` 中 `setupInterviewRoutes`。
- Handler: `api/handlers/interview_handler.go`。
- Service: `services/interview_service.go`。
- MySQL 仓储: `repositories/mysql/interview_session_repository.go`, `repositories/mysql/interview_record_repository.go`。
- 历史消息占位查询: `repositories/mysql/agent_message_repository.go`。
- DTO: `dto/interview.go`。
- 模型: `models/interview.go`。
- 会话列表当前部分复用 `models.AgentConversation` 和 `models.AgentMessage`。

## 核心流程

`InterviewSessionCreate`

- 路由: `POST /api/xunzhi/v1/interview/sessions`。
- Handler 读取 JWT 上下文 `user_id`。
- Service 创建 `models.InterviewSession`, 状态为 1。

`InterviewConversationList`

- 路由: `GET /api/xunzhi/v1/interview/conversations`。
- 当前 Service 查询的是 `models.AgentConversation`, 不是 `InterviewSession`。
- 如果产品语义是面试会话列表, 修改前要确认是否应迁移到 `InterviewSession` 或专门的 conversation 表。

`InterviewQuestionAndAnswer`

- 提题、答题、下一题、当前题、恢复、评分、建议、雷达图、表情评估等方法目前大多返回示例数据。
- 不要把这些返回值当成真实业务规则。
- 接入真实 AI 或 Redis 状态时, 要定义题目状态、作答状态、评分维度和幂等策略。

`InterviewRecord`

- `SaveInterviewRecord` 会写入 `interview_records`。
- `PageInterviewRecords` 可按 `session_id` 过滤。
- `GetBySessionId` 当前只返回第一条记录。
- `SaveInterviewRecordFromRedis` 仍是空实现。

`ResumePreview`

- 当前只返回 `"Resume preview endpoint"`。

## 修改指南

1. 先读 `docs/agent-knowledge/references/placeholder-risk-register.md`, 判断是不是在替换占位逻辑。
2. 新增真实面试状态时, 优先补齐模型字段和状态流转, 再接 AI 调用。
3. 涉及用户隔离时使用 `user_id`, 不要误用 `username`。
4. 如果复用 Agent 消息表, 明确区分普通 Agent 会话和面试会话。
5. 修改接口后同步更新 `routes-map.md`; 修改表结构后同步更新 `data-models.md`。

## 当前风险

- 面试核心业务多为示例返回值。
- 会话创建写 `interview_sessions`, 会话列表却读 `agent_conversations`。
- 面试记录按 session 查询只取第一条, 不适合多题记录展示。
- Redis 归档和简历预览未实现。
