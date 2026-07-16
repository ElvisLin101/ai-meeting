# Routes Map

基础前缀: `/api/xunzhi/v1`

## User

| Method | Path | Handler | Service |
| --- | --- | --- | --- |
| POST | `/users/login` | `UserHandler.Login` | `UserService.Login` |
| POST | `/users/register` | `UserHandler.Register` | `UserService.Register` |
| POST | `/users/logout` | `UserHandler.Logout` | none |
| GET | `/users/check-login` | `UserHandler.CheckLogin` | none |
| GET | `/users/is-admin` | `UserHandler.IsAdmin` | `UserService.IsAdmin` |
| GET | `/users/has-username` | `UserHandler.HasUsername` | `UserService.HasUsername` |
| GET | `/users/:username` | `UserHandler.GetUserByUsername` | `UserService.GetUserByUsername` |
| GET | `/users/actual/:username` | `UserHandler.GetUserByUsername` | `UserService.GetUserByUsername` |
| PUT | `/users` | `UserHandler.Update` | `UserService.Update` |
| POST | `/users/admin` | `UserHandler.AddAdmin` | `UserService.SetAdmin` |
| GET | `/users/page` | `UserHandler.PageUsers` | `UserService.PageUsers` |

## Agent

| Method | Path | Handler | Service |
| --- | --- | --- | --- |
| POST | `/agents/sessions` | `AgentController.CreateSession` | `AgentConversationService.CreateConversationWithTitle` |
| POST | `/agents/sessions/:sessionId/chat` | `AgentController.Chat` | `AgentMessageService.SaveMessage` |
| GET | `/agents/conversations` | `AgentController.PageConversations` | `AgentConversationService.PageConversations` |
| GET | `/agents/conversations/:sessionId/messages` | `AgentController.GetConversationHistory` | `AgentMessageService.GetConversationHistory` |
| GET | `/agents/messages/history` | `AgentController.PageHistoryMessages` | `AgentMessageService.PageHistoryMessages` |
| PUT | `/agents/conversations/:sessionId/end` | `AgentController.EndConversation` | `AgentConversationService.EndConversation` |
| GET | `/agents/memory/threshold` | `AgentController.GetMemoryThreshold` | `MemoryService.GetCompressionThresholdConfig` |
| PUT | `/agents/memory/threshold` | `AgentController.SetMemoryThreshold` | `MemoryService.SetCompressionThreshold` |
| POST | `/agents/files/upload` | `AgentFileController.Upload` | `AgentFileAssetService.UploadAndPersist` |
| POST | `/agent-properties` | `AgentPropertiesController.Create` | `AgentPropertiesService.Create` |
| DELETE | `/agent-properties/:id` | `AgentPropertiesController.Delete` | `AgentPropertiesService.Delete` |
| PUT | `/agent-properties` | `AgentPropertiesController.Update` | `AgentPropertiesService.Update` |
| GET | `/agent-properties/byName` | `AgentPropertiesController.GetByName` | `AgentPropertiesService.GetByName` |
| GET | `/agent-properties` | `AgentPropertiesController.GetByPage` | `AgentPropertiesService.GetByPage` |

## AI

| Method | Path | Handler | Service |
| --- | --- | --- | --- |
| POST | `/ai/conversations` | `AiConversationController.CreateConversation` | `AiConversationService.CreateConversationWithTitle` |
| GET | `/ai/conversations` | `AiConversationController.PageConversations` | `AiConversationService.PageConversations` |
| PUT | `/ai/conversations/:sessionId` | `AiConversationController.UpdateConversation` | `AiConversationService.UpdateConversation` |
| PUT | `/ai/conversations/:sessionId/end` | `AiConversationController.EndConversation` | `AiConversationService.EndConversation` |
| DELETE | `/ai/conversations/:sessionId` | `AiConversationController.DeleteConversation` | `AiConversationService.DeleteConversation` |
| GET | `/ai/conversations/:sessionId` | `AiConversationController.GetConversationById` | `AiConversationService.GetConversationBySessionId` |
| POST | `/ai/sessions/:sessionId/chat` | `AiMessageController.Chat` | `AiMessageService.Chat` |
| POST | `/ai/sessions/:sessionId/chat/stream` | `AiMessageController.ChatStream` | `AiMessageService.ChatStream` |
| GET | `/ai/history/:sessionId` | `AiMessageController.GetConversationHistory` | `AiMessageService.GetConversationHistory` |
| GET | `/ai/history/page` | `AiMessageController.PageHistoryMessages` | `AiMessageService.PageHistoryMessages` |
| GET | `/ai/memory/threshold` | `AiMessageController.GetMemoryThreshold` | `AiMemoryService.GetCompressionThresholdConfig` |
| PUT | `/ai/memory/threshold` | `AiMessageController.SetMemoryThreshold` | `AiMemoryService.SetCompressionThreshold` |
| GET | `/ai-properties/options` | `AiPropertiesController.GetAvailableAiModels` | `AiPropertiesService.GetAvailableAiModels` |
| GET | `/ai-properties/presets` | `AiPropertiesController.GetPresetModels` | `clients.PresetModels` |
| POST | `/ai-properties/preset` | `AiPropertiesController.CreateFromPreset` | `AiPropertiesService.CreateAiProperties` |
| POST | `/ai-properties` | `AiPropertiesController.CreateAiProperties` | `AiPropertiesService.CreateAiProperties` |
| PUT | `/ai-properties` | `AiPropertiesController.UpdateAiProperties` | `AiPropertiesService.UpdateAiProperties` |
| DELETE | `/ai-properties/:id` | `AiPropertiesController.DeleteAiProperties` | `AiPropertiesService.DeleteAiProperties` |
| GET | `/ai-properties/:id` | `AiPropertiesController.GetAiPropertiesById` | `AiPropertiesService.GetAiPropertiesById` |
| GET | `/ai-properties` | `AiPropertiesController.PageAiProperties` | `AiPropertiesService.PageAiProperties` |
| GET | `/ai-properties/enabled` | `AiPropertiesController.GetAllEnabledAiProperties` | `AiPropertiesService.GetAllEnabledAiProperties` |
| PUT | `/ai-properties/:id/status` | `AiPropertiesController.ToggleAiPropertiesStatus` | `AiPropertiesService.ToggleAiPropertiesStatus` |

## Interview

| Method | Path | Handler | Service |
| --- | --- | --- | --- |
| POST | `/interview/sessions` | `InterviewSessionController.CreateSession` | `InterviewSessionFacade.CreateSession` |
| GET | `/interview/conversations` | `InterviewSessionController.PageConversations` | `InterviewSessionFacade.PageConversations` |
| GET | `/interview/conversations/:sessionId/messages` | `InterviewSessionController.GetConversationHistory` | `InterviewSessionFacade.GetConversationHistory` |
| GET | `/interview/messages/history` | `InterviewSessionController.PageHistoryMessages` | `InterviewSessionFacade.PageHistoryMessages` |
| PUT | `/interview/sessions/:sessionId/finish` | `InterviewSessionController.FinishSession` | `InterviewSessionFacade.FinishSession` |
| PUT | `/interview/conversations/:sessionId/end` | `InterviewSessionController.EndConversation` | `InterviewSessionFacade.EndConversation` |
| POST | `/interview/sessions/:sessionId/interview-questions` | `InterviewSessionController.ExtractInterviewQuestions` | `InterviewSessionFacade.ExtractInterviewQuestions` |
| POST | `/interview/sessions/:sessionId/interview/answer` | `InterviewSessionController.AnswerInterviewQuestion` | `InterviewSessionFacade.AnswerInterviewQuestion` |
| POST | `/interview/sessions/:sessionId/interview/answer-json` | `InterviewSessionController.AnswerInterviewQuestionJson` | `InterviewSessionFacade.AnswerInterviewQuestion` |
| GET | `/interview/sessions/:sessionId/next-question` | `InterviewSessionController.GetNextQuestion` | `InterviewSessionFacade.GetNextQuestion` |
| GET | `/interview/sessions/:sessionId/current-question` | `InterviewSessionController.GetCurrentQuestion` | `InterviewSessionFacade.GetCurrentQuestion` |
| GET | `/interview/sessions/:sessionId/restore` | `InterviewSessionController.RestoreInterviewSession` | `InterviewSessionFacade.RestoreInterviewSession` |
| GET | `/interview/sessions/:sessionId/interview/questions` | `InterviewSessionController.GetSessionInterviewQuestions` | `InterviewSessionFacade.GetSessionInterviewQuestions` |
| GET | `/interview/sessions/:sessionId/interview/score` | `InterviewSessionController.GetSessionTotalScore` | `InterviewSessionFacade.GetSessionTotalScore` |
| GET | `/interview/sessions/:sessionId/interview/suggestions` | `InterviewSessionController.GetSessionInterviewSuggestions` | `InterviewSessionFacade.GetSessionInterviewSuggestions` |
| GET | `/interview/sessions/:sessionId/resume/score` | `InterviewSessionController.GetSessionResumeScore` | `InterviewSessionFacade.GetSessionResumeScore` |
| GET | `/interview/sessions/:sessionId/radar-chart` | `InterviewSessionController.GetRadarChartData` | `InterviewSessionFacade.GetRadarChartData` |
| POST | `/interview/sessions/:sessionId/demeanor-evaluation` | `InterviewSessionController.EvaluateDemeanor` | `InterviewSessionFacade.EvaluateDemeanor` |
| POST | `/interview/interview/record` | `InterviewRecordController.SaveInterviewRecord` | `InterviewRecordService.SaveInterviewRecord` |
| GET | `/interview/interview/records` | `InterviewRecordController.PageInterviewRecords` | `InterviewRecordService.PageInterviewRecords` |
| GET | `/interview/interview/record/:sessionId` | `InterviewRecordController.GetInterviewRecordBySessionId` | `InterviewRecordService.GetBySessionId` |
| POST | `/interview/interview/record/save-from-redis/:sessionId` | `InterviewRecordController.SaveInterviewRecordFromRedis` | `InterviewRecordService.SaveInterviewRecordFromRedis` |
| GET | `/interview/sessions/:sessionId/resume/preview` | `InterviewResumeController.PreviewResume` | none |

## Media

媒体路由位于 `setupMediaRoutes`, 但对应 handler/service 文件当前不在本次知识初始覆盖范围。修改媒体接口时先补充新的 media Skill。
