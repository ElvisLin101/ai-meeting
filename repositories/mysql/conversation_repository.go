package mysql

import (
	"ai-meeting/models"

	"gorm.io/gorm"
)

// CreateConversation 创建会话（session_id 唯一）
func CreateConversation(conv *models.Conversation) error {
	return DB.Create(conv).Error
}

// UpsertConversation 按 session_id 幂等写入（冲突时更新标题/状态等可变字段）
func UpsertConversation(conv *models.Conversation) error {
	return DB.Where("session_id = ?", conv.SessionID).
		Assign(map[string]interface{}{
			"user_id":       conv.UserID,
			"scene":         conv.Scene,
			"agent_id":      conv.AgentID,
			"title":         conv.Title,
			"status":        conv.Status,
			"message_count": conv.MessageCount,
		}).
		FirstOrCreate(conv).Error
}

// FindConversationBySessionID 按 session_id 查询会话
func FindConversationBySessionID(sessionID string) (*models.Conversation, error) {
	var conv models.Conversation
	result := DB.Where("session_id = ?", sessionID).First(&conv)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &conv, nil
}

// FindConversationBySessionAndUser 查询并校验归属
func FindConversationBySessionAndUser(sessionID, userID string) (*models.Conversation, error) {
	var conv models.Conversation
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).First(&conv)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &conv, nil
}

// PageConversations 按用户与场景分页查询会话
func PageConversations(userID, scene string, offset, limit int) ([]models.Conversation, int64, error) {
	var convs []models.Conversation
	var total int64
	query := DB.Model(&models.Conversation{}).Where("user_id = ?", userID)
	if scene != "" {
		query = query.Where("scene = ?", scene)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	result := query.Order("updated_at DESC").Offset(offset).Limit(limit).Find(&convs)
	return convs, total, result.Error
}

// UpdateConversation 更新会话可变字段
func UpdateConversation(sessionID, userID string, updates map[string]interface{}) error {
	return DB.Model(&models.Conversation{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Updates(updates).Error
}

// UpdateConversationMessageCount 更新会话消息计数
func UpdateConversationMessageCount(sessionID, userID string, count int) error {
	return DB.Model(&models.Conversation{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Update("message_count", count).Error
}

// EndConversation 结束会话
func EndConversation(sessionID, userID string) error {
	return DB.Model(&models.Conversation{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Update("status", models.ConversationStatusEnded).Error
}

// DeleteConversation 删除会话及其消息
func DeleteConversation(sessionID, userID string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ? AND user_id = ?", sessionID, userID).
			Delete(&models.Conversation{}).Error; err != nil {
			return err
		}
		return tx.Where("session_id = ?", sessionID).Delete(&models.Message{}).Error
	})
}
