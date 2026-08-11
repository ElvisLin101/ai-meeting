package main

import (
	"ai-meeting/api/routes"
	"ai-meeting/config"
	"ai-meeting/models"
	"ai-meeting/repositories"
	mongorepo "ai-meeting/repositories/mongo"
	mysqlrepo "ai-meeting/repositories/mysql"
	"ai-meeting/services/metric"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// shutdownTimeout 优雅停机等待存量请求完成的超时
const shutdownTimeout = 10 * time.Second

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
			Module:  module,
			Event:   event,
			Success: success,
			Extra:   extra,
		})
	})

	if err := mongorepo.InitMongoDB(); err != nil {
		log.Fatalf("Failed to initialize MongoDB: %v", err)
	}

	r := routes.SetupRouter()

	port := config.AppConfig.Server.Port
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// 后台启动 HTTP 服务
	go func() {
		logrus.Infof("Server starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待退出信号 (Ctrl+C / 部署滚动重启 / docker stop)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logrus.Info("Shutting down server...")

	// 1. 停止接收新请求, 等待存量请求完成(超时后强制关闭)
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logrus.Warnf("Server forced to shutdown: %v", err)
		_ = srv.Close() // 超时后强制关闭剩余连接
	}

	// 2. 排空指标 buffer 落库(必须在关 DB 之前)
	metricSvc.Stop()

	// 3. 关闭底层连接
	closeConnections()

	logrus.Info("Server exited")
}

// closeConnections 关闭 MySQL/Redis/Mongo 连接
func closeConnections() {
	if sqlDB, err := mysqlrepo.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if repositories.RedisClient != nil {
		_ = repositories.RedisClient.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if mongorepo.Client != nil {
		_ = mongorepo.Client.Disconnect(ctx)
	}
}
