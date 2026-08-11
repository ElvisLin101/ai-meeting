package flow

import (
	"context"
	"testing"

	"ai-meeting/dto"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// newTestRedis 启动内存 Redis 用于测试
func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func newTestIdempotency(t *testing.T) *IdempotencyService {
	t.Helper()
	return NewIdempotencyService(newTestRedis(t))
}

func TestIdempotency_NewRequest(t *testing.T) {
	s := newTestIdempotency(t)
	res, err := s.TryStart(context.Background(), "s1", "req-1")
	if err != nil {
		t.Fatalf("TryStart failed: %v", err)
	}
	if res.Status != IdempotencyNew {
		t.Errorf("status = %s, want %s", res.Status, IdempotencyNew)
	}
}

func TestIdempotency_EmptyRequestID(t *testing.T) {
	s := newTestIdempotency(t)
	res, err := s.TryStart(context.Background(), "s1", "")
	if err != nil {
		t.Fatalf("TryStart failed: %v", err)
	}
	if res.Status != IdempotencyNew {
		t.Errorf("status = %s, want %s", res.Status, IdempotencyNew)
	}
}

func TestIdempotency_ProcessingBlocked(t *testing.T) {
	s := newTestIdempotency(t)
	ctx := context.Background()

	if _, err := s.TryStart(ctx, "s1", "req-1"); err != nil {
		t.Fatalf("first TryStart failed: %v", err)
	}
	res, err := s.TryStart(ctx, "s1", "req-1")
	if err != nil {
		t.Fatalf("second TryStart failed: %v", err)
	}
	if res.Status != IdempotencyProcessing {
		t.Errorf("status = %s, want %s", res.Status, IdempotencyProcessing)
	}
}

func TestIdempotency_ReplayAfterSuccess(t *testing.T) {
	s := newTestIdempotency(t)
	ctx := context.Background()

	if _, err := s.TryStart(ctx, "s1", "req-1"); err != nil {
		t.Fatalf("TryStart failed: %v", err)
	}
	resp := &dto.InterviewAnswerRespDTO{QuestionNumber: "1", Score: 88, Finished: true}
	if err := s.MarkSucceeded(ctx, "s1", "req-1", resp); err != nil {
		t.Fatalf("MarkSucceeded failed: %v", err)
	}

	res, err := s.TryStart(ctx, "s1", "req-1")
	if err != nil {
		t.Fatalf("TryStart after success failed: %v", err)
	}
	if res.Status != IdempotencySucceeded {
		t.Fatalf("status = %s, want %s", res.Status, IdempotencySucceeded)
	}
	if res.Response == nil || res.Response.Score != 88 || res.Response.QuestionNumber != "1" {
		t.Errorf("replayed response mismatch: %+v", res.Response)
	}
}

func TestIdempotency_ClearProcessing(t *testing.T) {
	s := newTestIdempotency(t)
	ctx := context.Background()

	if _, err := s.TryStart(ctx, "s1", "req-1"); err != nil {
		t.Fatalf("TryStart failed: %v", err)
	}
	if err := s.ClearProcessing(ctx, "s1", "req-1"); err != nil {
		t.Fatalf("ClearProcessing failed: %v", err)
	}

	res, err := s.TryStart(ctx, "s1", "req-1")
	if err != nil {
		t.Fatalf("TryStart after clear failed: %v", err)
	}
	if res.Status != IdempotencyNew {
		t.Errorf("status = %s, want %s (失败后应可重试)", res.Status, IdempotencyNew)
	}
}

func TestNormalizeRequestId(t *testing.T) {
	a := NormalizeRequestId("s1", "1", "同一个答案")
	b := NormalizeRequestId("s1", "1", "同一个答案")
	if a != b {
		t.Errorf("NormalizeRequestId not deterministic: %s vs %s", a, b)
	}
	if len(a) == 0 || a[:5] != "auto-" {
		t.Errorf("unexpected request id format: %q", a)
	}
	c := NormalizeRequestId("s1", "1", "不同答案")
	if c == a {
		t.Errorf("different answers produced same id: %s", c)
	}
}
