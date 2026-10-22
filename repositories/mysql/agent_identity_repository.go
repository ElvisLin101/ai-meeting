package mysql

import (
	"ai-meeting/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertAgentIdentity 写入 Agent 自我定位（agent_id 唯一）
func UpsertAgentIdentity(identity *models.AgentIdentity) error {
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"identity_json", "version", "updated_at"}),
	}).Create(identity).Error
}

// FindAgentIdentity 读取 Agent 自我定位
func FindAgentIdentity(agentID uint) (*models.AgentIdentity, error) {
	var identity models.AgentIdentity
	result := DB.Where("agent_id = ?", agentID).First(&identity)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &identity, nil
}
