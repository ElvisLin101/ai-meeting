package agentctx

import (
	"context"
	"testing"
	"time"
)

func TestAsyncPoolShutdownWaitsForTasks(t *testing.T) {
	pool := newAsyncPool(context.Background())
	done := make(chan struct{})
	if !pool.Go(func(ctx context.Context) {
		time.Sleep(20 * time.Millisecond)
		close(done)
	}) {
		t.Fatal("task should be accepted")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case <-done:
	default:
		t.Fatal("shutdown returned before task completed")
	}
	if pool.Go(func(context.Context) {}) {
		t.Fatal("pool should reject tasks after shutdown")
	}
}

func TestAsyncPoolCancelsParentContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	pool := newAsyncPool(parent)
	started := make(chan struct{})
	stopped := make(chan struct{})
	if !pool.Go(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(stopped)
	}) {
		t.Fatal("task should be accepted")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("task did not start")
	}
	cancelParent()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("task did not observe parent cancellation")
	}
}
