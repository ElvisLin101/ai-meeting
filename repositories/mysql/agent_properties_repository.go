package mysql

import "ai-meeting/models"

func CreateAgentProperties(prop *models.AgentProperties) error {
	return DB.Create(prop).Error
}

func UpdateAgentProperties(id uint, updates map[string]interface{}) error {
	result := DB.Model(&models.AgentProperties{}).Where("id = ?", id).Updates(updates)
	return result.Error
}

func DeleteAgentProperties(id uint) error {
	return DB.Delete(&models.AgentProperties{}, id).Error
}

func FindAgentPropertiesByName(name string) (*models.AgentProperties, error) {
	var prop models.AgentProperties
	result := DB.Where("name = ?", name).First(&prop)
	return &prop, result.Error
}

func FindAgentPropertiesByID(id uint) (*models.AgentProperties, error) {
	var prop models.AgentProperties
	result := DB.Where("id = ?", id).First(&prop)
	return &prop, result.Error
}

func PageAgentProperties(limit int) ([]models.AgentProperties, int64, error) {
	var props []models.AgentProperties
	var total int64
	result := DB.Model(&models.AgentProperties{}).Count(&total).Limit(limit).Find(&props)
	return props, total, result.Error
}

func ListActiveAgentProperties() ([]models.AgentProperties, error) {
	var props []models.AgentProperties
	result := DB.Where("is_enabled = ?", true).Find(&props)
	return props, result.Error
}
