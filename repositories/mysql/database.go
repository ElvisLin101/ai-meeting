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

	// 自动迁移所有 MySQL 表。
	// 生产环境建议关闭 AutoMigrate，改为显式版本化迁移；这里失败必须阻断启动，
	// 否则 schema 漂移会在第一次写库时才以 SQL 错误暴露。
	if err := DB.AutoMigrate(
		&models.MetricLog{},
		&models.User{},
		&models.AiProperties{},
		&models.AgentProperties{},
		&models.AgentFileAsset{},
		// Agent-first 改造: 统一会话/消息 + 跨会话画像 + run 可观测
		&models.Conversation{},
		&models.Message{},
		&models.SessionSequence{},
		&models.CompressedContextRecord{},
		&models.UserProfile{},
		&models.ProfileCandidate{},
		&models.ProfileConfirmation{},
		&models.ProfileEvent{},
		&models.AgentIdentity{},
		&models.AgentRun{},
		&models.AgentStep{},
		// 面试模块从 Mongo 迁移到 MySQL 的行模型
		&models.InterviewSessionRow{},
		&models.InterviewRecordRow{},
		&models.InterviewQuestionRow{},
		&models.InterviewHotSnapshotRow{},
		&models.InterviewColdSnapshotRow{},
		&models.InterviewTurnArchiveRow{},
	); err != nil {
		return fmt.Errorf("auto migrate tables: %w", err)
	}

	logrus.Info("Database connection established")
	return nil
}
