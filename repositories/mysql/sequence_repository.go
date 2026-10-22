package mysql

import (
	"ai-meeting/models"

	"gorm.io/gorm"
)

// NextSessionSequence 原子分配会话内序列号（从 1 开始）。
//
// 用 MySQL 的 LAST_INSERT_ID(expr) 技巧在单条语句内完成"自增并取值"：
//
//	INSERT INTO session_sequences (session_id, seq) VALUES (?, LAST_INSERT_ID(1))
//	ON DUPLICATE KEY UPDATE seq = LAST_INSERT_ID(seq + 1)
//
// LAST_INSERT_ID 是连接级状态，必须在同一事务（同一连接）内读取。
func NextSessionSequence(tx *gorm.DB, sessionID string) (int64, error) {
	if tx == nil {
		tx = DB
	}
	var seq int64
	err := tx.Transaction(func(t *gorm.DB) error {
		if err := t.Exec(
			"INSERT INTO session_sequences (session_id, seq, updated_at) VALUES (?, LAST_INSERT_ID(1), NOW()) "+
				"ON DUPLICATE KEY UPDATE seq = LAST_INSERT_ID(seq + 1), updated_at = NOW()",
			sessionID,
		).Error; err != nil {
			return err
		}
		return t.Raw("SELECT LAST_INSERT_ID()").Scan(&seq).Error
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}

// FindSessionSequence 查询当前序列号（不存在返回 0）
func FindSessionSequence(sessionID string) (int64, error) {
	var record models.SessionSequence
	result := DB.Where("session_id = ?", sessionID).First(&record)
	if result.Error == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if result.Error != nil {
		return 0, result.Error
	}
	return record.Seq, nil
}
