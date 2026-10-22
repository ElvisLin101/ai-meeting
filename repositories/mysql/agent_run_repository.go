package mysql

import (
	"ai-meeting/models"

	"gorm.io/gorm"
)

// CreateAgentRun 创建 run 记录
func CreateAgentRun(run *models.AgentRun) error {
	return DB.Create(run).Error
}

// FinishAgentRun 更新 run 结束状态
func FinishAgentRun(runID, status, stopReason, output, errMsg string, steps, tokens int) error {
	updates := map[string]interface{}{
		"status":      status,
		"stop_reason": stopReason,
		"output":      output,
		"error":       errMsg,
		"steps":       steps,
		"tokens":      tokens,
	}
	return DB.Model(&models.AgentRun{}).Where("run_id = ?", runID).
		Updates(updates).Update("finished_at", gorm.Expr("NOW()")).Error
}

// UpdateAgentRunPlan 保存 Plan-and-Execute 的任务图与任务状态。
func UpdateAgentRunPlan(runID, planJSON string) error {
	return DB.Model(&models.AgentRun{}).Where("run_id = ?", runID).
		Update("plan_json", planJSON).Error
}

// FindAgentRun 按 run_id 查询
func FindAgentRun(runID string) (*models.AgentRun, error) {
	var run models.AgentRun
	result := DB.Where("run_id = ?", runID).First(&run)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &run, nil
}

// PageAgentRuns 分页查询用户的 run
func PageAgentRuns(userID string, offset, limit int) ([]models.AgentRun, int64, error) {
	var runs []models.AgentRun
	var total int64
	query := DB.Model(&models.AgentRun{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	result := query.Order("id DESC").Offset(offset).Limit(limit).Find(&runs)
	return runs, total, result.Error
}

// CreateAgentStep 写入步骤记录
func CreateAgentStep(step *models.AgentStep) error {
	return DB.Create(step).Error
}

// ListAgentSteps 按步序读取一次 run 的全部步骤
func ListAgentSteps(runID string) ([]models.AgentStep, error) {
	var steps []models.AgentStep
	result := DB.Where("run_id = ?", runID).Order("step_index ASC").Find(&steps)
	return steps, result.Error
}
