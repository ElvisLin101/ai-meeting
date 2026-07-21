package main

import (
	"ai-meeting/api/routes"
	"ai-meeting/config"
	"ai-meeting/models"
	"ai-meeting/repositories"
	mongorepo "ai-meeting/repositories/mongo"
	mysqlrepo "ai-meeting/repositories/mysql"
	"ai-meeting/services/metric"
	"log"

	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetLevel(logrus.InfoLevel)
	logrus.Info("Starting AI-Meeting application...")

	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := mysqlrepo.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	repositories.InitRedis()

	// 注入 singleflight 指标回调
	metricSvc := metric.GetMetricService()
	repositories.SingleFlight.SetMetricFunc(func(module, event string, success bool, extra string) {
		metricSvc.Record(models.MetricLog{
			Module:   module,
			Event:    event,
			Success:  success,
			Extra:    extra,
		})
	})

	if err := mongorepo.InitMongoDB(); err != nil {
		log.Fatalf("Failed to initialize MongoDB: %v", err)
	}

	r := routes.SetupRouter()

	port := config.AppConfig.Server.Port
	logrus.Infof("Server starting on port %s", port)

	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
