package mysql

import (
	"ai-meeting/models"

	"gorm.io/gorm/clause"
)

// CreateProfileCandidate 写入一条画像候选；value_hash 冲突时忽略。
func CreateProfileCandidate(c *models.ProfileCandidate) error {
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "value_hash"}},
		DoNothing: true,
	}).Create(c).Error
}

// ListPendingProfileCandidates 读取待 consolidation 的候选。
func ListPendingProfileCandidates(userID string, limit int) ([]models.ProfileCandidate, error) {
	if limit <= 0 {
		limit = 50
	}
	var candidates []models.ProfileCandidate
	result := DB.Where("user_id = ? AND status = ?", userID, "pending").
		Order("id ASC").Limit(limit).Find(&candidates)
	return candidates, result.Error
}

// CountPendingProfileCandidates 统计待 consolidation 候选数量。
func CountPendingProfileCandidates(userID string) (int64, error) {
	var count int64
	result := DB.Model(&models.ProfileCandidate{}).
		Where("user_id = ? AND status = ?", userID, "pending").Count(&count)
	return count, result.Error
}

// MarkProfileCandidates 批量更新候选状态（promoted / rejected）。
func MarkProfileCandidates(ids []uint, status string) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&models.ProfileCandidate{}).
		Where("id IN ?", ids).Update("status", status).Error
}
