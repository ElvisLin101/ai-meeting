package tools

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ai-meeting/clients"
)

// safeFakeTool 在 fakeTool 基础上声明只读且可并发。
type safeFakeTool struct {
	fakeTool
}

func (f *safeFakeTool) Meta() ToolMeta {
	return ToolMeta{ReadOnly: true, ConcurrencySafe: true}
}

func testCall(id, name string) clients.ToolCall {
	return clients.ToolCall{
		ID:   id,
		Type: "function",
		Function: clients.ToolCallFunction{
			Name:      name,
			Arguments: "{}",
		},
	}
}

func TestBatchExecutor_ParallelWhenAllSafe(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})

	reg := NewRegistry()
	for _, name := range []string{"a", "b"} {
		name := name
		reg.Register(&safeFakeTool{fakeTool{
			name:   name,
			params: json.RawMessage(`{"type":"object"}`),
			fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
				started <- name
				<-release
				return Result{Content: name + "-ok"}, nil
			},
		}})
	}
	exec := NewExecutor(reg, time.Second, 1024)
	batch := NewBatchExecutor(reg, exec, 2)

	done := make(chan []CallOutcome, 1)
	go func() {
		done <- batch.Execute(context.Background(), &Context{},
			[]clients.ToolCall{testCall("c1", "a"), testCall("c2", "b")})
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("tools did not start in parallel; batch was likely serialized")
		}
	}
	close(release)

	outcomes := <-done
	if len(outcomes) != 2 {
		t.Fatalf("want 2 outcomes, got %d", len(outcomes))
	}
	for i, want := range []string{"a-ok", "b-ok"} {
		if outcomes[i].Err != nil {
			t.Fatalf("outcome %d failed: %v", i, outcomes[i].Err)
		}
		if outcomes[i].Result.Content != want {
			t.Fatalf("outcome %d content = %q, want %q", i, outcomes[i].Result.Content, want)
		}
	}
}

func TestBatchExecutor_SerialWhenAnyToolUnsafe(t *testing.T) {
	var active, peak int32
	reg := NewRegistry()
	reg.Register(&fakeTool{
		name:   "unsafe",
		params: json.RawMessage(`{"type":"object"}`),
		fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
			n := atomic.AddInt32(&active, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			return Result{Content: "ok"}, nil
		},
	})
	exec := NewExecutor(reg, time.Second, 1024)
	batch := NewBatchExecutor(reg, exec, 4)

	outcomes := batch.Execute(context.Background(), &Context{}, []clients.ToolCall{
		testCall("c1", "unsafe"), testCall("c2", "unsafe"), testCall("c3", "unsafe"),
	})
	for i, outcome := range outcomes {
		if outcome.Err != nil {
			t.Fatalf("outcome %d failed: %v", i, outcome.Err)
		}
	}
	if got := atomic.LoadInt32(&peak); got != 1 {
		t.Fatalf("unsafe tools must run serially, peak concurrency = %d", got)
	}
}

func TestBatchExecutor_PreservesCallOrder(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&safeFakeTool{fakeTool{
		name:   "slow",
		params: json.RawMessage(`{"type":"object"}`),
		fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
			time.Sleep(40 * time.Millisecond)
			return Result{Content: "slow-ok"}, nil
		},
	}})
	reg.Register(&safeFakeTool{fakeTool{
		name:   "fast",
		params: json.RawMessage(`{"type":"object"}`),
		fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
			return Result{Content: "fast-ok"}, nil
		},
	}})
	exec := NewExecutor(reg, time.Second, 1024)
	batch := NewBatchExecutor(reg, exec, 2)

	outcomes := batch.Execute(context.Background(), &Context{}, []clients.ToolCall{
		testCall("c1", "slow"), testCall("c2", "fast"),
	})
	if outcomes[0].Call.ID != "c1" || outcomes[1].Call.ID != "c2" {
		t.Fatalf("outcomes must follow input order, got %q then %q", outcomes[0].Call.ID, outcomes[1].Call.ID)
	}
}

func TestBatchExecutor_FailureIsolation(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&safeFakeTool{fakeTool{
		name:   "bad",
		params: json.RawMessage(`{"type":"object"}`),
		fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
			return Result{}, errors.New("boom")
		},
	}})
	reg.Register(&safeFakeTool{fakeTool{
		name:   "good",
		params: json.RawMessage(`{"type":"object"}`),
		fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
			return Result{Content: "good-ok"}, nil
		},
	}})
	exec := NewExecutor(reg, time.Second, 1024)
	batch := NewBatchExecutor(reg, exec, 2)

	outcomes := batch.Execute(context.Background(), &Context{}, []clients.ToolCall{
		testCall("c1", "bad"), testCall("c2", "good"),
	})
	if outcomes[0].Err == nil {
		t.Fatal("bad tool should return an error")
	}
	if outcomes[1].Err != nil || outcomes[1].Result.Content != "good-ok" {
		t.Fatalf("good tool should still succeed, got err=%v content=%q", outcomes[1].Err, outcomes[1].Result.Content)
	}
}

func TestBatchExecutor_CanceledContext(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&safeFakeTool{fakeTool{
		name:   "a",
		params: json.RawMessage(`{"type":"object"}`),
		fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
			return Result{Content: "ok"}, nil
		},
	}})
	exec := NewExecutor(reg, time.Second, 1024)
	batch := NewBatchExecutor(reg, exec, 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcomes := batch.Execute(ctx, &Context{}, []clients.ToolCall{testCall("c1", "a")})
	if len(outcomes) != 1 || outcomes[0].Err == nil {
		t.Fatalf("canceled context should return an error outcome, got %+v", outcomes)
	}
}

func TestBatchExecutor_RespectsMaxParallel(t *testing.T) {
	var active, peak int32
	reg := NewRegistry()
	reg.Register(&safeFakeTool{fakeTool{
		name:   "a",
		params: json.RawMessage(`{"type":"object"}`),
		fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
			n := atomic.AddInt32(&active, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			return Result{Content: "ok"}, nil
		},
	}})
	exec := NewExecutor(reg, time.Second, 1024)
	batch := NewBatchExecutor(reg, exec, 2)

	outcomes := batch.Execute(context.Background(), &Context{}, []clients.ToolCall{
		testCall("c1", "a"), testCall("c2", "a"), testCall("c3", "a"),
	})
	for i, outcome := range outcomes {
		if outcome.Err != nil {
			t.Fatalf("outcome %d failed: %v", i, outcome.Err)
		}
	}
	if got := atomic.LoadInt32(&peak); got > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", got)
	}
}
