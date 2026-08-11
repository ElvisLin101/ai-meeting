package routes

import (
	"ai-meeting/api/handlers"
	"ai-meeting/api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(middleware.Recovery())

	// 静态前端页面
	r.StaticFile("/", "./static/index.html")

	r.Use(middleware.CORSMiddleware())
	// AuthMiddleware: 解析 JWT, 有效则注入 username/user_id; 缺失/无效不拦截——
	// 公开接口(登录/注册等)需要放行, 受保护路由由 RequireAuth 在组内强制认证
	r.Use(middleware.AuthMiddleware())

	api := r.Group("/api/xunzhi/v1")

	// 受保护路由组: 无有效 JWT 直接 401(补齐"缺失 token 默认放行"的安全缺口)
	authed := api.Group("")
	authed.Use(middleware.RequireAuth())

	setupUserRoutes(api, authed)
	setupAgentRoutes(authed)
	setupAiRoutes(authed)
	setupInterviewRoutes(authed)
	setupMediaRoutes(authed)

	return r
}

func setupUserRoutes(api, authed *gin.RouterGroup) {
	userHandler := handlers.NewUserHandler()

	// 公开接口(无需登录)
	public := api.Group("/users")
	public.POST("/login", userHandler.Login)
	public.POST("/register", userHandler.Register)
	public.GET("/check-login", userHandler.CheckLogin)
	public.GET("/is-admin", userHandler.IsAdmin)
	public.GET("/has-username", userHandler.HasUsername)

	// 受保护接口(需要有效 JWT)
	users := authed.Group("/users")
	users.POST("/logout", userHandler.Logout)
	users.GET("/:username", userHandler.GetUserByUsername)
	users.GET("/actual/:username", userHandler.GetUserByUsername)
	users.PUT("", userHandler.Update)
	users.POST("/admin", userHandler.AddAdmin)
	users.GET("/page", userHandler.PageUsers)
}

func setupAgentRoutes(authed *gin.RouterGroup) {
	agentController := handlers.NewAgentController()
	agentFileController := handlers.NewAgentFileController()
	agentPropertiesController := handlers.NewAgentPropertiesController()

	agents := authed.Group("/agents")
	agents.POST("/sessions", agentController.CreateSession)
	agents.POST("/sessions/:sessionId/chat", agentController.Chat)
	agents.GET("/conversations", agentController.PageConversations)
	agents.GET("/conversations/:sessionId/messages", agentController.GetConversationHistory)
	agents.GET("/messages/history", agentController.PageHistoryMessages)
	agents.PUT("/conversations/:sessionId/end", agentController.EndConversation)

	agentFiles := authed.Group("/agents/files")
	agentFiles.POST("/upload", agentFileController.Upload)

	agentProperties := authed.Group("/agent-properties")
	agentProperties.POST("", agentPropertiesController.Create)
	agentProperties.DELETE("/:id", agentPropertiesController.Delete)
	agentProperties.PUT("", agentPropertiesController.Update)
	agentProperties.GET("/byName", agentPropertiesController.GetByName)
	agentProperties.GET("", agentPropertiesController.GetByPage)
}

func setupAiRoutes(authed *gin.RouterGroup) {
	aiConversationController := handlers.NewAiConversationController()
	aiMessageController := handlers.NewAiMessageController()
	aiPropertiesController := handlers.NewAiPropertiesController()

	aiConversations := authed.Group("/ai/conversations")
	aiConversations.POST("", aiConversationController.CreateConversation)
	aiConversations.GET("", aiConversationController.PageConversations)
	aiConversations.PUT("/:sessionId", aiConversationController.UpdateConversation)
	aiConversations.PUT("/:sessionId/end", aiConversationController.EndConversation)
	aiConversations.DELETE("/:sessionId", aiConversationController.DeleteConversation)
	aiConversations.GET("/:sessionId", aiConversationController.GetConversationById)

	aiMessages := authed.Group("/ai")
	aiMessages.POST("/sessions/:sessionId/chat", aiMessageController.Chat)
	aiMessages.POST("/sessions/:sessionId/chat/stream", aiMessageController.ChatStream)
	aiMessages.GET("/history/:sessionId", aiMessageController.GetConversationHistory)
	aiMessages.GET("/history/page", aiMessageController.PageHistoryMessages)
	aiMessages.GET("/memory/threshold", aiMessageController.GetMemoryThreshold)
	aiMessages.PUT("/memory/threshold", aiMessageController.SetMemoryThreshold)

	aiProperties := authed.Group("/ai-properties")
	aiProperties.GET("/options", aiPropertiesController.GetAvailableAiModels)
	aiProperties.GET("/presets", aiPropertiesController.GetPresetModels)
	aiProperties.POST("/preset", aiPropertiesController.CreateFromPreset)
	aiProperties.POST("", aiPropertiesController.CreateAiProperties)
	aiProperties.PUT("", aiPropertiesController.UpdateAiProperties)
	aiProperties.DELETE("/:id", aiPropertiesController.DeleteAiProperties)
	aiProperties.GET("/:id", aiPropertiesController.GetAiPropertiesById)
	aiProperties.GET("", aiPropertiesController.PageAiProperties)
	aiProperties.GET("/enabled", aiPropertiesController.GetAllEnabledAiProperties)
	aiProperties.PUT("/:id/status", aiPropertiesController.ToggleAiPropertiesStatus)
}

func setupInterviewRoutes(authed *gin.RouterGroup) {
	sessionController := handlers.NewInterviewSessionController()
	recordController := handlers.NewInterviewRecordController()
	resumeController := handlers.NewInterviewResumeController()

	interview := authed.Group("/interview")

	interview.POST("/sessions", sessionController.CreateSession)
	interview.GET("/conversations", sessionController.PageConversations)
	interview.GET("/conversations/:sessionId/messages", sessionController.GetConversationHistory)
	interview.GET("/messages/history", sessionController.PageHistoryMessages)
	interview.PUT("/sessions/:sessionId/finish", sessionController.FinishSession)
	interview.PUT("/conversations/:sessionId/end", sessionController.EndConversation)
	interview.POST("/sessions/:sessionId/interview-questions", sessionController.ExtractInterviewQuestions)
	interview.POST("/sessions/:sessionId/resume/upload", sessionController.UploadResume)
	interview.POST("/sessions/:sessionId/interview/answer", sessionController.AnswerInterviewQuestion)
	interview.POST("/sessions/:sessionId/interview/answer-json", sessionController.AnswerInterviewQuestionJson)
	interview.GET("/sessions/:sessionId/next-question", sessionController.GetNextQuestion)
	interview.GET("/sessions/:sessionId/current-question", sessionController.GetCurrentQuestion)
	interview.GET("/sessions/:sessionId/restore", sessionController.RestoreInterviewSession)
	interview.GET("/sessions/:sessionId/interview/questions", sessionController.GetSessionInterviewQuestions)
	interview.GET("/sessions/:sessionId/interview/score", sessionController.GetSessionTotalScore)
	interview.GET("/sessions/:sessionId/interview/suggestions", sessionController.GetSessionInterviewSuggestions)
	interview.GET("/sessions/:sessionId/resume/score", sessionController.GetSessionResumeScore)
	interview.GET("/sessions/:sessionId/radar-chart", sessionController.GetRadarChartData)

	interview.POST("/interview/record", recordController.SaveInterviewRecord)
	interview.GET("/interview/records", recordController.PageInterviewRecords)
	interview.GET("/interview/record/:sessionId", recordController.GetInterviewRecordBySessionId)
	interview.POST("/interview/record/save-from-redis/:sessionId", recordController.SaveInterviewRecordFromRedis)

	interview.GET("/sessions/:sessionId/resume/preview", resumeController.PreviewResume)
}

func setupMediaRoutes(authed *gin.RouterGroup) {
	ttsController := handlers.NewXunfeiTtsController()
	wsController := handlers.NewWebSocketController()

	xunfeiTts := authed.Group("/xunfei/tts")
	xunfeiTts.POST("/tasks", ttsController.CreateTask)
	xunfeiTts.GET("/tasks/:taskId", ttsController.QueryTask)
	xunfeiTts.POST("/synthesize", ttsController.SynthesizeAndWait)

	websocket := authed.Group("/websocket")
	websocket.GET("/user/:userId/status", wsController.CheckUserStatus)
	websocket.POST("/send-message", wsController.SendMessage)
	websocket.POST("/notification/:userId", wsController.SendNotification)
	websocket.POST("/transcription/:userId", wsController.SendTranscriptionResult)
	websocket.POST("/error/:userId", wsController.SendErrorMessage)
}
