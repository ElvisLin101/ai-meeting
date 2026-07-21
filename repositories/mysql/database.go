package mysql

import (
	"ai-meeting/config"
	"ai-meeting/models"
	"fmt"

	"github.com/sirupsen/logrus"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() error {
	cfg := config.AppConfig.Database

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
	)

	var err error
	DB, err = gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	// 自动迁移所有 MySQL 表
	if err := DB.AutoMigrate(
		&models.MetricLog{},
		&models.User{},
		&models.AiProperties{},
		&models.AgentProperties{},
		&models.AgentFileAsset{},
	); err != nil {
		logrus.Warnf("Failed to auto migrate tables: %v", err)
	}

	logrus.Info("Database connection established")
	return nil
}
