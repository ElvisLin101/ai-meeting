package handlers

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	"ai-meeting/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type AgentController struct {
	agentConversationService *services.AgentConversationService
	agentMessageService      *services.AgentMessageService
	memoryService            *services.MemoryService
}

func NewAgentController() *AgentController {
	return &AgentController{
		agentConversationService: services.GetAgentConversationService(),
		agentMessageService:      services.GetAgentMessageService(),
		memoryService:            services.GetMemoryService(),
	}
}

func (c *AgentController) CreateSession(ctx *gin.Context) {
	var req dto.AgentSessionCreateReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.agentConversationService.CreateConversationWithTitle(
		username.(string),
		"1",
		req.FirstMessage,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *AgentController) Chat(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	var req dto.UserMessageReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	req.SessionID = sessionID
	req.UserName = username.(string)

	if err := c.agentMessageService.SaveMessage(sessionID, username.(string), "user", req.Content); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go func() {
		if _, err := c.agentMessageService.GetConversationHistoryWithContext(sessionID, username.(string)); err != nil {
			logrus.Warnf("Failed to build memory context, session=%s, err=%v", sessionID, err)
		}
	}()

	ctx.JSON(http.StatusOK, gin.H{"message": "Message received"})
}

func (c *AgentController) PageConversations(ctx *gin.Context) {
	var req dto.AgentConversationPageReqDTO
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversations, total, err := c.agentConversationService.PageConversations(username.(string), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AgentConversationRespDTO
	for _, conv := range conversations {
		resp = append(resp, dto.AgentConversationRespDTO{
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

func (c *AgentController) GetConversationHistory(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	messages, err := c.agentMessageService.GetConversationHistory(sessionID, username.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AgentMessageHistoryRespDTO
	for _, msg := range messages {
		resp = append(resp, toAgentMessageHistoryResp(msg))
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *AgentController) PageHistoryMessages(ctx *gin.Context) {
	sessionID := ctx.Query("sessionId")
	current := ctx.DefaultQuery("current", "1")
	size := ctx.DefaultQuery("size", "10")

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	page, _ := strconv.Atoi(current)
	pageSize, _ := strconv.Atoi(size)

	messages, total, err := c.agentMessageService.PageHistoryMessages(sessionID, page, pageSize, username.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AgentMessageHistoryRespDTO
	for _, msg := range messages {
		resp = append(resp, toAgentMessageHistoryResp(msg))
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":  resp,
		"total": total,
	})
}

func (c *AgentController) EndConversation(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := c.agentConversationService.EndConversation(sessionID, username.(string)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Conversation ended"})
}

func (c *AgentController) GetMemoryThreshold(ctx *gin.Context) {
	if !hasUsername(ctx) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	threshold, minThreshold, maxThreshold, triggerOffset := c.memoryService.GetCompressionThresholdConfig()
	ctx.JSON(http.StatusOK, dto.MemoryThresholdRespDTO{
		Threshold:     threshold,
		MinThreshold:  minThreshold,
		MaxThreshold:  maxThreshold,
		TriggerOffset: triggerOffset,
	})
}

func (c *AgentController) SetMemoryThreshold(ctx *gin.Context) {
	if !hasUsername(ctx) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req dto.MemoryThresholdReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := c.memoryService.SetCompressionThreshold(req.Threshold); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	threshold, minThreshold, maxThreshold, triggerOffset := c.memoryService.GetCompressionThresholdConfig()
	ctx.JSON(http.StatusOK, dto.MemoryThresholdRespDTO{
		Threshold:     threshold,
		MinThreshold:  minThreshold,
		MaxThreshold:  maxThreshold,
		TriggerOffset: triggerOffset,
	})
}

func hasUsername(ctx *gin.Context) bool {
	username, exists := ctx.Get("username")
	return exists && username != ""
}

func toAgentMessageHistoryResp(msg models.AgentMessage) dto.AgentMessageHistoryRespDTO {
	resp := dto.AgentMessageHistoryRespDTO{
		ID:        msg.ID,
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

type AgentFileController struct {
	fileAssetService *services.AgentFileAssetService
}

func NewAgentFileController() *AgentFileController {
	return &AgentFileController{
		fileAssetService: services.GetAgentFileAssetService(),
	}
}

func (c *AgentFileController) Upload(ctx *gin.Context) {
	sessionID := ctx.PostForm("sessionId")
	bizType := ctx.PostForm("bizType")
	file, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	savePath := "./uploads/" + file.Filename
	if err := ctx.SaveUploadedFile(file, savePath); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := c.fileAssetService.UploadAndPersist(sessionID, bizType, username.(string), file.Filename, savePath, file.Size)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

type AgentPropertiesController struct {
	propertiesService *services.AgentPropertiesService
}

func NewAgentPropertiesController() *AgentPropertiesController {
	return &AgentPropertiesController{
		propertiesService: services.GetAgentPropertiesService(),
	}
}

func (c *AgentPropertiesController) Create(ctx *gin.Context) {
	var req dto.AgentPropertiesReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.propertiesService.Create(req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Create success"})
}

func (c *AgentPropertiesController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if err := c.propertiesService.Delete(uint(id)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Delete success"})
}

func (c *AgentPropertiesController) Update(ctx *gin.Context) {
	var req dto.AgentPropertiesReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.propertiesService.Update(req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Update success"})
}

func (c *AgentPropertiesController) GetByName(ctx *gin.Context) {
	name := ctx.Query("name")
	prop, err := c.propertiesService.GetByName(name)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}

	ctx.JSON(http.StatusOK, dto.AgentPropertiesRespDTO{
		ID:          prop.ID,
		Name:        prop.Name,
		Description: prop.Description,
		Config:      prop.Config,
		IsEnabled:   prop.IsEnabled,
		CreatedAt:   prop.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

func (c *AgentPropertiesController) GetByPage(ctx *gin.Context) {
	var req dto.AgentPropertiesReqDTO
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	props, total, err := c.propertiesService.GetByPage(req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AgentPropertiesRespDTO
	for _, prop := range props {
		resp = append(resp, dto.AgentPropertiesRespDTO{
			ID:          prop.ID,
			Name:        prop.Name,
			Description: prop.Description,
			Config:      prop.Config,
			IsEnabled:   prop.IsEnabled,
			CreatedAt:   prop.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":  resp,
		"total": total,
	})
}
