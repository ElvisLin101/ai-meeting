package mysql

import "ai-meeting/models"

func CreateAiConversation(conversation *models.AiConversation) error {
	return DB.Create(conversation).Error
}

func PageAiConversations(userID string, offset, limit int) ([]models.AiConversation, int64, error) {
	var conversations []models.AiConversation
	var total int64
	result := DB.Model(&models.AiConversation{}).
		Where("user_id = ?", userID).
		Count(&total).
		Order("updated_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&conversations)
	return conversations, total, result.Error
}

func UpdateAiConversation(sessionID, userID string, updates map[string]interface{}) error {
	result := DB.Model(&models.AiConversation{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Updates(updates)
	return result.Error
}

func EndAiConversation(sessionID, userID string) error {
	result := DB.Model(&models.AiConversation{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Update("status", 0)
	return result.Error
}

func DeleteAiConversation(sessionID, userID string) error {
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).Delete(&models.AiConversation{})
	return result.Error
}

func FindAiConversationBySessionID(sessionID, userID string) (*models.AiConversation, error) {
	var conversation models.AiConversation
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).First(&conversation)
	return &conversation, result.Error
}
