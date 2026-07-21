package mysql

import (
	"ai-meeting/models"
	"time"
)

// BatchCreateMetricLogs 批量插入指标日志
func BatchCreateMetricLogs(logs []models.MetricLog) error {
	if len(logs) == 0 {
		return nil
	}
	now := time.Now()
	for i := range logs {
		if logs[i].CreatedAt.IsZero() {
			logs[i].CreatedAt = now
		}
	}
	return DB.CreateInBatches(logs, 100).Error
}
