package mysql

import "ai-meeting/models"

func ListInterviewAgentMessagesAsc(sessionID, userID string) ([]models.AgentMessage, error) {
	var messages []models.AgentMessage
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("sequence ASC").
		Find(&messages)
	return messages, result.Error
}

func PageInterviewAgentMessages(sessionID string, page, size int, userID string) ([]models.AgentMessage, int64, error) {
	var messages []models.AgentMessage
	var total int64
	query := DB.Model(&models.AgentMessage{}).Where("user_id = ?", userID)
	if sessionID != "" {
		query = query.Where("session_id = ?", sessionID)
	}
	offset := (page - 1) * size
	result := query.Count(&total).Order("created_at DESC").Offset(offset).Limit(size).Find(&messages)
	return messages, total, result.Error
}
