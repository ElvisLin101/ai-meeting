package agent

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
)

// agentRepository 是 AgentChat / Agent 配置 / 会话消息服务的最小存储接口。
type agentRepository interface {
	CreateConversation(conv *models.Conversation) error
	PageConversations(userID, scene string, offset, limit int) ([]models.Conversation, int64, error)
	EndConversation(sessionID, userID string) error
	FindConversationBySessionAndUser(sessionID, userID string) (*models.Conversation, error)
	ListMessagesAsc(sessionID, userID string) ([]models.Message, error)
	PageMessages(sessionID, userID string, page, size int) ([]models.Message, int64, error)
	CreateMessage(msg *models.Message) error
	CountMessages(sessionID, userID string) (int64, error)
	UpdateConversationMessageCount(sessionID, userID string, count int) error

	CreateAgentProperties(prop *models.AgentProperties) error
	UpdateAgentProperties(id uint, updates map[string]interface{}) error
	DeleteAgentProperties(id uint) error
	FindAgentPropertiesByName(name string) (*models.AgentProperties, error)
	PageAgentProperties(limit int) ([]models.AgentProperties, int64, error)
	CreateAgentFileAsset(asset *models.AgentFileAsset) error
}

type mysqlAgentRepository struct{}

func (mysqlAgentRepository) CreateConversation(conv *models.Conversation) error {
	return mysqlrepo.CreateConversation(conv)
}

func (mysqlAgentRepository) PageConversations(userID, scene string, offset, limit int) ([]models.Conversation, int64, error) {
	return mysqlrepo.PageConversations(userID, scene, offset, limit)
}

func (mysqlAgentRepository) EndConversation(sessionID, userID string) error {
	return mysqlrepo.EndConversation(sessionID, userID)
}

func (mysqlAgentRepository) FindConversationBySessionAndUser(sessionID, userID string) (*models.Conversation, error) {
	return mysqlrepo.FindConversationBySessionAndUser(sessionID, userID)
}

func (mysqlAgentRepository) ListMessagesAsc(sessionID, userID string) ([]models.Message, error) {
	return mysqlrepo.ListMessagesAsc(sessionID, userID)
}

func (mysqlAgentRepository) PageMessages(sessionID, userID string, page, size int) ([]models.Message, int64, error) {
	return mysqlrepo.PageMessages(sessionID, userID, page, size)
}

func (mysqlAgentRepository) CreateMessage(msg *models.Message) error {
	return mysqlrepo.CreateMessage(msg)
}

func (mysqlAgentRepository) CountMessages(sessionID, userID string) (int64, error) {
	return mysqlrepo.CountMessages(sessionID, userID)
}

func (mysqlAgentRepository) UpdateConversationMessageCount(sessionID, userID string, count int) error {
	return mysqlrepo.UpdateConversationMessageCount(sessionID, userID, count)
}

func (mysqlAgentRepository) CreateAgentProperties(prop *models.AgentProperties) error {
	return mysqlrepo.CreateAgentProperties(prop)
}

func (mysqlAgentRepository) UpdateAgentProperties(id uint, updates map[string]interface{}) error {
	return mysqlrepo.UpdateAgentProperties(id, updates)
}

func (mysqlAgentRepository) DeleteAgentProperties(id uint) error {
	return mysqlrepo.DeleteAgentProperties(id)
}

func (mysqlAgentRepository) FindAgentPropertiesByName(name string) (*models.AgentProperties, error) {
	return mysqlrepo.FindAgentPropertiesByName(name)
}

func (mysqlAgentRepository) PageAgentProperties(limit int) ([]models.AgentProperties, int64, error) {
	return mysqlrepo.PageAgentProperties(limit)
}

func (mysqlAgentRepository) CreateAgentFileAsset(asset *models.AgentFileAsset) error {
	return mysqlrepo.CreateAgentFileAsset(asset)
}
