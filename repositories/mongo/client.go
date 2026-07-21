package mongo

import (
	"ai-meeting/config"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	Client              *drivermongo.Client
	ErrMongoUnavailable = errors.New("mongodb is not initialized")
)

func InitMongoDB() error {
	cfg := config.AppConfig.MongoDB

	uri := fmt.Sprintf("mongodb://%s:%d", cfg.Host, cfg.Port)
	if cfg.Username != "" && cfg.Password != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s:%d/%s?authSource=admin", cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	Client, err = drivermongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}

	if err := Client.Ping(ctx, nil); err != nil {
		return err
	}

	logrus.Info("MongoDB connection established")
	return nil
}

func GetCollection(name string) (*drivermongo.Collection, error) {
	if Client == nil {
		return nil, ErrMongoUnavailable
	}
	return Client.Database(config.AppConfig.MongoDB.DBName).Collection(name), nil
}
