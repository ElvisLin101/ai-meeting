package metric

import (
	"testing"

	"ai-meeting/models"
)

// ============================================================
// 指标服务测试: channel 非阻塞/满丢弃 + AI 调用埋点组装 + 空批 flush
// 直接构造 &MetricService{}, 不启动后台 goroutine, 不触 MySQL
// ============================================================

func TestRecord_Buffered(t *testing.T) {
	s := &MetricService{ch: make(chan models.MetricLog, 2)}

	s.Record(models.MetricLog{Module: "a", Event: "e1"})
	s.Record(models.MetricLog{Module: "b", Event: "e2"})
	if len(s.ch) != 2 {
		t.Fatalf("channel len = %d, want 2", len(s.ch))
	}

	// channel 满时丢弃, 不阻塞
	s.Record(models.MetricLog{Module: "c", Event: "e3"})
	if len(s.ch) != 2 {
		t.Errorf("channel 满应丢弃, len = %d", len(s.ch))
	}

	// 内容 FIFO 顺序
	first := <-s.ch
	if first.Module != "a" || first.Event != "e1" {
		t.Errorf("first = %+v", first)
	}
}

func TestRecordAICall(t *testing.T) {
	s := &MetricService{ch: make(chan models.MetricLog, 1)}
	s.RecordAICall("ai_call", "eval", "session-1", true, "", false, 1234, "extra-info")

	log := <-s.ch
	if log.Module != "ai_call" || log.Event != "eval" {
		t.Errorf("module/event = %s/%s", log.Module, log.Event)
	}
	if log.SessionID != "session-1" || !log.Success {
		t.Errorf("session/success = %s/%v", log.SessionID, log.Success)
	}
	if log.IsRetry || log.DurationMs != 1234 || log.Extra != "extra-info" {
		t.Errorf("字段组装异常: %+v", log)
	}
}

func TestFlush_EmptyBatch(t *testing.T) {
	s := &MetricService{}
	// 空批不触发 MySQL, 不应 panic
	s.flush(nil)
	s.flush([]models.MetricLog{})
}
