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
