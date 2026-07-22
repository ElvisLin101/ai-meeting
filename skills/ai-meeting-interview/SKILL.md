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
- 面试长会话运行态治理(状态机、运行态恢复、热冷快照、轮次归档、幂等补偿) → 先读 `docs/agent-knowledge/references/interview-runtime-governance.md`。

## 代码地图

- 路由: `api/routes/routes.go` 中 `setupInterviewRoutes`。
- Handler: `api/handlers/interview_handler.go`。
- Service: `services/interview/interview_service.go`。
- 状态机: `services/interview/flow/flow_state_machine.go`, `services/interview/flow/follow_up_rule.go`。
- 答题流水线: `services/interview/flow/answer_pipeline.go`(编排: 幂等→锁→评分→推进flow→写分→turn log), `services/interview/flow/idempotency_service.go`(processing/replay双key), `services/interview/flow/turn_repair_service.go`(turn log 写失败异步补偿)。
- 快照/恢复: `services/interview/runtime/snapshot_service.go`(refreshSnapshot 写 Mongo CAS + ensureRuntime 从 Mongo 重建 Redis)。
- 通用分布式锁: `pkg/lock/lock.go`(题级锁 SetNX+Lua)。
- AI 调用(评分/出题/追问): `services/interview/evaluation/evaluation_service.go`, `services/interview/evaluation/extraction_service.go`, `services/interview/evaluation/followup_service.go`, `services/interview/evaluation/prompt.go`, `services/interview/evaluation/response_parser.go`。走 DeepSeek(`CallConfiguredAIChat`, aiID=0)。
- 运行态缓存: `services/interview/runtime/cache_keys.go`, `services/interview/runtime/flow_cache.go`, `services/interview/runtime/score_cache.go`, `services/interview/runtime/turn_log_cache.go`, `services/interview/runtime/question_cache.go`(题目/建议/简历上下文)。
- Mongo 仓储: `repositories/mongo/interview_session_repository.go`, `repositories/mongo/interview_record_repository.go`, `repositories/mongo/interview_question_repository.go`。
- 历史消息查询复用 Mongo: `repositories/mongo/agent_message_repository.go`。
- DTO: `dto/interview.go`。
- 模型: `models/interview.go`(会话/记录/题目), `models/interview_runtime.go`(FlowState/TurnLog/阶段枚举)。
- PDF 简历解析: `clients/resume_parser.go`(ledongthuc/pdf, 提取文本)。
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

- 出题: `POST /sessions/:sessionId/interview-questions`(body: `resume_content` 文本) 或 `POST /sessions/:sessionId/resume/upload`(multipart file: PDF)。PDF 上传后解析文本→回填 `InterviewSession.ResumePath`(Mongo)→走出题流程。
- 出题流程: `ExtractionService.ExtractQuestions`(DeepSeek) → 写 Redis(questions/suggestions/resumeScore/direction/resumeContext) → `ResetScore` + `EnsureInitialized`(flow=ASKING, Q#="1") → 返回第一题。
- 答题: `POST /sessions/:sessionId/interview/answer-json`(body: question_number/answer_content/request_id)。走 `AnswerPipeline`: 幂等→题级锁→评分→推进flow→计分→turn log。
- 取题: `GET /sessions/:sessionId/current-question` 或 `/next-question`。读 flow + Redis 题面。
- 恢复: `GET /sessions/:sessionId/restore`。读 flow + Redis 返回当前题号/题面/总分。
- 评分查询: `GET /sessions/:sessionId/interview/score`(Redis score key)、`GET /sessions/:sessionId/resume/score`(Redis resumeScore key)。
- 题目查询: `GET /sessions/:sessionId/interview/questions`(Redis questions Hash)、`GET /sessions/:sessionId/interview/suggestions`(Redis suggestions Hash)。
- 雷达图: `GET /sessions/:sessionId/radar-chart`。从 resumeScore + interviewScore 加权计算四维（简历匹配/面试表现/专业技能/综合潜力）。
- 简历预览: `GET /sessions/:sessionId/resume/preview`。从 Mongo 读 ResumePath → 解析 PDF 返回文本。
- 表情/神态评估已移除（不依赖多模态 API）。

`InterviewRecord`

- `SaveInterviewRecord` 会写入 MongoDB `interview_records`。
- `PageInterviewRecords` 可按 `session_id` 过滤。
- `GetBySessionId` 当前只返回第一条记录。
- `SaveInterviewRecordFromRedis` 已实现: 从 Mongo `TurnArchive` 汇总轮次算平均分, 取最后一轮作报告概要, 写入 `InterviewRecord`。

`ResumePreview`

- 从 Mongo 读 `ResumePath` → 解析 PDF 返回文本。

## 修改指南

1. 先读 `docs/agent-knowledge/references/placeholder-risk-register.md`, 判断是不是在替换占位逻辑。
2. 新增真实面试状态时, 优先补齐模型字段和状态流转, 再接 AI 调用。
3. 涉及用户隔离时使用 `user_id`, 不要误用 `username`。
4. 如果复用 Agent 消息表, 明确区分普通 Agent 会话和面试会话。
5. 修改接口后同步更新 `routes-map.md`; 修改表结构后同步更新 `data-models.md`。

## 当前风险

- 会话创建写 MongoDB `interview_sessions`, 会话列表却读 Mongo `agent_conversations`（数据源错位, 均在 Mongo 但表不同）。
- 面试记录按 session 查询只取第一条, 不适合多题记录展示。
- 运行态治理(状态机/运行态恢复/热冷快照/轮次归档/幂等补偿)已落地, 详见 `docs/agent-knowledge/references/interview-runtime-governance.md` 及 `services/interview/flow/`、`services/interview/runtime/`、`services/interview/evaluation/` 目录。`InterviewQuestion` 已有 Mongo 仓储并被 `question_cache` 使用。
