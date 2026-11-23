package callback

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
)

// orderHandler 记录收到的事件名，用于验证 Multi 的顺序与 nil 跳过。
type orderHandler struct {
	Base
	mu    sync.Mutex
	names []string
}

func (h *orderHandler) add(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.names = append(h.names, name)
}

func (h *orderHandler) OnRunStart(context.Context, RunStartEvent)   { h.add("run_start") }
func (h *orderHandler) OnRunEnd(context.Context, RunEndEvent)       { h.add("run_end") }
func (h *orderHandler) OnLLMStart(context.Context, LLMStartEvent)   { h.add("llm_start") }
func (h *orderHandler) OnLLMEnd(context.Context, LLMEndEvent)       { h.add("llm_end") }
func (h *orderHandler) OnToolStart(context.Context, ToolStartEvent) { h.add("tool_start") }
func (h *orderHandler) OnToolEnd(context.Context, ToolEndEvent)     { h.add("tool_end") }

func TestMulti_DispatchesInOrderAndSkipsNil(t *testing.T) {
	a := &orderHandler{}
	b := &orderHandler{}
	var m Multi = Multi{a, nil, b}
	ctx := context.Background()

	m.OnRunStart(ctx, RunStartEvent{})
	m.OnRunEnd(ctx, RunEndEvent{})
	m.OnLLMStart(ctx, LLMStartEvent{})
	m.OnLLMEnd(ctx, LLMEndEvent{})
	m.OnToolStart(ctx, ToolStartEvent{})
	m.OnToolEnd(ctx, ToolEndEvent{})

	want := "run_start,run_end,llm_start,llm_end,tool_start,tool_end"
	for name, h := range map[string]*orderHandler{"a": a, "b": b} {
		if got := strings.Join(h.names, ","); got != want {
			t.Fatalf("handler %s got %q, want %q", name, got, want)
		}
	}
}

func TestOrNop_NilIsSafe(t *testing.T) {
	var h Handler
	h = OrNop(h)
	if h == nil {
		t.Fatal("OrNop should return a non-nil handler")
	}
	// 空实现不应 panic。
	h.OnRunStart(context.Background(), RunStartEvent{})
	h.OnRunEnd(context.Background(), RunEndEvent{})
	h.OnLLMStart(context.Background(), LLMStartEvent{})
	h.OnLLMEnd(context.Background(), LLMEndEvent{})
	h.OnToolStart(context.Background(), ToolStartEvent{})
	h.OnToolEnd(context.Background(), ToolEndEvent{})
}

func newTestLogger() (*logrus.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := logrus.New()
	logger.SetOutput(buf)
	logger.SetLevel(logrus.DebugLevel)
	logger.SetFormatter(&logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})
	return logger, buf
}

func TestLogging_EmitsMetadata(t *testing.T) {
	logger, buf := newTestLogger()
	h := NewLogging(logger)
	ctx := context.Background()

	h.OnRunStart(ctx, RunStartEvent{RunID: "r1", SessionID: "s1", AgentID: 7, InputRunes: 12})
	h.OnLLMStart(ctx, LLMStartEvent{RunID: "r1", SessionID: "s1", Step: 1, MessageCount: 3, ToolCount: 2})
	h.OnLLMEnd(ctx, LLMEndEvent{RunID: "r1", SessionID: "s1", Step: 1, Tokens: 99, ToolCalls: []string{"echo"}})
	h.OnToolStart(ctx, ToolStartEvent{RunID: "r1", SessionID: "s1", Step: 1, CallID: "c1", ToolName: "echo", ArgsBytes: 4})
	h.OnToolEnd(ctx, ToolEndEvent{RunID: "r1", SessionID: "s1", Step: 1, CallID: "c1", ToolName: "echo", ResultBytes: 8, Success: true})
	h.OnRunEnd(ctx, RunEndEvent{RunID: "r1", SessionID: "s1", Steps: 2, Tokens: 100, StopReason: "final_answer"})

	out := buf.String()
	for _, want := range []string{
		"[agent-callback] run start", "run_id=r1", "session_id=s1",
		"[agent-callback] llm start", "message_count=3",
		"[agent-callback] llm end", "tokens=99",
		"[agent-callback] tool start", "tool=echo",
		"[agent-callback] tool end", "success=true",
		"[agent-callback] run end", "stop_reason=final_answer",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestLogging_WarnsOnError(t *testing.T) {
	logger, buf := newTestLogger()
	h := NewLogging(logger)
	ctx := context.Background()

	h.OnLLMEnd(ctx, LLMEndEvent{RunID: "r1", Step: 2, Err: errors.New("boom")})
	h.OnToolEnd(ctx, ToolEndEvent{RunID: "r1", Step: 2, ToolName: "echo", Err: errors.New("tool down")})
	h.OnRunEnd(ctx, RunEndEvent{RunID: "r1", StopReason: "llm_error", Err: errors.New("boom")})

	out := buf.String()
	for _, want := range []string{
		"level=warning", "llm end with error", "error=boom",
		"tool end with error", "error=\"tool down\"",
		"run end with error", "stop_reason=llm_error",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestNewLogging_NilLoggerFallsBackToStandard(t *testing.T) {
	h := NewLogging(nil)
	if h.logger == nil {
		t.Fatal("logger should fall back to standard logger")
	}
}
