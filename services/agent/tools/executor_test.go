package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ai-meeting/clients"
)

type fakeTool struct {
	name   string
	fn     func(ctx context.Context, args json.RawMessage) (Result, error)
	params json.RawMessage
}

func (f *fakeTool) Name() string                { return f.name }
func (f *fakeTool) Description() string         { return "fake tool" }
func (f *fakeTool) Parameters() json.RawMessage { return f.params }
func (f *fakeTool) Execute(ctx context.Context, _ *Context, args json.RawMessage) (Result, error) {
	return f.fn(ctx, args)
}

func TestExecutor_UnknownTool(t *testing.T) {
	exec := NewExecutor(NewRegistry(), time.Second, 1024)
	_, err := exec.Execute(context.Background(), &Context{}, clients.ToolCall{Function: clients.ToolCallFunction{Name: "nope"}})
	if err == nil {
		t.Fatal("unknown tool should fail")
	}
}

func TestExecutor_PanicRecovered(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&fakeTool{name: "boom", params: json.RawMessage(`{"type":"object"}`), fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
		panic("boom")
	}})
	exec := NewExecutor(reg, time.Second, 1024)
	_, err := exec.Execute(context.Background(), &Context{}, clients.ToolCall{Function: clients.ToolCallFunction{Name: "boom", Arguments: "{}"}})
	if err == nil {
		t.Fatal("panic should be converted to error")
	}
}

func TestExecutor_Timeout(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&fakeTool{name: "slow", params: json.RawMessage(`{"type":"object"}`), fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}})
	exec := NewExecutor(reg, 20*time.Millisecond, 1024)
	_, err := exec.Execute(context.Background(), &Context{}, clients.ToolCall{Function: clients.ToolCallFunction{Name: "slow", Arguments: "{}"}})
	if err == nil {
		t.Fatal("timeout should fail")
	}
}

func TestExecutor_TruncatesResult(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&fakeTool{name: "big", params: json.RawMessage(`{"type":"object"}`), fn: func(ctx context.Context, args json.RawMessage) (Result, error) {
		return Result{Content: strings.Repeat("中", 1000)}, nil
	}})
	exec := NewExecutor(reg, time.Second, 100)
	res, err := exec.Execute(context.Background(), &Context{}, clients.ToolCall{Function: clients.ToolCallFunction{Name: "big", Arguments: "{}"}})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !res.Truncated || len(res.Content) >= 1000 {
		t.Fatalf("result should be truncated, got len=%d truncated=%v", len(res.Content), res.Truncated)
	}
}

func TestTruncateRunesUsesByteBudget(t *testing.T) {
	ascii := truncateRunes(strings.Repeat("a", 1000), 100)
	if len(ascii) > 100 {
		t.Fatalf("ascii result exceeds byte budget: %d", len(ascii))
	}
	if len(ascii) < 90 {
		t.Fatalf("ascii result wastes byte budget: %d", len(ascii))
	}

	chinese := truncateRunes(strings.Repeat("中", 1000), 100)
	if len(chinese) > 100 {
		t.Fatalf("chinese result exceeds byte budget: %d", len(chinese))
	}
	if !utf8.ValidString(chinese) {
		t.Fatalf("chinese result is not valid UTF-8: %q", chinese)
	}
}
