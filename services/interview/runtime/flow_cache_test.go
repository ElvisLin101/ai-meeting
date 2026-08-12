package runtime

import (
	"context"
	"sync"
	"testing"

	"ai-meeting/internal/testutil"
	"ai-meeting/models"
)

func newTestFlowCache(t *testing.T) *FlowCache {
	t.Helper()
	return NewFlowCache(testutil.NewTestRedis(t))
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

func TestFlowCache_RollbackFlowCAS_Success(t *testing.T) {
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

	// 自己推进到 EVALUATING (version 2)
	adv, err := c.MutateFlow(ctx, "s1", func(s *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		s.Status = models.FlowEvaluating
		return s, nil
	})
	if err != nil {
		t.Fatalf("MutateFlow failed: %v", err)
	}
	if adv.Version != 2 {
		t.Fatalf("version = %d, want 2", adv.Version)
	}

	// 用自己推进后的 version 条件回滚到快照(ASKING v1) → 成功
	rolled, err := c.RollbackFlowCAS(ctx, "s1", adv.Version, initial)
	if err != nil {
		t.Fatalf("RollbackFlowCAS failed: %v", err)
	}
	if !rolled {
		t.Fatal("expected rollback to succeed, got skipped")
	}

	got, _ := c.GetFlow(ctx, "s1")
	if got.Status != models.FlowAsking || got.Version != 1 {
		t.Errorf("after rollback: %+v, want ASKING v1", got)
	}
}

// TestFlowCache_RollbackFlowCAS_SkipWhenAdvanced 条件回滚: 当前 version 已不是
// 调用方推进后的版本(被其他请求推进)时, 必须放弃回滚, 不砸掉并发已提交的进度。
func TestFlowCache_RollbackFlowCAS_SkipWhenAdvanced(t *testing.T) {
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

	// 自己推进到 EVALUATING(v2), 随后"别人"又推进到 ASKING 第 2 题(v3)
	if _, err := c.MutateFlow(ctx, "s1", func(s *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		s.Status = models.FlowEvaluating
		return s, nil
	}); err != nil {
		t.Fatalf("MutateFlow#1 failed: %v", err)
	}
	advanced, err := c.MutateFlow(ctx, "s1", func(s *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		s.Status = models.FlowAsking
		s.CurrentIndex = 1
		s.CurrentQuestionNumber = "2"
		return s, nil
	})
	if err != nil {
		t.Fatalf("MutateFlow#2 failed: %v", err)
	}

	// 用旧 version(=2) 回滚 → 当前 v3 不匹配 → 跳过, flow 保持 v3
	rolled, err := c.RollbackFlowCAS(ctx, "s1", 2, initial)
	if err != nil {
		t.Fatalf("RollbackFlowCAS failed: %v", err)
	}
	if rolled {
		t.Fatal("expected rollback to be skipped, got rolled back")
	}

	got, _ := c.GetFlow(ctx, "s1")
	if got.Version != advanced.Version || got.CurrentQuestionNumber != "2" {
		t.Errorf("flow should stay untouched after skipped rollback, got %+v (want v%d Q2)", got, advanced.Version)
	}
}

// TestFlowCache_RollbackFlowCAS_Concurrent 并发竞争: 推进与"旧 version 回滚"同时发生。
// 回滚固定期望 version=1, 任何一次推进成功后 version >= 2, 后续回滚全部被跳过,
// 因此已提交的推进(FollowUpCount)绝不能被回滚吃掉, 最终状态必须自洽。
func TestFlowCache_RollbackFlowCAS_Concurrent(t *testing.T) {
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

	const n = 6
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				if _, err := c.MutateFlow(ctx, "s1", func(s *models.InterviewFlowState) (*models.InterviewFlowState, error) {
					s.FollowUpCount++
					return s, nil
				}); err != nil {
					t.Errorf("concurrent MutateFlow failed: %v", err)
				}
			} else {
				if _, err := c.RollbackFlowCAS(ctx, "s1", 1, initial); err != nil {
					t.Errorf("concurrent RollbackFlowCAS failed: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()

	got, _ := c.GetFlow(ctx, "s1")
	if got.Version != 1+got.FollowUpCount {
		t.Errorf("inconsistent state: version=%d FollowUpCount=%d (rollback must not eat committed progress)",
			got.Version, got.FollowUpCount)
	}
	if got.FollowUpCount == 0 {
		t.Error("expected at least one advance to succeed")
	}
}
