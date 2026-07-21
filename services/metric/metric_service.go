package metric

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ============================================================
// MetricService 统一异步指标日志服务
//
// 各模块通过 Record() 往 channel 发一条指标, 零等待
// 后台 goroutine 每 3 秒或缓冲区满时批量 flush 到 MySQL
// 主流程不阻塞, 不因日志失败影响业务
// ============================================================

const (
	bufferSize     = 4096
	flushInterval  = 3 * time.Second
	flushBatchSize = 200
)

// MetricService 指标服务单例
type MetricService struct {
	ch     chan models.MetricLog
	stopCh chan struct{}
	wg     sync.WaitGroup
}

var instance *MetricService
var once sync.Once

// GetMetricService 获取单例
func GetMetricService() *MetricService {
	once.Do(func() {
		instance = &MetricService{
			ch:     make(chan models.MetricLog, bufferSize),
			stopCh: make(chan struct{}),
		}
		instance.Start()
	})
	return instance
}

// Start 启动后台 flush goroutine
func (s *MetricService) Start() {
	s.wg.Add(1)
	go s.flushLoop()
	logrus.Info("MetricService started")
}

// Stop 停止并 flush 剩余数据
func (s *MetricService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// Record 异步记录一条指标（非阻塞, channel 满则丢弃并告警）
func (s *MetricService) Record(log models.MetricLog) {
	select {
	case s.ch <- log:
	default:
		logrus.Warn("Metric channel full, dropping metric log")
	}
}

// RecordAICall 便捷方法: 记录 AI 调用指标
func (s *MetricService) RecordAICall(module, event, sessionID string, success bool, errorType string, isRetry bool, durationMs int64, extra string) {
	s.Record(models.MetricLog{
		Module:     module,
		Event:      event,
		SessionID:  sessionID,
		Success:    success,
		ErrorType:  errorType,
		IsRetry:    isRetry,
		DurationMs: durationMs,
		Extra:      extra,
	})
}

// flushLoop 后台定时批量写入
func (s *MetricService) flushLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]models.MetricLog, 0, flushBatchSize)

	for {
		select {
		case <-s.stopCh:
			// 停止时 drain 剩余
			s.drainAndFlush(batch)
			return
		case log := <-s.ch:
			batch = append(batch, log)
			if len(batch) >= flushBatchSize {
				s.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

// drainAndFlush 停止时排空 channel 并写入
func (s *MetricService) drainAndFlush(batch []models.MetricLog) {
	for {
		select {
		case log := <-s.ch:
			batch = append(batch, log)
		default:
			if len(batch) > 0 {
				s.flush(batch)
			}
			return
		}
	}
}

// flush 批量写入 MySQL, 失败只告警不 panic
func (s *MetricService) flush(batch []models.MetricLog) {
	if len(batch) == 0 {
		return
	}
	if err := mysqlrepo.BatchCreateMetricLogs(batch); err != nil {
		logrus.Warnf("Failed to flush metric logs: %v, count=%d", err, len(batch))
	}
}
