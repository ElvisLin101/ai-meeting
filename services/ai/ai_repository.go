package ai

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
)

// aiRepository 是 AI 会话/消息/模型配置服务需要的最小存储接口。
// 生产实现走 MySQL；测试可注入内存 fake。
type aiRepository interface {
	CreateConversation(conv *models.Conversation) error
	PageConversations(userID, scene string, offset, limit int) ([]models.Conversation, int64, error)
	UpdateConversation(sessionID, userID string, updates map[string]interface{}) error
	EndConversation(sessionID, userID string) error
	DeleteConversation(sessionID, userID string) error
	FindConversationBySessionAndUser(sessionID, userID string) (*models.Conversation, error)

	ListMessagesAsc(sessionID, userID string) ([]models.Message, error)
	PageMessages(sessionID, userID string, page, size int) ([]models.Message, int64, error)
	CreateMessage(msg *models.Message) error
	CountMessages(sessionID, userID string) (int64, error)

	FindAllEnabledAiProperties() ([]models.AiProperties, error)
	CreateAiProperties(prop *models.AiProperties) error
	UpdateAiProperties(id uint, updates map[string]interface{}) error
	DeleteAiProperties(id uint) error
	FindAiPropertiesByID(id uint) (*models.AiProperties, error)
	PageAiProperties(offset, limit int) ([]models.AiProperties, int64, error)
	ToggleAiPropertiesStatus(id uint, isEnabled bool) error
}

type mysqlAIRepository struct{}

// aiMemoryRepository 是 AI 记忆压缩服务需要的最小存储接口。
type aiMemoryRepository interface {
	ListMessagesAfterSequenceDesc(sessionID, userID string, after int64) ([]models.Message, error)
	ListMessagesDesc(sessionID, userID string) ([]models.Message, error)
	UpsertCompressedContext(record *models.CompressedContextRecord) error
	FindCompressedContext(sessionID string) (*models.CompressedContextRecord, error)
	DeleteCompressedContext(sessionID string) error
}

func (mysqlAIRepository) CreateConversation(conv *models.Conversation) error {
	return mysqlrepo.CreateConversation(conv)
}

func (mysqlAIRepository) PageConversations(userID, scene string, offset, limit int) ([]models.Conversation, int64, error) {
	return mysqlrepo.PageConversations(userID, scene, offset, limit)
}

func (mysqlAIRepository) UpdateConversation(sessionID, userID string, updates map[string]interface{}) error {
	return mysqlrepo.UpdateConversation(sessionID, userID, updates)
}

func (mysqlAIRepository) EndConversation(sessionID, userID string) error {
	return mysqlrepo.EndConversation(sessionID, userID)
}

func (mysqlAIRepository) DeleteConversation(sessionID, userID string) error {
	return mysqlrepo.DeleteConversation(sessionID, userID)
}

func (mysqlAIRepository) FindConversationBySessionAndUser(sessionID, userID string) (*models.Conversation, error) {
	return mysqlrepo.FindConversationBySessionAndUser(sessionID, userID)
}

func (mysqlAIRepository) ListMessagesAsc(sessionID, userID string) ([]models.Message, error) {
	return mysqlrepo.ListMessagesAsc(sessionID, userID)
}

func (mysqlAIRepository) PageMessages(sessionID, userID string, page, size int) ([]models.Message, int64, error) {
	return mysqlrepo.PageMessages(sessionID, userID, page, size)
}

func (mysqlAIRepository) CreateMessage(msg *models.Message) error {
	return mysqlrepo.CreateMessage(msg)
}

func (mysqlAIRepository) CountMessages(sessionID, userID string) (int64, error) {
	return mysqlrepo.CountMessages(sessionID, userID)
}

func (mysqlAIRepository) FindAllEnabledAiProperties() ([]models.AiProperties, error) {
	return mysqlrepo.FindAllEnabledAiProperties()
}

func (mysqlAIRepository) CreateAiProperties(prop *models.AiProperties) error {
	return mysqlrepo.CreateAiProperties(prop)
}

func (mysqlAIRepository) UpdateAiProperties(id uint, updates map[string]interface{}) error {
	return mysqlrepo.UpdateAiProperties(id, updates)
}

func (mysqlAIRepository) DeleteAiProperties(id uint) error {
	return mysqlrepo.DeleteAiProperties(id)
}

func (mysqlAIRepository) FindAiPropertiesByID(id uint) (*models.AiProperties, error) {
	return mysqlrepo.FindAiPropertiesByID(id)
}

func (mysqlAIRepository) PageAiProperties(offset, limit int) ([]models.AiProperties, int64, error) {
	return mysqlrepo.PageAiProperties(offset, limit)
}

func (mysqlAIRepository) ToggleAiPropertiesStatus(id uint, isEnabled bool) error {
	return mysqlrepo.ToggleAiPropertiesStatus(id, isEnabled)
}

func (mysqlAIRepository) ListMessagesAfterSequenceDesc(sessionID, userID string, after int64) ([]models.Message, error) {
	return mysqlrepo.ListMessagesAfterSequenceDesc(sessionID, userID, after)
}

func (mysqlAIRepository) ListMessagesDesc(sessionID, userID string) ([]models.Message, error) {
	return mysqlrepo.ListMessagesDesc(sessionID, userID)
}

func (mysqlAIRepository) UpsertCompressedContext(record *models.CompressedContextRecord) error {
	return mysqlrepo.UpsertCompressedContext(record)
}

func (mysqlAIRepository) FindCompressedContext(sessionID string) (*models.CompressedContextRecord, error) {
	return mysqlrepo.FindCompressedContext(sessionID)
}

func (mysqlAIRepository) DeleteCompressedContext(sessionID string) error {
	return mysqlrepo.DeleteCompressedContext(sessionID)
}
