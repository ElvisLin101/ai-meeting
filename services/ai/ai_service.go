package ai

import (
	"ai-meeting/clients"
	"ai-meeting/dto"
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"sync"

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
	conversation := models.Conversation{
		SessionID: sessionID, UserID: username, Scene: models.ConversationSceneAI,
		AgentID: aiID, Title: title, Status: models.ConversationStatusActive, MessageCount: 0,
	}
	if err := mysqlrepo.CreateConversation(&conversation); err != nil {
		return nil, err
	}
	return &dto.AiSessionCreateRespDTO{SessionID: sessionID, Title: title}, nil
}

// PageConversations 分页查询用户的AI会话列表
func (s *AiConversationService) PageConversations(username string, req dto.AiConversationPageReqDTO) ([]models.AiConversation, int64, error) {
	offset := (req.Page - 1) * req.Size
	convs, total, err := mysqlrepo.PageConversations(username, models.ConversationSceneAI, offset, req.Size)
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.AiConversation, 0, len(convs))
	for _, c := range convs {
		out = append(out, toAiConversation(c))
	}
	return out, total, nil
}

// UpdateConversation 更新会话信息
func (s *AiConversationService) UpdateConversation(sessionID string, messageCount int, title, username string) error {
	updates := map[string]interface{}{}
	if messageCount > 0 {
		updates["message_count"] = messageCount
	}
	if title != "" {
		updates["title"] = title
	}
	if len(updates) == 0 {
		return nil
	}
	return mysqlrepo.UpdateConversation(sessionID, username, updates)
}

// UpdateConversationMessageCount 仅更新会话消息计数
func (s *AiConversationService) UpdateConversationMessageCount(sessionID, username string, messageCount int) error {
	return mysqlrepo.UpdateConversation(sessionID, username, map[string]interface{}{
		"message_count": messageCount,
	})
}

// EndConversation 结束会话
func (s *AiConversationService) EndConversation(sessionID, username string) error {
	return mysqlrepo.EndConversation(sessionID, username)
}

// DeleteConversation 删除会话
func (s *AiConversationService) DeleteConversation(sessionID, username string) error {
	if err := mysqlrepo.DeleteConversation(sessionID, username); err != nil {
		return err
	}
	return GetAiMemoryService().ClearCompressedContext(sessionID)
}

// GetConversationBySessionId 根据会话ID查询会话
func (s *AiConversationService) GetConversationBySessionId(sessionID, username string) (*models.AiConversation, error) {
	conv, err := mysqlrepo.FindConversationBySessionAndUser(sessionID, username)
	if err != nil || conv == nil {
		return nil, err
	}
	out := toAiConversation(*conv)
	return &out, nil
}

var (
	aiConversationServiceInstance *AiConversationService
	aiConversationServiceOnce     sync.Once
)

// GetAiConversationService 获取AiConversationService单例
func GetAiConversationService() *AiConversationService {
	aiConversationServiceOnce.Do(func() {
		aiConversationServiceInstance = &AiConversationService{}
		logrus.Info("AiConversationService instance created")
	})
	return aiConversationServiceInstance
}

type AiMessageService struct{}

// GetConversationHistory 获取会话历史消息
func (s *AiMessageService) GetConversationHistory(sessionID, username string) ([]models.AiMessage, error) {
	messages, err := mysqlrepo.ListMessagesAsc(sessionID, username)
	if err != nil {
		return nil, err
	}
	out := make([]models.AiMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, toAiMessage(m))
	}
	return out, nil
}

// PageHistoryMessages 分页查询历史消息
func (s *AiMessageService) PageHistoryMessages(sessionID string, page, size int, username string) ([]models.AiMessage, int64, error) {
	messages, total, err := mysqlrepo.PageMessages(sessionID, username, page, size)
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.AiMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, toAiMessage(m))
	}
	return out, total, nil
}

// SaveMessage 保存消息
func (s *AiMessageService) SaveMessage(sessionID, userID, role, content string) error {
	return mysqlrepo.CreateMessage(&models.Message{
		SessionID: sessionID, UserID: userID, Role: role, Content: content,
	})
}

// toAiConversation 统一会话模型转旧响应模型
func toAiConversation(c models.Conversation) models.AiConversation {
	return models.AiConversation{
		SessionID: c.SessionID, UserID: c.UserID, AiID: c.AgentID,
		Title: c.Title, Status: c.Status, MessageCnt: c.MessageCount,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// toAiMessage 统一消息模型转旧响应模型
func toAiMessage(m models.Message) models.AiMessage {
	return models.AiMessage{
		ID: m.ID, SessionID: m.SessionID, UserID: m.UserID, Role: m.Role,
		Content: m.Content, Sequence: int(m.Sequence), CreatedAt: m.CreatedAt,
	}
}

var (
	aiMessageServiceInstance *AiMessageService
	aiMessageServiceOnce     sync.Once
)

// GetAiMessageService 获取AiMessageService单例
func GetAiMessageService() *AiMessageService {
	aiMessageServiceOnce.Do(func() {
		aiMessageServiceInstance = &AiMessageService{}
		logrus.Info("AiMessageService instance created")
	})
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
	if err := mysqlrepo.CreateAiProperties(&prop); err != nil {
		return err
	}
	clients.InvalidateAiPropertyCache()
	return nil
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
	if err := mysqlrepo.UpdateAiProperties(req.ID, updates); err != nil {
		return err
	}
	clients.InvalidateAiPropertyCache()
	return nil
}

// DeleteAiProperties 删除AI配置
func (s *AiPropertiesService) DeleteAiProperties(id uint) error {
	if err := mysqlrepo.DeleteAiProperties(id); err != nil {
		return err
	}
	clients.InvalidateAiPropertyCache()
	return nil
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
	if err := mysqlrepo.ToggleAiPropertiesStatus(id, isEnabled == 1); err != nil {
		return err
	}
	clients.InvalidateAiPropertyCache()
	return nil
}

var (
	aiPropertiesServiceInstance *AiPropertiesService
	aiPropertiesServiceOnce     sync.Once
)

// GetAiPropertiesService 获取AiPropertiesService单例
func GetAiPropertiesService() *AiPropertiesService {
	aiPropertiesServiceOnce.Do(func() {
		aiPropertiesServiceInstance = &AiPropertiesService{}
		logrus.Info("AiPropertiesService instance created")
	})
	return aiPropertiesServiceInstance
}
