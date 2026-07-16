package ai

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	mongorepo "ai-meeting/repositories/mongo"
	mysqlrepo "ai-meeting/repositories/mysql"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type AiConversationService struct{}

// CreateConversationWithTitle 创建AI会话并生成标题
func (s *AiConversationService) CreateConversationWithTitle(username string, aiID uint, firstMessage string) (*dto.AiSessionCreateRespDTO, error) {
	sessionID := uuid.New().String()
	title := "New AI Conversation"
	if len(firstMessage) > 50 {
		title = firstMessage[:50] + "..."
	}
	conversation := models.AiConversation{SessionID: sessionID, UserID: username, AiID: aiID, Title: title, Status: 1, MessageCnt: 0}
	if err := mysqlrepo.CreateAiConversation(&conversation); err != nil {
		return nil, err
	}
	return &dto.AiSessionCreateRespDTO{SessionID: sessionID, Title: title}, nil
}

// PageConversations 分页查询用户的AI会话列表
func (s *AiConversationService) PageConversations(username string, req dto.AiConversationPageReqDTO) ([]models.AiConversation, int64, error) {
	offset := (req.Page - 1) * req.Size
	return mysqlrepo.PageAiConversations(username, offset, req.Size)
}

// UpdateConversation 更新会话信息
func (s *AiConversationService) UpdateConversation(sessionID string, messageCount int, title, username string) error {
	updates := map[string]interface{}{}
	if messageCount > 0 {
		updates["message_cnt"] = messageCount
	}
	if title != "" {
		updates["title"] = title
	}
	return mysqlrepo.UpdateAiConversation(sessionID, username, updates)
}

// UpdateConversationMessageCount 仅更新会话消息计数
func (s *AiConversationService) UpdateConversationMessageCount(sessionID, username string, messageCount int) error {
	updates := map[string]interface{}{
		"message_cnt": messageCount,
		"updated_at":  time.Now(),
	}
	return mysqlrepo.UpdateAiConversation(sessionID, username, updates)
}

// EndConversation 结束会话
func (s *AiConversationService) EndConversation(sessionID, username string) error {
	return mysqlrepo.EndAiConversation(sessionID, username)
}

// DeleteConversation 删除会话
func (s *AiConversationService) DeleteConversation(sessionID, username string) error {
	if err := mysqlrepo.DeleteAiConversation(sessionID, username); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mongorepo.DeleteAiMessagesBySession(ctx, sessionID, username); err != nil {
		return err
	}
	return GetAiMemoryService().ClearCompressedContext(sessionID)
}

// GetConversationBySessionId 根据会话ID查询会话
func (s *AiConversationService) GetConversationBySessionId(sessionID, username string) (*models.AiConversation, error) {
	return mysqlrepo.FindAiConversationBySessionID(sessionID, username)
}

var aiConversationServiceInstance *AiConversationService

// GetAiConversationService 获取AiConversationService单例
func GetAiConversationService() *AiConversationService {
	if aiConversationServiceInstance == nil {
		aiConversationServiceInstance = &AiConversationService{}
	}
	logrus.Info("AiConversationService instance created")
	return aiConversationServiceInstance
}

type AiMessageService struct{}

// GetConversationHistory 获取会话历史消息
func (s *AiMessageService) GetConversationHistory(sessionID, username string) ([]models.AiMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.ListAiMessagesAsc(ctx, sessionID, username)
}

// PageHistoryMessages 分页查询历史消息
func (s *AiMessageService) PageHistoryMessages(sessionID string, page, size int, username string) ([]models.AiMessage, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.PageAiMessages(ctx, sessionID, page, size, username)
}

// SaveMessage 保存消息
func (s *AiMessageService) SaveMessage(sessionID, userID, role, content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := mongorepo.SaveAiMessage(ctx, sessionID, userID, role, content)
	return err
}

var aiMessageServiceInstance *AiMessageService

// GetAiMessageService 获取AiMessageService单例
func GetAiMessageService() *AiMessageService {
	if aiMessageServiceInstance == nil {
		aiMessageServiceInstance = &AiMessageService{}
	}
	logrus.Info("AiMessageService instance created")
	return aiMessageServiceInstance
}

type AiPropertiesService struct{}

// GetAvailableAiModels 获取所有可用的AI模型
func (s *AiPropertiesService) GetAvailableAiModels() ([]models.AiProperties, error) {
	return mysqlrepo.FindAllEnabledAiProperties()
}

// CreateAiProperties 创建AI配置
func (s *AiPropertiesService) CreateAiProperties(req dto.AiPropertiesCreateReqDTO) error {
	prop := models.AiProperties{Name: req.Name, ModelType: req.ModelType, ApiKey: req.ApiKey, ApiSecret: req.ApiSecret, Endpoint: req.Endpoint, Config: req.Config, IsEnabled: true}
	return mysqlrepo.CreateAiProperties(&prop)
}

// UpdateAiProperties 更新AI配置
func (s *AiPropertiesService) UpdateAiProperties(req dto.AiPropertiesUpdateReqDTO) error {
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.ModelType != "" {
		updates["model_type"] = req.ModelType
	}
	if req.ApiKey != "" {
		updates["api_key"] = req.ApiKey
	}
	if req.ApiSecret != "" {
		updates["api_secret"] = req.ApiSecret
	}
	if req.Endpoint != "" {
		updates["endpoint"] = req.Endpoint
	}
	if req.Config != "" {
		updates["config"] = req.Config
	}
	return mysqlrepo.UpdateAiProperties(req.ID, updates)
}

// DeleteAiProperties 删除AI配置
func (s *AiPropertiesService) DeleteAiProperties(id uint) error {
	return mysqlrepo.DeleteAiProperties(id)
}

// GetAiPropertiesById 根据ID查询AI配置
func (s *AiPropertiesService) GetAiPropertiesById(id uint) (*models.AiProperties, error) {
	return mysqlrepo.FindAiPropertiesByID(id)
}

// PageAiProperties 分页查询AI配置
func (s *AiPropertiesService) PageAiProperties(req dto.AiPropertiesPageReqDTO) ([]models.AiProperties, int64, error) {
	offset := (req.Page - 1) * req.Size
	return mysqlrepo.PageAiProperties(offset, req.Size)
}

// GetAllEnabledAiProperties 获取所有启用的AI配置
func (s *AiPropertiesService) GetAllEnabledAiProperties() ([]models.AiProperties, error) {
	return mysqlrepo.FindAllEnabledAiProperties()
}

// ToggleAiPropertiesStatus 切换AI配置启用状态
func (s *AiPropertiesService) ToggleAiPropertiesStatus(id uint, isEnabled int) error {
	return mysqlrepo.ToggleAiPropertiesStatus(id, isEnabled == 1)
}

var aiPropertiesServiceInstance *AiPropertiesService

// GetAiPropertiesService 获取AiPropertiesService单例
func GetAiPropertiesService() *AiPropertiesService {
	if aiPropertiesServiceInstance == nil {
		aiPropertiesServiceInstance = &AiPropertiesService{}
	}
	logrus.Info("AiPropertiesService instance created")
	return aiPropertiesServiceInstance
}
