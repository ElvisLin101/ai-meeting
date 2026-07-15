# Placeholder And Drift Register

这个文件登记“看起来有接口, 但业务尚未完整实现”的位置。修改相关模块时, 先判断是否正在替换这些占位逻辑。

## Agent

- `api/handlers/agent_handler.go`: `AgentController.Chat` 保存用户消息并异步触发 memory 压缩判断, 但不调用模型, 不保存 assistant 回复。
- `services/agent_service.go`: `CreateConversationWithTitle` 入参有 `agentID`, 但当前 `AgentID` 固定为 1。
- `services/agent_service.go`: `GetConversationHistoryWithContext` 已接 memory, 但 handler 未使用。

## AI

- `api/handlers/ai_handler.go`: `AiMessageController.ChatStream` 已接 SSE, 但流式中断时不会保存不完整 assistant 回复。
- `repositories/mongo/ai_message_repository.go`: `AiMessage.Sequence` 通过查询当前最大值后加一生成, 高并发同会话写入时可能重复。
- `clients/ai_model_client.go`: 模型调用按 OpenAI-compatible endpoint 解析, 非兼容 provider 需要适配。
- `config/config.yaml`: `ai.deepseek.api_key` 当前预留为空, 本地运行真实模型前需要填入或通过 `AI_DEEPSEEK_API_KEY` 覆盖。

## Interview

- `services/interview_service.go`: `ExtractInterviewQuestions`, `AnswerInterviewQuestion`, `GetNextQuestion`, `GetCurrentQuestion`, `RestoreInterviewSession`, `GetSessionInterviewQuestions`, `GetSessionTotalScore`, `GetSessionInterviewSuggestions`, `GetSessionResumeScore`, `GetRadarChartData`, `EvaluateDemeanor` 均返回示例数据。
- `services/interview_service.go`: `SaveInterviewRecordFromRedis` 是空实现。
- `api/handlers/interview_handler.go`: `PreviewResume` 只返回固定提示。
- `InterviewSessionFacade.CreateSession` 写 `InterviewSession`, 但 `PageConversations` 读 `AgentConversation`。

## Memory

- `services/memory_service.go`: `SetCompressionThreshold` 是运行时内存配置, 服务重启后恢复默认阈值。
- `services/ai_memory_service.go`: `SetCompressionThreshold` 是运行时内存配置, 服务重启后恢复默认阈值。
- `services/ai_memory_service.go`: 只做当前进程内 `sync.Map` 防重复压缩, 多实例部署时仍可能并发压缩同一 AI 会话。

## User/Auth

- `services/user_service.go`: 密码明文比较。
- `api/middleware/auth.go`: 缺失或无效 token 默认放行。
- `api/handlers/user_handler.go`: `GenerateToken(user.Username, string(rune(user.ID)))` 可能不是预期的 user ID 字符串。

替换任意占位逻辑后, 从本文件移除或改写对应条目。
