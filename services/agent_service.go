package services

import (
	"ai-meeting/clients"
	"ai-meeting/dto"
	"ai-meeting/models"
	mongorepo "ai-meeting/repositories/mongo"
	mysqlrepo "ai-meeting/repositories/mysql"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type AgentConversationService struct{}

// CreateConversationWithTitle 创建Agent会话并生成标题
func (s *AgentConversationService) CreateConversationWithTitle(username, agentID string, firstMessage string) (*dto.AgentSessionCreateRespDTO, error) {
	sessionID := uuid.New().String()
	title := "New Conversation"
	if len(firstMessage) > 50 {
		title = firstMessage[:50] + "..."
	}
	conversation := models.AgentConversation{SessionID: sessionID, UserID: username, AgentID: 1, Title: title, Status: 1, MessageCnt: 0}
	if err := mysqlrepo.CreateAgentConversation(&conversation); err != nil {
		return nil, err
	}
	return &dto.AgentSessionCreateRespDTO{SessionID: sessionID, Title: title}, nil
}

// PageConversations 分页查询用户的Agent会话列表
func (s *AgentConversationService) PageConversations(username string, req dto.AgentConversationPageReqDTO) ([]models.AgentConversation, int64, error) {
	offset := (req.Page - 1) * req.Size
	return mysqlrepo.PageAgentConversations(username, offset, req.Size)
}

// EndConversation 结束会话
func (s *AgentConversationService) EndConversation(sessionID, userID string) error {
	return mysqlrepo.EndAgentConversation(sessionID, userID)
}

var agentConversationServiceInstance *AgentConversationService

// GetAgentConversationService 获取AgentConversationService单例
func GetAgentConversationService() *AgentConversationService {
	if agentConversationServiceInstance == nil {
		agentConversationServiceInstance = &AgentConversationService{}
	}
	logrus.Info("AgentConversationService instance created")
	return agentConversationServiceInstance
}

type AgentMessageService struct{}

// GetConversationHistory 获取会话历史消息
func (s *AgentMessageService) GetConversationHistory(sessionID, userID string) ([]models.AgentMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return mongorepo.ListAgentMessagesAsc(ctx, sessionID, userID)
}

// PageHistoryMessages 分页查询历史消息
func (s *AgentMessageService) PageHistoryMessages(sessionID string, page, size int, userID string) ([]models.AgentMessage, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return mongorepo.PageAgentMessages(ctx, sessionID, page, size, userID)
}

// SaveMessage 保存消息
func (s *AgentMessageService) SaveMessage(sessionID, userID, role, content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return mongorepo.SaveAgentMessage(ctx, sessionID, userID, role, content)
}

// GetConversationHistoryWithContext 获取带上下文的历史消息
func (s *AgentMessageService) GetConversationHistoryWithContext(sessionID, userID string) (string, error) {
	memoryService := GetMemoryService()
	threshold := memoryService.GetCompressionThreshold()

	// 获取上下文
	context, err := memoryService.GetContext(sessionID, userID, threshold)
	if err != nil {
		return "", err
	}

	return context, nil
}

// AgentChatSSE Agent SSE 流式聊天
// onChunk 回调用于实时推送 chunk 给前端
// 返回完整回复内容
func (s *AgentMessageService) AgentChatSSE(sessionID, userID, content string, onChunk func(chunk string)) (string, error) {
	// 1. 会话归属校验
	conv, err := mysqlrepo.GetAgentConversationBySessionId(sessionID, userID)
	if err != nil || conv == nil {
		return "", fmt.Errorf("会话不存在或无权限: %w", err)
	}

	// 2. 解析智能体配置
	agentProps := GetAgentPropertiesLoader().GetByAgentID(conv.AgentID)
	if agentProps == nil {
		return "", fmt.Errorf("智能体配置不存在, agentID=%d", conv.AgentID)
	}
	if agentProps.ApiKey == "" || agentProps.ApiSecret == "" || agentProps.ApiFlowId == "" {
		return "", fmt.Errorf("智能体配置不完整, 缺少 apiKey/apiSecret/apiFlowId")
	}

	// 3. 保存用户消息
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mongorepo.SaveAgentMessage(ctx, sessionID, userID, "user", content); err != nil {
		return "", fmt.Errorf("保存用户消息失败: %w", err)
	}

	// 4. 加载历史消息构建 history
	historyCtx, historyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer historyCancel()
	messages, err := mongorepo.ListAgentMessagesAsc(historyCtx, sessionID, userID)
	if err != nil {
		return "", fmt.Errorf("加载历史消息失败: %w", err)
	}
	history := buildXingChenHistory(messages)

	// 5. 调用讯飞星辰工作流流式聊天
	startTime := time.Now()
	xingChenClient := clients.GetXingChenClient()
	fullContent, err := xingChenClient.ChatStream(
		content, sessionID, history,
		agentProps.ApiFlowId, agentProps.ApiKey, agentProps.ApiSecret,
		onChunk,
	)
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		// 6a. 出错也保存一条错误 assistant 消息
		errorMsg := ""
		if err != nil {
			errorMsg = err.Error()
		}
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer saveCancel()
		mongorepo.SaveAgentMessageWithDetail(saveCtx, sessionID, userID, "assistant", "Sorry, an error occurred while processing your request.", responseTime, errorMsg)
		logrus.Errorf("Agent chat failed, session=%s, err=%v", sessionID, err)
		return "", fmt.Errorf("调用工作流失败: %w", err)
	}

	// 6b. 保存 assistant 回复
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	assistantSeq, err := mongorepo.SaveAgentMessageWithDetail(saveCtx, sessionID, userID, "assistant", fullContent, responseTime, "")
	if err != nil {
		logrus.Errorf("Failed to save assistant message, session=%s, err=%v", sessionID, err)
	}

	// 7. 更新会话消息计数
	if err := mysqlrepo.UpdateAgentConversationMessageCount(sessionID, assistantSeq); err != nil {
		logrus.Errorf("Failed to update conversation message count, session=%s, err=%v", sessionID, err)
	}

	// 8. 异步触发记忆压缩
	go func() {
		memoryService := GetMemoryService()
		threshold := memoryService.GetCompressionThreshold()
		memoryService.CompressContext(sessionID, userID, threshold)
	}()

	logrus.Infof("Agent chat completed, session=%s, responseTime=%dms, contentLen=%d",
		sessionID, responseTime, len(fullContent))
	return fullContent, nil
}

// buildXingChenHistory 将历史消息转为讯飞星辰需要的格式
func buildXingChenHistory(messages []models.AgentMessage) []clients.XingChenHistoryItem {
	// 排除最后一条（当前用户消息，已经作为 input 传入）
	if len(messages) > 0 {
		messages = messages[:len(messages)-1]
	}

	history := make([]clients.XingChenHistoryItem, 0, len(messages))
	for _, msg := range messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "assistant"
		}
		history = append(history, clients.XingChenHistoryItem{
			Role:        role,
			ContentType: "text",
			Content:     msg.Content,
		})
	}
	return history
}

var agentMessageServiceInstance *AgentMessageService

// GetAgentMessageService 获取AgentMessageService单例
func GetAgentMessageService() *AgentMessageService {
	if agentMessageServiceInstance == nil {
		agentMessageServiceInstance = &AgentMessageService{}
	}
	logrus.Info("AgentMessageService instance created")
	return agentMessageServiceInstance
}

type AgentPropertiesService struct{}

// Create 创建Agent配置
func (s *AgentPropertiesService) Create(req dto.AgentPropertiesReqDTO) error {
	prop := models.AgentProperties{Name: req.Name, Description: req.Description, Config: req.Config, IsEnabled: true}
	return mysqlrepo.CreateAgentProperties(&prop)
}

// Update 更新Agent配置
func (s *AgentPropertiesService) Update(req dto.AgentPropertiesReqDTO) error {
	return mysqlrepo.UpdateAgentProperties(req.ID, map[string]interface{}{"name": req.Name, "description": req.Description, "config": req.Config})
}

// Delete 删除Agent配置
func (s *AgentPropertiesService) Delete(id uint) error {
	return mysqlrepo.DeleteAgentProperties(id)
}

// GetByName 根据名称查询Agent配置
func (s *AgentPropertiesService) GetByName(name string) (*models.AgentProperties, error) {
	return mysqlrepo.FindAgentPropertiesByName(name)
}

// GetByPage 分页查询Agent配置
func (s *AgentPropertiesService) GetByPage(req dto.AgentPropertiesReqDTO) ([]models.AgentProperties, int64, error) {
	return mysqlrepo.PageAgentProperties(10)
}

var agentPropertiesServiceInstance *AgentPropertiesService

// GetAgentPropertiesService 获取AgentPropertiesService单例
func GetAgentPropertiesService() *AgentPropertiesService {
	if agentPropertiesServiceInstance == nil {
		agentPropertiesServiceInstance = &AgentPropertiesService{}
	}
	logrus.Info("AgentPropertiesService instance created")
	return agentPropertiesServiceInstance
}

type AgentFileAssetService struct{}

// UploadAndPersist 上传文件并持久化记录
func (s *AgentFileAssetService) UploadAndPersist(sessionID, bizType, username string, filename, path string, size int64) (*dto.AgentFileUploadRespDTO, error) {
	asset := models.AgentFileAsset{SessionID: sessionID, UserID: username, Filename: filename, Path: path, BizType: bizType, Size: size}
	if err := mysqlrepo.CreateAgentFileAsset(&asset); err != nil {
		return nil, err
	}
	return &dto.AgentFileUploadRespDTO{FileID: fmt.Sprintf("%d", asset.ID), Filename: filename, Path: path}, nil
}

var agentFileAssetServiceInstance *AgentFileAssetService

// GetAgentFileAssetService 获取AgentFileAssetService单例
func GetAgentFileAssetService() *AgentFileAssetService {
	if agentFileAssetServiceInstance == nil {
		agentFileAssetServiceInstance = &AgentFileAssetService{}
	}
	logrus.Info("AgentFileAssetService instance created")
	return agentFileAssetServiceInstance
}
