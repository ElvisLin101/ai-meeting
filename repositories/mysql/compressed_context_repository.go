package mysql

import (
	"ai-meeting/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertCompressedContext 按 (session_id, memory_scope) 覆盖写入压缩快照
func UpsertCompressedContext(record *models.CompressedContextRecord) error {
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id", "memory_scope", "compressed_content", "index",
			"total_length", "message_count", "updated_at",
		}),
	}).Create(record).Error
}

// FindCompressedContext 读取压缩快照
func FindCompressedContext(sessionID string) (*models.CompressedContextRecord, error) {
	var record models.CompressedContextRecord
	result := DB.Where("session_id = ?", sessionID).First(&record)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &record, nil
}

// DeleteCompressedContext 删除压缩快照
func DeleteCompressedContext(sessionID string) error {
	return DB.Where("session_id = ?", sessionID).Delete(&models.CompressedContextRecord{}).Error
}
