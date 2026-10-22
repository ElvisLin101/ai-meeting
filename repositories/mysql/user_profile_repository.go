package mysql

import (
	"ai-meeting/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertUserProfile 写入用户画像（user_id 唯一），版本号递增由调用方维护
func UpsertUserProfile(profile *models.UserProfile) error {
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"profile_json", "version", "updated_at"}),
	}).Create(profile).Error
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
