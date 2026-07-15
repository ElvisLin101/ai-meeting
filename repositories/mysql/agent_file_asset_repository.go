package mysql

import "ai-meeting/models"

func CreateAgentFileAsset(asset *models.AgentFileAsset) error {
	return DB.Create(asset).Error
}
