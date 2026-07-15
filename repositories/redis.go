package repositories

import (
	"ai-meeting/config"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

var RedisClient *redis.Client

func InitRedis() {
	cfg := config.GetConfig()
	RedisClient = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// 测试连接
	_, err := RedisClient.Ping(RedisClient.Context()).Result()
	if err != nil {
		logrus.Fatalf("Failed to connect to Redis: %v", err)
	}
	logrus.Info("Redis connection established")
}
