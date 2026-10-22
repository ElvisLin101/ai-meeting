package mysql

import (
	"ai-meeting/models"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertUserProfile 写入用户画像（user_id 唯一），版本号递增由调用方维护
func UpsertUserProfile(profile *models.UserProfile) error {
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"profile_json", "schema_version", "version", "last_consolidated_seq", "updated_at",
		}),
	}).Create(profile).Error
}

// UpdateUserProfileCAS 用版本号做乐观锁更新画像；返回 false 表示版本冲突。
func UpdateUserProfileCAS(userID, profileJSON string, expectedVersion int, lastSeq int64) (bool, error) {
	res := DB.Model(&models.UserProfile{}).
		Where("user_id = ? AND version = ?", userID, expectedVersion).
		Updates(map[string]interface{}{
			"profile_json":          profileJSON,
			"schema_version":        1,
			"version":               expectedVersion + 1,
			"last_consolidated_seq": lastSeq,
			"updated_at":            time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// FindUserProfile 读取用户画像
func FindUserProfile(userID string) (*models.UserProfile, error) {
	var profile models.UserProfile
	result := DB.Where("user_id = ?", userID).First(&profile)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &profile, nil
}
