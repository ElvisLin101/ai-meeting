package flow

import (
	"ai-meeting/models"
	"ai-meeting/services/interview/runtime"
	"ai-meeting/services/metric"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// ============================================================
// TurnRepairService turn log 写失败时的异步补偿
// 写失败 → 入 Redis 队列 → 定时重试写入, 最多 6 次
// ============================================================

const (
	turnRepairQueueKey = "interview:turn:repair:queue"
	turnRepairInterval = 3 * time.Second
	turnRepairMaxRetry = 6
)

// turnRepairItem 补偿队列条目
type turnRepairItem struct {
	SessionID string                   `json:"session_id"`
	Turn      models.InterviewTurnLog  `json:"turn"`
	Retry     int                      `json:"retry"`
}

// TurnRepairService turn log 补偿服务
type TurnRepairService struct {
	rdb          *redis.Client
	turnLogCache *runtime.TurnLogCache
	stopCh       chan struct{}
}

// NewTurnRepairService 创建补偿服务
func NewTurnRepairService(rdb *redis.Client, turnLogCache *runtime.TurnLogCache) *TurnRepairService {
	return &TurnRepairService{
		rdb:          rdb,
		turnLogCache: turnLogCache,
		stopCh:       make(chan struct{}),
	}
}

// Enqueue turn log 写失败时入队
func (s *TurnRepairService) Enqueue(ctx context.Context, sessionID string, turn *models.InterviewTurnLog) {
	item := turnRepairItem{
		SessionID: sessionID,
		Turn:      *turn,
		Retry:     0,
	}
	payload, err := json.Marshal(item)
	if err != nil {
		logrus.Errorf("Failed to marshal turn repair item: %v", err)
		return
	}
	if err := s.rdb.LPush(ctx, turnRepairQueueKey, payload).Err(); err != nil {
		logrus.Errorf("Failed to enqueue turn repair, session=%s, err=%v", sessionID, err)
	}
}

// Start 启动补偿定时任务
func (s *TurnRepairService) Start() {
	go s.loop()
	logrus.Info("TurnRepairService started")
}

// Stop 停止补偿定时任务
func (s *TurnRepairService) Stop() {
	close(s.stopCh)
}

// loop 定时从队列取条目重试
func (s *TurnRepairService) loop() {
	ticker := time.NewTicker(turnRepairInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processBatch()
		}
	}
}

// processBatch 批量处理队列
func (s *TurnRepairService) processBatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := 0; i < 50; i++ { // 每轮最多处理 50 条
		payload, err := s.rdb.RPop(ctx, turnRepairQueueKey).Result()
		if err == redis.Nil {
			return // 队列空
		}
		if err != nil {
			logrus.Warnf("Failed to pop turn repair item: %v", err)
			return
		}

		var item turnRepairItem
		if err := json.Unmarshal([]byte(payload), &item); err != nil {
			logrus.Errorf("Failed to unmarshal turn repair item: %v", err)
			continue
		}

		_, err = s.turnLogCache.AppendTurnIfAbsent(ctx, item.SessionID, &item.Turn)
		if err != nil {
			item.Retry++
			if item.Retry >= turnRepairMaxRetry {
				logrus.Errorf("Turn repair max retries exceeded, session=%s, requestId=%s", item.SessionID, item.Turn.RequestID)
				metric.GetMetricService().Record(models.MetricLog{Module: "repair", Event: "max_retries_exceeded", Success: false, SessionID: item.SessionID})
				continue // 丢弃
			}
			// 重新入队
			retryPayload, _ := json.Marshal(item)
			s.rdb.LPush(ctx, turnRepairQueueKey, retryPayload)
			logrus.Warnf("Turn repair retry %d/%d, session=%s", item.Retry, turnRepairMaxRetry, item.SessionID)
		} else {
			logrus.Infof("Turn repair succeeded, session=%s, requestId=%s", item.SessionID, item.Turn.RequestID)
			metric.GetMetricService().Record(models.MetricLog{Module: "repair", Event: "repair_succeeded", Success: true, SessionID: item.SessionID})
		}
	}
}

// PendingCount 队列待处理条目数（用于监控）
func (s *TurnRepairService) PendingCount(ctx context.Context) (int64, error) {
	count, err := s.rdb.LLen(ctx, turnRepairQueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get pending count: %w", err)
	}
	return count, nil
}
