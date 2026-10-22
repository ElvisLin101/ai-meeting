package mysql

import (
	"ai-meeting/models"

	"gorm.io/gorm"
)

// CreateMessage 分配会话内序列并写入消息（MySQL 为全量历史真相源）
func CreateMessage(msg *models.Message) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		seq, err := NextSessionSequence(tx, msg.SessionID)
		if err != nil {
			return err
		}
		msg.Sequence = seq
		return tx.Create(msg).Error
	})
}

// ListMessagesAsc 按 sequence 正序读取某会话全部消息
func ListMessagesAsc(sessionID, userID string) ([]models.Message, error) {
	var messages []models.Message
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("sequence ASC").Find(&messages)
	return messages, result.Error
}

// ListMessagesDesc 按 sequence 倒序读取某会话全部消息
func ListMessagesDesc(sessionID, userID string) ([]models.Message, error) {
	var messages []models.Message
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("sequence DESC").Find(&messages)
	return messages, result.Error
}

// ListMessagesAfterSequenceDesc 读取 sequence > after 的消息（倒序），用于压缩边界后的近期窗口
func ListMessagesAfterSequenceDesc(sessionID, userID string, after int64) ([]models.Message, error) {
	var messages []models.Message
	result := DB.Where("session_id = ? AND user_id = ? AND sequence > ?", sessionID, userID, after).
		Order("sequence DESC").Find(&messages)
	return messages, result.Error
}

// ListRecentMessages 读取最近 limit 条消息（正序返回，便于拼装上下文）
func ListRecentMessages(sessionID, userID string, limit int) ([]models.Message, error) {
	var messages []models.Message
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("sequence DESC").Limit(limit).Find(&messages)
	// 反转为正序
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, result.Error
}

// ListRecentMessagesAfter 读取 sequence > afterSeq 的最近 limit 条消息（正序）。
func ListRecentMessagesAfter(sessionID, userID string, afterSeq int64, limit int) ([]models.Message, error) {
	if limit <= 0 {
		limit = 500
	}
	var messages []models.Message
	result := DB.Where("session_id = ? AND user_id = ? AND sequence > ?", sessionID, userID, afterSeq).
		Order("sequence DESC").Limit(limit).Find(&messages)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, result.Error
}

// PageMessages 分页查询消息
func PageMessages(sessionID, userID string, page, size int) ([]models.Message, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	var messages []models.Message
	var total int64
	query := DB.Model(&models.Message{}).Where("session_id = ? AND user_id = ?", sessionID, userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	result := query.Order("sequence DESC").Offset((page - 1) * size).Limit(size).Find(&messages)
	return messages, total, result.Error
}

// CountMessages 统计会话消息数
func CountMessages(sessionID, userID string) (int64, error) {
	var total int64
	result := DB.Model(&models.Message{}).Where("session_id = ? AND user_id = ?", sessionID, userID).Count(&total)
	return total, result.Error
}

// DeleteMessagesBySession 删除会话全部消息
func DeleteMessagesBySession(sessionID string) error {
	return DB.Where("session_id = ?", sessionID).Delete(&models.Message{}).Error
}
