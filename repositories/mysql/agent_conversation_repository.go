package mysql

import "ai-meeting/models"

func CreateAgentConversation(conversation *models.AgentConversation) error {
	return DB.Create(conversation).Error
}

func PageAgentConversations(userID string, offset, limit int) ([]models.AgentConversation, int64, error) {
	var conversations []models.AgentConversation
	var total int64
	result := DB.Model(&models.AgentConversation{}).
		Where("user_id = ?", userID).
		Count(&total).
		Order("updated_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&conversations)
	return conversations, total, result.Error
}

func EndAgentConversation(sessionID, userID string) error {
	result := DB.Model(&models.AgentConversation{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Update("status", 0)
	return result.Error
}

// GetAgentConversationBySessionId 按 sessionId 和 userID 查询会话（用于归属校验）
func GetAgentConversationBySessionId(sessionID, userID string) (*models.AgentConversation, error) {
	var conv models.AgentConversation
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).First(&conv)
	if result.Error != nil {
		return nil, result.Error
	}
	return &conv, nil
}

// GetAgentConversationBySessionIdOnly 按 sessionId 查询会话（不需要 userID）
func GetAgentConversationBySessionIdOnly(sessionID string) (*models.AgentConversation, error) {
	var conv models.AgentConversation
	result := DB.Where("session_id = ?", sessionID).First(&conv)
	if result.Error != nil {
		return nil, result.Error
	}
	return &conv, nil
}

// UpdateAgentConversationMessageCount 更新会话消息计数
func UpdateAgentConversationMessageCount(sessionID string, messageCnt int) error {
	result := DB.Model(&models.AgentConversation{}).
		Where("session_id = ?", sessionID).
		Update("message_cnt", messageCnt)
	return result.Error
}
