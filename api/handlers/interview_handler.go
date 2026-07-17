package handlers

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	"ai-meeting/services/interview"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type InterviewSessionController struct {
	sessionFacade *interview.InterviewSessionFacade
}

func NewInterviewSessionController() *InterviewSessionController {
	return &InterviewSessionController{
		sessionFacade: interview.GetInterviewSessionFacade(),
	}
}

func (c *InterviewSessionController) CreateSession(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.CreateSession(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) PageConversations(ctx *gin.Context) {
	var req dto.InterviewConversationPageReqDTO
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversations, total, err := c.sessionFacade.PageConversations(userID.(string), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.InterviewConversationRespDTO
	for _, conv := range conversations {
		resp = append(resp, dto.InterviewConversationRespDTO{
			SessionID:   conv.SessionID,
			Title:       conv.Title,
			MessageCnt:  conv.MessageCnt,
			Status:      conv.Status,
			UpdatedTime: conv.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":  resp,
		"total": total,
	})
}

func (c *InterviewSessionController) GetConversationHistory(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	messages, err := c.sessionFacade.GetConversationHistory(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AgentMessageHistoryRespDTO
	for _, msg := range messages {
		resp = append(resp, toInterviewMessageHistoryResp(msg))
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *InterviewSessionController) PageHistoryMessages(ctx *gin.Context) {
	sessionID := ctx.Query("sessionId")
	current, _ := strconv.Atoi(ctx.DefaultQuery("current", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))

	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	messages, total, err := c.sessionFacade.PageHistoryMessages(sessionID, current, size, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AgentMessageHistoryRespDTO
	for _, msg := range messages {
		resp = append(resp, toInterviewMessageHistoryResp(msg))
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":  resp,
		"total": total,
	})
}

func (c *InterviewSessionController) FinishSession(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := c.sessionFacade.FinishSession(sessionID, userID.(string)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Session finished"})
}

func (c *InterviewSessionController) EndConversation(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := c.sessionFacade.EndConversation(sessionID, userID.(string)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Conversation ended"})
}

func (c *InterviewSessionController) ExtractInterviewQuestions(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")

	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	username, _ := ctx.Get("username")

	result, err := c.sessionFacade.ExtractInterviewQuestions(sessionID, userID.(string), username.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) AnswerInterviewQuestion(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	questionNumber := ctx.Query("questionNumber")
	answerContent := ctx.Query("answerContent")
	requestId := ctx.Query("requestId")

	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	req := dto.InterviewAnswerReqDTO{
		QuestionNumber: questionNumber,
		AnswerContent:  answerContent,
		RequestId:      requestId,
	}

	result, err := c.sessionFacade.AnswerInterviewQuestion(sessionID, req, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) AnswerInterviewQuestionJson(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	var req dto.InterviewAnswerReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.AnswerInterviewQuestion(sessionID, req, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) GetNextQuestion(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.GetNextQuestion(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) GetCurrentQuestion(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.GetCurrentQuestion(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) RestoreInterviewSession(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.RestoreInterviewSession(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) GetSessionInterviewQuestions(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.GetSessionInterviewQuestions(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) GetSessionTotalScore(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.GetSessionTotalScore(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) GetSessionInterviewSuggestions(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.GetSessionInterviewSuggestions(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) GetSessionResumeScore(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.GetSessionResumeScore(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) GetRadarChartData(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.sessionFacade.GetRadarChartData(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *InterviewSessionController) EvaluateDemeanor(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")

	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	username, _ := ctx.Get("username")

	result, err := c.sessionFacade.EvaluateDemeanor(sessionID, userID.(string), username.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

type InterviewRecordController struct {
	recordService *interview.InterviewRecordService
}

func NewInterviewRecordController() *InterviewRecordController {
	return &InterviewRecordController{
		recordService: interview.GetInterviewRecordService(),
	}
}

func (c *InterviewRecordController) SaveInterviewRecord(ctx *gin.Context) {
	var req dto.InterviewRecordSaveReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := c.recordService.SaveInterviewRecord(req.SessionID, userID.(string), req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Save success"})
}

func (c *InterviewRecordController) PageInterviewRecords(ctx *gin.Context) {
	var req dto.InterviewRecordPageReqDTO
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	records, total, err := c.recordService.PageInterviewRecords(userID.(string), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.InterviewRecordRespDTO
	for _, record := range records {
		resp = append(resp, toInterviewRecordResp(record))
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":  resp,
		"total": total,
	})
}

func (c *InterviewRecordController) GetInterviewRecordBySessionId(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	record, err := c.recordService.GetBySessionId(sessionID, userID.(string))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}

	ctx.JSON(http.StatusOK, toInterviewRecordResp(*record))
}

func (c *InterviewRecordController) SaveInterviewRecordFromRedis(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	userID, exists := ctx.Get("user_id")
	if !exists || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := c.recordService.SaveInterviewRecordFromRedis(sessionID, userID.(string)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Save success"})
}

type InterviewResumeController struct{}

func NewInterviewResumeController() *InterviewResumeController {
	return &InterviewResumeController{}
}

func (c *InterviewResumeController) PreviewResume(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	_ = sessionID
	ctx.JSON(http.StatusOK, gin.H{"message": "Resume preview endpoint"})
}

func toInterviewMessageHistoryResp(msg models.AgentMessage) dto.AgentMessageHistoryRespDTO {
	resp := dto.AgentMessageHistoryRespDTO{
		SessionID: msg.SessionID,
		Role:      msg.Role,
		Content:   msg.Content,
		Sequence:  msg.Sequence,
		CreatedAt: msg.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if !msg.MongoID.IsZero() {
		resp.MessageID = msg.MongoID.Hex()
	}
	return resp
}

func toInterviewRecordResp(record models.InterviewRecord) dto.InterviewRecordRespDTO {
	resp := dto.InterviewRecordRespDTO{
		SessionID:   record.SessionID,
		QuestionNum: record.QuestionNum,
		Question:    record.Question,
		Answer:      record.Answer,
		Score:       record.Score,
		Suggestions: record.Suggestions,
	}
	if !record.MongoID.IsZero() {
		resp.RecordID = record.MongoID.Hex()
	}
	return resp
}
