package mysql

import "ai-meeting/models"

func FindEnabledAiProperty(aiID uint) (*models.AiProperties, error) {
	var prop models.AiProperties
	query := DB.Where("is_enabled = ?", true)
	if aiID > 0 {
		query = query.Where("id = ?", aiID)
	}

	result := query.Order("id ASC").First(&prop)
	return &prop, result.Error
}

func FindAllEnabledAiProperties() ([]models.AiProperties, error) {
	var props []models.AiProperties
	result := DB.Where("is_enabled = ?", true).Find(&props)
	return props, result.Error
}

func CreateAiProperties(prop *models.AiProperties) error {
	return DB.Create(prop).Error
}

func UpdateAiProperties(id uint, updates map[string]interface{}) error {
	result := DB.Model(&models.AiProperties{}).Where("id = ?", id).Updates(updates)
	return result.Error
}

func DeleteAiProperties(id uint) error {
	return DB.Delete(&models.AiProperties{}, id).Error
}

func FindAiPropertiesByID(id uint) (*models.AiProperties, error) {
	var prop models.AiProperties
	result := DB.Where("id = ?", id).First(&prop)
	return &prop, result.Error
}

func PageAiProperties(offset, limit int) ([]models.AiProperties, int64, error) {
	var props []models.AiProperties
	var total int64
	result := DB.Model(&models.AiProperties{}).Count(&total).Offset(offset).Limit(limit).Find(&props)
	return props, total, result.Error
}

func ToggleAiPropertiesStatus(id uint, isEnabled bool) error {
	result := DB.Model(&models.AiProperties{}).Where("id = ?", id).Update("is_enabled", isEnabled)
	return result.Error
}
