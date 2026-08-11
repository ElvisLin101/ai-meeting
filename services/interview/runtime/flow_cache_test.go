package runtime

import (
	"context"
	"sync"
	"testing"

	"ai-meeting/models"

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

func newTestFlowCache(t *testing.T) *FlowCache {
	t.Helper()
	return NewFlowCache(newTestRedis(t))
}

func TestFlowCache_SaveAndGet(t *testing.T) {
	c := newTestFlowCache(t)
	ctx := context.Background()

	state := &models.InterviewFlowState{
		Status:                models.FlowAsking,
		CurrentIndex:          0,
		CurrentQuestionNumber: "1",
		TotalQuestions:        5,
		FollowUpCount:         0,
		MaxFollowUp:           2,
		Version:               1,
	}
	if err := c.SaveFlow(ctx, "s1", state); err != nil {
		t.Fatalf("SaveFlow failed: %v", err)
	}

	got, err := c.GetFlow(ctx, "s1")
	if err != nil {
		t.Fatalf("GetFlow failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetFlow returned nil")
	}
	if got.Status != models.FlowAsking || got.CurrentQuestionNumber != "1" ||
		got.TotalQuestions != 5 || got.Version != 1 {
		t.Errorf("GetFlow mismatch: %+v", got)
	}
}

func TestFlowCache_GetFlow_Missing(t *testing.T) {
	c := newTestFlowCache(t)
	got, err := c.GetFlow(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetFlow failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing flow, got %+v", got)
	}
}

func TestFlowCache_MutateFlow(t *testing.T) {
	c := newTestFlowCache(t)
	ctx := context.Background()

	initial := &models.InterviewFlowState{
		Status:                models.FlowAsking,
		CurrentIndex:          0,
		CurrentQuestionNumber: "1",
		TotalQuestions:        3,
		FollowUpCount:         0,
		MaxFollowUp:           2,
		Version:               1,
	}
	if err := c.SaveFlow(ctx, "s1", initial); err != nil {
		t.Fatalf("SaveFlow failed: %v", err)
	}

	next, err := c.MutateFlow(ctx, "s1", func(s *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		s.Status = models.FlowEvaluating
		return s, nil
	})
	if err != nil {
		t.Fatalf("MutateFlow failed: %v", err)
	}
	if next.Status != models.FlowEvaluating {
		t.Errorf("status = %s, want EVALUATING", next.Status)
	}
	if next.Version != 2 {
		t.Errorf("version = %d, want 2", next.Version)
	}

	got, _ := c.GetFlow(ctx, "s1")
	if got.Version != 2 || got.Status != models.FlowEvaluating {
		t.Errorf("persisted state mismatch: %+v", got)
	}
}

// TestFlowCache_MutateFlow_Concurrent 并发 CAS：N 个 goroutine 各 +1 FollowUpCount，
// 最终必须恰好为 N（版本冲突由 CAS 重试解决，不允许丢更新）。
// 注意: 生产中同一 session 的 flow 变更已由题级锁串行化，此处仅验证 CAS 在
// 轻度并发下仍正确——N 不超过 flowCASMaxRetries（最后一个胜者最多输 N-1 次）。
func TestFlowCache_MutateFlow_Concurrent(t *testing.T) {
	c := newTestFlowCache(t)
	ctx := context.Background()

	initial := &models.InterviewFlowState{
		Status:                models.FlowAsking,
		CurrentIndex:          0,
		CurrentQuestionNumber: "1",
		TotalQuestions:        3,
		FollowUpCount:         0,
		MaxFollowUp:           10,
		Version:               1,
	}
	if err := c.SaveFlow(ctx, "s1", initial); err != nil {
		t.Fatalf("SaveFlow failed: %v", err)
	}

	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := c.MutateFlow(ctx, "s1", func(s *models.InterviewFlowState) (*models.InterviewFlowState, error) {
				s.FollowUpCount++
				return s, nil
			}); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent MutateFlow failed: %v", err)
	}

	got, _ := c.GetFlow(ctx, "s1")
	if got.FollowUpCount != n {
		t.Errorf("FollowUpCount = %d, want %d (lost update)", got.FollowUpCount, n)
	}
	if got.Version != 1+n {
		t.Errorf("version = %d, want %d", got.Version, 1+n)
	}
}

func TestFlowCache_MutateFlow_NotFound(t *testing.T) {
	c := newTestFlowCache(t)
	_, err := c.MutateFlow(context.Background(), "missing", func(s *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		return s, nil
	})
	if err == nil {
		t.Fatal("expected error for missing flow, got nil")
	}
}
