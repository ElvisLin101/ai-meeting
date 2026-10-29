package metric

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
)

type metricRepository interface {
	BatchCreateMetricLogs(logs []models.MetricLog) error
}

type mysqlMetricRepository struct{}

func (mysqlMetricRepository) BatchCreateMetricLogs(logs []models.MetricLog) error {
	return mysqlrepo.BatchCreateMetricLogs(logs)
}
