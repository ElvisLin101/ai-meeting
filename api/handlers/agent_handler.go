package handlers

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	agent "ai-meeting/services/agent"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type AgentController struct {
	agentConversationService *agent.AgentConversationService
	agentMessageService      *agent.AgentMessageService
}

func NewAgentController() *AgentController {
	return &AgentController{
		agentConversationService: agent.GetAgentConversationService(),
		agentMessageService:      agent.GetAgentMessageService(),
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

	// 设置 SSE 响应头
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	// 调用 Agent Chat SSE，通过 onChunk 回调实时推送 chunk 给前端
	_, err := c.agentMessageService.AgentChatSSE(sessionID, username.(string), req.Content, func(chunk string) {
		ctx.SSEvent("message", chunk)
		ctx.Writer.Flush()
	})

	if err != nil {
		// SSE 模式下错误也通过 event 推送
		ctx.SSEvent("error", err.Error())
		ctx.Writer.Flush()
		logrus.Errorf("Agent chat SSE failed, session=%s, err=%v", sessionID, err)
	}

	// 发送结束标记
	ctx.SSEvent("end", "[DONE]")
	ctx.Writer.Flush()
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
	fileAssetService *agent.AgentFileAssetService
}

func NewAgentFileController() *AgentFileController {
	return &AgentFileController{
		fileAssetService: agent.GetAgentFileAssetService(),
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
	propertiesService *agent.AgentPropertiesService
}

func NewAgentPropertiesController() *AgentPropertiesController {
	return &AgentPropertiesController{
		propertiesService: agent.GetAgentPropertiesService(),
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
