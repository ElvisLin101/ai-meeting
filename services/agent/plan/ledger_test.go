package plan

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLedgerDeduplicatesAndAllowsRetry(t *testing.T) {
	ledger := NewLedger()

	var calls int32
	release := make(chan struct{})
	fn := func() TaskResult {
		atomic.AddInt32(&calls, 1)
		<-release
		return TaskResult{ID: "t1", Status: StatusDone, Summary: "ok"}
	}

	var wg sync.WaitGroup
	results := make([]TaskResult, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = ledger.Do("same-key", fn)
		}(i)
	}
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("same key should execute once, got %d", got)
	}
	if results[0].Status != StatusDone || results[1].Status != StatusDone {
		t.Fatalf("unexpected results: %+v", results)
	}

	// 失败不缓存，同一 key 可以在重试时再次执行。
	var failCalls int32
	retryFn := func() TaskResult {
		n := atomic.AddInt32(&failCalls, 1)
		if n == 1 {
			return TaskResult{ID: "t2", Status: StatusFailed, Error: "timeout", Retryable: true}
		}
		return TaskResult{ID: "t2", Status: StatusDone, Summary: "ok"}
	}
	first, _ := ledger.Do("retry-key", retryFn)
	second, _ := ledger.Do("retry-key", retryFn)
	if failCalls != 2 || first.Status != StatusFailed || second.Status != StatusDone {
		t.Fatalf("failed result should not be cached: calls=%d first=%+v second=%+v", failCalls, first, second)
	}
}

func TestEnsureIdempotencyKeys(t *testing.T) {
	p := &Plan{Tasks: []Task{
		{ID: "t1", Type: TaskTypeAgent, Goal: "a"},
		{ID: "t2", Type: TaskTypeAgent, Goal: "b", DependsOn: []string{"t1"}},
	}}
	p.Normalize()
	if err := p.EnsureIdempotencyKeys(); err != nil {
		t.Fatalf("ensure keys: %v", err)
	}
	k1, k2 := p.Tasks[0].IdempotencyKey, p.Tasks[1].IdempotencyKey
	if k1 == "" || k2 == "" || k1 == k2 {
		t.Fatalf("keys should be non-empty and distinct: %q %q", k1, k2)
	}

	same := &Plan{Tasks: []Task{
		{ID: "t1", Type: TaskTypeAgent, Goal: "a"},
		{ID: "t2", Type: TaskTypeAgent, Goal: "b", DependsOn: []string{"t1"}},
	}}
	same.Normalize()
	if err := same.EnsureIdempotencyKeys(); err != nil {
		t.Fatalf("ensure same keys: %v", err)
	}
	if same.Tasks[0].IdempotencyKey != k1 || same.Tasks[1].IdempotencyKey != k2 {
		t.Fatal("same semantic plan should produce the same keys")
	}

	changed := &Plan{Tasks: []Task{
		{ID: "t1", Type: TaskTypeAgent, Goal: "changed"},
		{ID: "t2", Type: TaskTypeAgent, Goal: "b", DependsOn: []string{"t1"}},
	}}
	changed.Normalize()
	if err := changed.EnsureIdempotencyKeys(); err != nil {
		t.Fatalf("ensure changed keys: %v", err)
	}
	if changed.Tasks[1].IdempotencyKey == k2 {
		t.Fatal("upstream change should invalidate downstream key")
	}
}

func TestSchedulerRetriesRetryableFailure(t *testing.T) {
	p := &Plan{Tasks: []Task{agentTask("t1")}}
	p.Normalize()
	if err := p.EnsureIdempotencyKeys(); err != nil {
		t.Fatalf("ensure keys: %v", err)
	}

	var calls int32
	s := &Scheduler{
		MaxParallel: 1,
		RetryPolicy: RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Millisecond},
		Ledger:      NewLedger(),
	}
	results := s.Run(context.Background(), p, func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return TaskResult{ID: task.ID, Status: StatusFailed, Error: "timeout", Retryable: true}
		}
		return TaskResult{ID: task.ID, Status: StatusDone, Summary: "ok"}
	})

	if calls != 3 {
		t.Fatalf("want 3 attempts, got %d", calls)
	}
	if len(results) != 1 || results[0].Status != StatusDone || results[0].Attempts != 3 {
		t.Fatalf("unexpected result: %+v", results)
	}
}

func TestSchedulerDoesNotRetryNonRetryableFailure(t *testing.T) {
	p := &Plan{Tasks: []Task{agentTask("t1")}}
	p.Normalize()
	if err := p.EnsureIdempotencyKeys(); err != nil {
		t.Fatalf("ensure keys: %v", err)
	}

	var calls int32
	s := &Scheduler{
		MaxParallel: 1,
		RetryPolicy: RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Millisecond},
		Ledger:      NewLedger(),
	}
	results := s.Run(context.Background(), p, func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
		atomic.AddInt32(&calls, 1)
		return TaskResult{ID: task.ID, Status: StatusFailed, Error: "invalid input", Retryable: false}
	})

	if calls != 1 {
		t.Fatalf("non-retryable failure should run once, got %d", calls)
	}
	if len(results) != 1 || results[0].Status != StatusFailed {
		t.Fatalf("unexpected result: %+v", results)
	}
}
