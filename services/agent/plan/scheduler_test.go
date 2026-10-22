package plan

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func agentTask(id string, deps ...string) Task {
	return Task{ID: id, Type: TaskTypeAgent, Goal: id, DependsOn: deps}
}

func TestSchedulerDependencyOrder(t *testing.T) {
	p := &Plan{Tasks: []Task{
		agentTask("t1"),
		agentTask("t2"),
		agentTask("t3", "t1", "t2"),
	}}

	var mu sync.Mutex
	var order []string
	results := (&Scheduler{MaxParallel: 2}).Run(context.Background(), p,
		func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
			mu.Lock()
			order = append(order, task.ID)
			mu.Unlock()
			return TaskResult{ID: task.ID, Status: StatusDone, Summary: task.ID + "-ok"}
		})

	if len(order) != 3 {
		t.Fatalf("want 3 executions, got %v", order)
	}
	if order[2] != "t3" {
		t.Fatalf("t3 must run after t1/t2, order=%v", order)
	}
	if len(results) != 3 || results[2].ID != "t3" || results[2].Status != StatusDone {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSchedulerRunsIndependentTasksInParallel(t *testing.T) {
	p := &Plan{Tasks: []Task{agentTask("t1"), agentTask("t2")}}
	started := make(chan string, 2)
	release := make(chan struct{})

	done := make(chan []TaskResult, 1)
	go func() {
		done <- (&Scheduler{MaxParallel: 2}).Run(context.Background(), p,
			func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
				started <- task.ID
				<-release
				return TaskResult{ID: task.ID, Status: StatusDone}
			})
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("independent tasks did not start in parallel")
		}
	}
	close(release)

	results := <-done
	if len(results) != 2 || results[0].Status != StatusDone || results[1].Status != StatusDone {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSchedulerFailureSkipsDependents(t *testing.T) {
	p := &Plan{Tasks: []Task{
		agentTask("t1"),
		agentTask("t2"),
		agentTask("t3", "t1"),
		agentTask("t4", "t3"),
	}}

	results := (&Scheduler{MaxParallel: 2}).Run(context.Background(), p,
		func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
			if task.ID == "t1" {
				return TaskResult{ID: task.ID, Status: StatusFailed, Error: "boom"}
			}
			return TaskResult{ID: task.ID, Status: StatusDone, Summary: "ok"}
		})

	byID := map[string]TaskResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	if byID["t1"].Status != StatusFailed {
		t.Fatalf("t1 should fail, got %+v", byID["t1"])
	}
	if byID["t2"].Status != StatusDone {
		t.Fatalf("independent t2 should still run, got %+v", byID["t2"])
	}
	if byID["t3"].Status != StatusSkipped || byID["t4"].Status != StatusSkipped {
		t.Fatalf("dependents should be skipped, got t3=%s t4=%s", byID["t3"].Status, byID["t4"].Status)
	}
}

func TestSchedulerCanceledContext(t *testing.T) {
	p := &Plan{Tasks: []Task{agentTask("t1"), agentTask("t2")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int32
	results := (&Scheduler{MaxParallel: 2}).Run(ctx, p,
		func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
			atomic.AddInt32(&calls, 1)
			return TaskResult{ID: task.ID, Status: StatusDone}
		})

	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("canceled scheduler should not execute tasks, got %d calls", calls)
	}
	for _, r := range results {
		if r.Status != StatusCanceled {
			t.Fatalf("want canceled, got %+v", r)
		}
	}
}

func TestSchedulerPanicRecovered(t *testing.T) {
	p := &Plan{Tasks: []Task{agentTask("t1"), agentTask("t2")}}
	results := (&Scheduler{MaxParallel: 2}).Run(context.Background(), p,
		func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
			if task.ID == "t1" {
				panic("boom")
			}
			return TaskResult{ID: task.ID, Status: StatusDone}
		})

	byID := map[string]TaskResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	if byID["t1"].Status != StatusFailed {
		t.Fatalf("panic should become failed, got %+v", byID["t1"])
	}
	if byID["t2"].Status != StatusDone {
		t.Fatalf("sibling should still run, got %+v", byID["t2"])
	}
}

func TestSchedulerMaxParallel(t *testing.T) {
	p := &Plan{Tasks: []Task{agentTask("t1"), agentTask("t2"), agentTask("t3")}}
	var active, peak int32

	results := (&Scheduler{MaxParallel: 2}).Run(context.Background(), p,
		func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
			n := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&peak)
				if n <= old || atomic.CompareAndSwapInt32(&peak, old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			return TaskResult{ID: task.ID, Status: StatusDone}
		})

	for _, r := range results {
		if r.Status != StatusDone {
			t.Fatalf("task failed: %+v", r)
		}
	}
	if got := atomic.LoadInt32(&peak); got > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", got)
	}
}

func TestSchedulerPassesUpstream(t *testing.T) {
	p := &Plan{Tasks: []Task{
		agentTask("t1"),
		agentTask("t2", "t1"),
	}}

	var got string
	results := (&Scheduler{MaxParallel: 2}).Run(context.Background(), p,
		func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
			if task.ID == "t2" {
				got = upstream["t1"].Summary
			}
			return TaskResult{ID: task.ID, Status: StatusDone, Summary: task.ID + "-summary"}
		})

	if len(results) != 2 || got != "t1-summary" {
		t.Fatalf("upstream not passed, got %q results=%+v", got, results)
	}
}

func TestSchedulerTaskTimeout(t *testing.T) {
	p := &Plan{Tasks: []Task{agentTask("t1")}}
	results := (&Scheduler{MaxParallel: 1, TaskTimeout: 20 * time.Millisecond}).Run(context.Background(), p,
		func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
			<-ctx.Done()
			return TaskResult{ID: task.ID, Status: StatusFailed, Error: ctx.Err().Error()}
		})

	if len(results) != 1 || results[0].Status != StatusFailed {
		t.Fatalf("want timeout failure, got %+v", results)
	}
}
