package mysql

import (
	"ai-meeting/models"
	"time"

	"gorm.io/gorm"
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
// userID 过滤防止跨用户误更新。
func MarkProfileCandidates(userID string, ids []uint, status string) error {
	if userID == "" || len(ids) == 0 {
		return nil
	}
	return DB.Model(&models.ProfileCandidate{}).
		Where("user_id = ? AND id IN ?", userID, ids).Update("status", status).Error
}

// CreateProfileConfirmation 写入待用户确认的画像项。
func CreateProfileConfirmation(c *models.ProfileConfirmation) error {
	return DB.Create(c).Error
}

// CreateProfileEvent 写入 L2 consolidation 审计记录。
func CreateProfileEvent(e *models.ProfileEvent) error {
	return DB.Create(e).Error
}

// ListPendingProfileConfirmations 读取待用户确认的画像项。
func ListPendingProfileConfirmations(userID string, limit int) ([]models.ProfileConfirmation, error) {
	if limit <= 0 {
		limit = 50
	}
	var items []models.ProfileConfirmation
	result := DB.Where("user_id = ? AND status = ?", userID, "pending").
		Order("id ASC").Limit(limit).Find(&items)
	return items, result.Error
}

// FindProfileConfirmation 按 id + 用户读取确认项。
func FindProfileConfirmation(id uint, userID string) (*models.ProfileConfirmation, error) {
	var item models.ProfileConfirmation
	result := DB.Where("id = ? AND user_id = ?", id, userID).First(&item)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &item, nil
}

// MarkProfileConfirmation 只允许把 pending 改为目标状态；返回是否命中。
func MarkProfileConfirmation(id uint, userID, status string) (bool, error) {
	res := DB.Model(&models.ProfileConfirmation{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, "pending").
		Updates(map[string]interface{}{"status": status, "resolved_at": time.Now()})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}
