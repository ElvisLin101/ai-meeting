package main

import (
	"ai-meeting/api/routes"
	"ai-meeting/config"
	"ai-meeting/repositories"
	mongorepo "ai-meeting/repositories/mongo"
	mysqlrepo "ai-meeting/repositories/mysql"
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
