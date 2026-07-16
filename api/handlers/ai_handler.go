package handlers

import (
	"ai-meeting/clients"
	"ai-meeting/dto"
	"ai-meeting/services/ai"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type AiConversationController struct {
	conversationService *ai.AiConversationService
}

func NewAiConversationController() *AiConversationController {
	return &AiConversationController{
		conversationService: ai.GetAiConversationService(),
	}
}

func (c *AiConversationController) CreateConversation(ctx *gin.Context) {
	var req dto.AiSessionCreateReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := c.conversationService.CreateConversationWithTitle(
		username.(string),
		req.AiId,
		req.FirstMessage,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *AiConversationController) PageConversations(ctx *gin.Context) {
	var req dto.AiConversationPageReqDTO
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversations, total, err := c.conversationService.PageConversations(username.(string), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AiConversationRespDTO
	for _, conv := range conversations {
		resp = append(resp, dto.AiConversationRespDTO{
			SessionID:   conv.SessionID,
			AiId:        conv.AiID,
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

func (c *AiConversationController) UpdateConversation(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	messageCount, _ := strconv.Atoi(ctx.Query("messageCount"))
	title := ctx.Query("title")

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := c.conversationService.UpdateConversation(sessionID, messageCount, title, username.(string)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Update success"})
}

func (c *AiConversationController) EndConversation(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := c.conversationService.EndConversation(sessionID, username.(string)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Conversation ended"})
}

func (c *AiConversationController) DeleteConversation(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := c.conversationService.DeleteConversation(sessionID, username.(string)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Delete success"})
}

func (c *AiConversationController) GetConversationById(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conv, err := c.conversationService.GetConversationBySessionId(sessionID, username.(string))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}

	ctx.JSON(http.StatusOK, dto.AiConversationRespDTO{
		SessionID:   conv.SessionID,
		AiId:        conv.AiID,
		Title:       conv.Title,
		MessageCnt:  conv.MessageCnt,
		Status:      conv.Status,
		UpdatedTime: conv.UpdatedAt.Format("2006-01-02 15:04:05"),
	})
}

type AiMessageController struct {
	messageService *ai.AiMessageService
	memoryService  *ai.AiMemoryService
}

func NewAiMessageController() *AiMessageController {
	return &AiMessageController{
		messageService: ai.GetAiMessageService(),
		memoryService:  ai.GetAiMemoryService(),
	}
}

func (c *AiMessageController) Chat(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	var req dto.AiMessageReqDTO
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

	resp, err := c.messageService.Chat(ctx.Request.Context(), sessionID, username.(string), req.Content)
	if err != nil {
		if errors.Is(err, ai.ErrEmptyAiMessageContent) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, ai.ErrAiConversationNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *AiMessageController) ChatStream(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	var req dto.AiMessageReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": ai.ErrEmptyAiMessageContent.Error()})
		return
	}

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	ctx.Status(http.StatusOK)
	ctx.Writer.Flush()

	resp, err := c.messageService.ChatStream(ctx.Request.Context(), sessionID, username.(string), req.Content, func(chunk ai.AiChatStreamChunk) error {
		if err := ctx.Request.Context().Err(); err != nil {
			return err
		}
		if chunk.ReasoningContent != "" {
			ctx.SSEvent("reasoning", gin.H{"content": chunk.ReasoningContent})
		}
		if chunk.Content != "" {
			ctx.SSEvent("message", gin.H{"content": chunk.Content})
		}
		ctx.Writer.Flush()
		return ctx.Request.Context().Err()
	})
	if err != nil {
		ctx.SSEvent("error", gin.H{"error": err.Error()})
		ctx.Writer.Flush()
		return
	}

	ctx.SSEvent("done", gin.H{
		"session_id":           resp.SessionID,
		"user_message_id":      resp.UserMessageID,
		"assistant_message_id": resp.AssistantMessageID,
	})
	ctx.Writer.Flush()
}

func (c *AiMessageController) GetConversationHistory(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	messages, err := c.messageService.GetConversationHistory(sessionID, username.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AiMessageHistoryRespDTO
	for _, msg := range messages {
		messageID := ""
		if !msg.MongoID.IsZero() {
			messageID = msg.MongoID.Hex()
		}
		resp = append(resp, dto.AiMessageHistoryRespDTO{
			ID:        msg.ID,
			MessageID: messageID,
			SessionID: msg.SessionID,
			Role:      msg.Role,
			Content:   msg.Content,
			Sequence:  msg.Sequence,
			CreatedAt: msg.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *AiMessageController) PageHistoryMessages(ctx *gin.Context) {
	sessionID := ctx.Query("sessionId")
	current, _ := strconv.Atoi(ctx.DefaultQuery("current", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))

	username, exists := ctx.Get("username")
	if !exists || username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	messages, total, err := c.messageService.PageHistoryMessages(sessionID, current, size, username.(string))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AiMessageHistoryRespDTO
	for _, msg := range messages {
		messageID := ""
		if !msg.MongoID.IsZero() {
			messageID = msg.MongoID.Hex()
		}
		resp = append(resp, dto.AiMessageHistoryRespDTO{
			ID:        msg.ID,
			MessageID: messageID,
			SessionID: msg.SessionID,
			Role:      msg.Role,
			Content:   msg.Content,
			Sequence:  msg.Sequence,
			CreatedAt: msg.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":  resp,
		"total": total,
	})
}

func (c *AiMessageController) GetMemoryThreshold(ctx *gin.Context) {
	threshold, minThreshold, maxThreshold, triggerOffset := c.memoryService.GetCompressionThresholdConfig()
	ctx.JSON(http.StatusOK, dto.MemoryThresholdRespDTO{
		Threshold:     threshold,
		MinThreshold:  minThreshold,
		MaxThreshold:  maxThreshold,
		TriggerOffset: triggerOffset,
	})
}

func (c *AiMessageController) SetMemoryThreshold(ctx *gin.Context) {
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

type AiPropertiesController struct {
	propertiesService *ai.AiPropertiesService
}

func NewAiPropertiesController() *AiPropertiesController {
	return &AiPropertiesController{
		propertiesService: ai.GetAiPropertiesService(),
	}
}

func (c *AiPropertiesController) GetAvailableAiModels(ctx *gin.Context) {
	props, err := c.propertiesService.GetAvailableAiModels()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AiModelOptionRespDTO
	for _, prop := range props {
		resp = append(resp, dto.AiModelOptionRespDTO{
			ID:   prop.ID,
			Name: prop.Name,
		})
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *AiPropertiesController) CreateAiProperties(ctx *gin.Context) {
	var req dto.AiPropertiesCreateReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.propertiesService.CreateAiProperties(req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Create success"})
}

// GetPresetModels 返回预设模型模板列表
func (c *AiPropertiesController) GetPresetModels(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, clients.PresetModels)
}

// CreateFromPreset 按预设模板创建 AI 配置（用户只需填 apiKey）
func (c *AiPropertiesController) CreateFromPreset(ctx *gin.Context) {
	var req dto.AiPropertiesCreateFromPresetReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 查找预设模板
	preset := clients.GetPresetByProvider(req.Provider)
	if preset == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "不支持的 provider: " + req.Provider})
		return
	}

	// 用户没填的用预设默认值填充
	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = preset.Endpoint
	}
	modelType := req.ModelType
	if modelType == "" {
		modelType = preset.ModelType
	}
	config := req.Config
	if config == "" {
		config = preset.ConfigHint
	}

	createReq := dto.AiPropertiesCreateReqDTO{
		Name:      req.Name,
		ModelType: modelType,
		ApiKey:    req.ApiKey,
		ApiSecret: req.ApiSecret,
		Endpoint:  endpoint,
		Config:    config,
	}

	if err := c.propertiesService.CreateAiProperties(createReq); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Create success"})
}

func (c *AiPropertiesController) UpdateAiProperties(ctx *gin.Context) {
	var req dto.AiPropertiesUpdateReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.propertiesService.UpdateAiProperties(req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Update success"})
}

func (c *AiPropertiesController) DeleteAiProperties(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if err := c.propertiesService.DeleteAiProperties(uint(id)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Delete success"})
}

func (c *AiPropertiesController) GetAiPropertiesById(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	prop, err := c.propertiesService.GetAiPropertiesById(uint(id))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}

	ctx.JSON(http.StatusOK, dto.AiPropertiesRespDTO{
		ID:        prop.ID,
		Name:      prop.Name,
		ModelType: prop.ModelType,
		Endpoint:  prop.Endpoint,
		IsEnabled: prop.IsEnabled,
		CreatedAt: prop.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

func (c *AiPropertiesController) PageAiProperties(ctx *gin.Context) {
	var req dto.AiPropertiesPageReqDTO
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	props, total, err := c.propertiesService.PageAiProperties(req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AiPropertiesRespDTO
	for _, prop := range props {
		resp = append(resp, dto.AiPropertiesRespDTO{
			ID:        prop.ID,
			Name:      prop.Name,
			ModelType: prop.ModelType,
			Endpoint:  prop.Endpoint,
			IsEnabled: prop.IsEnabled,
			CreatedAt: prop.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":  resp,
		"total": total,
	})
}

func (c *AiPropertiesController) GetAllEnabledAiProperties(ctx *gin.Context) {
	props, err := c.propertiesService.GetAllEnabledAiProperties()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.AiPropertiesRespDTO
	for _, prop := range props {
		resp = append(resp, dto.AiPropertiesRespDTO{
			ID:        prop.ID,
			Name:      prop.Name,
			ModelType: prop.ModelType,
			Endpoint:  prop.Endpoint,
			IsEnabled: prop.IsEnabled,
			CreatedAt: prop.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *AiPropertiesController) ToggleAiPropertiesStatus(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	isEnabled, err := strconv.Atoi(ctx.Query("isEnabled"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid isEnabled"})
		return
	}

	if err := c.propertiesService.ToggleAiPropertiesStatus(uint(id), isEnabled); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}
