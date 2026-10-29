package loop

import (
	"ai-meeting/clients"
	"ai-meeting/models"
	"ai-meeting/services/agent/tools"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type echoTool struct{}

func (e *echoTool) Name() string                { return "echo" }
func (e *echoTool) Description() string         { return "echo" }
func (e *echoTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (e *echoTool) Execute(ctx context.Context, _ *tools.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "echo-ok"}, nil
}

// Meta 声明为只读且可并发，供并行批次测试使用。
func (e *echoTool) Meta() tools.ToolMeta {
	return tools.ToolMeta{ReadOnly: true, ConcurrencySafe: true}
}

type failTool struct{}

func (f *failTool) Name() string                { return "fail" }
func (f *failTool) Description() string         { return "fail" }
func (f *failTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *failTool) Execute(ctx context.Context, _ *tools.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{}, errors.New("tool failed")
}

// Meta 声明为只读且可并发；失败是确定性的，没有副作用。
func (f *failTool) Meta() tools.ToolMeta {
	return tools.ToolMeta{ReadOnly: true, ConcurrencySafe: true}
}

// barrierTool 在执行时通知 started 并等待 release，用于验证批次是否真的并行。
type barrierTool struct {
	name    string
	started chan<- string
	release <-chan struct{}
}

func (b *barrierTool) Name() string                { return b.name }
func (b *barrierTool) Description() string         { return "barrier" }
func (b *barrierTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (b *barrierTool) Meta() tools.ToolMeta        { return tools.ToolMeta{ConcurrencySafe: true} }
func (b *barrierTool) Execute(ctx context.Context, _ *tools.Context, args json.RawMessage) (tools.Result, error) {
	b.started <- b.name
	<-b.release
	return tools.Result{Content: b.name + "-ok"}, nil
}

type recordingRecorder struct{ steps []models.AgentStep }

func (r *recordingRecorder) RecordStep(step *models.AgentStep) error {
	r.steps = append(r.steps, *step)
	return nil
}

func withFakeLLM(t *testing.T, fn func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error)) {
	t.Helper()
	orig := callLLM
	callLLM = func(ctx context.Context, aiID uint, messages []clients.ToolMessage, toolDefs []clients.ToolDef, temperature float64, onDelta func(clients.ToolStreamDelta) error) (*clients.ToolChatResult, error) {
		return fn(ctx, messages)
	}
	t.Cleanup(func() { callLLM = orig })
}

func newLoopWithTools(t *testing.T, toolList []tools.Tool, cfg Config) (*Loop, *recordingRecorder) {
	t.Helper()
	reg := tools.NewRegistry()
	for _, tool := range toolList {
		if tool != nil {
			reg.Register(tool)
		}
	}
	rec := &recordingRecorder{}
	return New(reg, tools.NewExecutor(reg, time.Second, 4096), cfg, rec), rec
}

func newLoop(t *testing.T, tool tools.Tool, cfg Config) (*Loop, *recordingRecorder) {
	t.Helper()
	var list []tools.Tool
	if tool != nil {
		list = append(list, tool)
	}
	return newLoopWithTools(t, list, cfg)
}

func TestRun_FinalAnswer(t *testing.T) {
	withFakeLLM(t, func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error) {
		return &clients.ToolChatResult{Content: "final answer", FinishReason: "stop"}, nil
	})
	l, rec := newLoop(t, nil, Config{MaxSteps: 4, TotalTimeout: time.Second})
	res, err := l.Run(context.Background(), &tools.Context{}, nil, "hi", nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.StopReason != "final_answer" || res.Content != "final answer" || res.Steps != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(rec.steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(rec.steps))
	}
}

func TestRun_ToolThenFinal(t *testing.T) {
	calls := 0
	withFakeLLM(t, func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error) {
		calls++
		if calls == 1 {
			return &clients.ToolChatResult{
				ToolCalls: []clients.ToolCall{{
					ID: "call-1", Type: "function",
					Function: clients.ToolCallFunction{Name: "echo", Arguments: "{}"},
				}},
				FinishReason: "tool_calls",
			}, nil
		}
		return &clients.ToolChatResult{Content: "done after tool", FinishReason: "stop"}, nil
	})
	l, rec := newLoop(t, &echoTool{}, Config{MaxSteps: 4, TotalTimeout: time.Second})
	res, err := l.Run(context.Background(), &tools.Context{}, nil, "hi", nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.StopReason != "final_answer" || res.Steps != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(rec.steps) != 2 || rec.steps[0].Action != "echo" || rec.steps[0].Observation != "[echo] echo-ok" {
		t.Fatalf("unexpected steps: %+v", rec.steps)
	}
}

func TestRun_MaxSteps(t *testing.T) {
	withFakeLLM(t, func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error) {
		return &clients.ToolChatResult{
			ToolCalls: []clients.ToolCall{{
				ID: "c", Type: "function",
				Function: clients.ToolCallFunction{Name: "echo", Arguments: "{}"},
			}},
			FinishReason: "tool_calls",
		}, nil
	})
	l, _ := newLoop(t, &echoTool{}, Config{MaxSteps: 3, MaxRepeatActions: 100, TotalTimeout: time.Second})
	res, err := l.Run(context.Background(), &tools.Context{}, nil, "hi", nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.StopReason != "max_steps" || res.Steps != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRun_RepeatedAction(t *testing.T) {
	withFakeLLM(t, func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error) {
		return &clients.ToolChatResult{
			ToolCalls: []clients.ToolCall{{
				ID: "same", Type: "function",
				Function: clients.ToolCallFunction{Name: "echo", Arguments: `{"x":1}`},
			}},
			FinishReason: "tool_calls",
		}, nil
	})
	l, rec := newLoop(t, &echoTool{}, Config{MaxSteps: 10, MaxRepeatActions: 2, TotalTimeout: time.Second})
	res, err := l.Run(context.Background(), &tools.Context{}, nil, "hi", nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.StopReason != "repeated_action" {
		t.Fatalf("want repeated_action, got %+v", res)
	}
	if len(rec.steps) != 2 {
		t.Fatalf("repeated_action should record the stopped step, got %d steps", len(rec.steps))
	}
	last := rec.steps[len(rec.steps)-1]
	if last.Status != models.AgentStepStatusFailed || last.Error != "repeated_action" {
		t.Fatalf("unexpected stopped step: %+v", last)
	}
}

func TestRun_NonConsecutiveActionsNotRepeated(t *testing.T) {
	step := 0
	withFakeLLM(t, func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error) {
		step++
		switch step {
		case 1, 3:
			return &clients.ToolChatResult{
				ToolCalls: []clients.ToolCall{{
					ID: "a", Type: "function",
					Function: clients.ToolCallFunction{Name: "echo", Arguments: `{"x":1}`},
				}},
				FinishReason: "tool_calls",
			}, nil
		case 2:
			return &clients.ToolChatResult{
				ToolCalls: []clients.ToolCall{{
					ID: "b", Type: "function",
					Function: clients.ToolCallFunction{Name: "echo", Arguments: `{"x":2}`},
				}},
				FinishReason: "tool_calls",
			}, nil
		default:
			return &clients.ToolChatResult{Content: "done", FinishReason: "stop"}, nil
		}
	})
	l, _ := newLoop(t, &echoTool{}, Config{MaxSteps: 10, MaxRepeatActions: 2, TotalTimeout: time.Second})
	res, err := l.Run(context.Background(), &tools.Context{}, nil, "hi", nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.StopReason != "final_answer" {
		t.Fatalf("non-consecutive actions should not trigger repeated_action: %+v", res)
	}
}

func TestRun_ToolFailures(t *testing.T) {
	withFakeLLM(t, func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error) {
		return &clients.ToolChatResult{
			ToolCalls: []clients.ToolCall{{
				ID: "f", Type: "function",
				Function: clients.ToolCallFunction{Name: "fail", Arguments: "{}"},
			}},
			FinishReason: "tool_calls",
		}, nil
	})
	l, _ := newLoop(t, &failTool{}, Config{MaxSteps: 10, MaxRepeatActions: 100, MaxConsecutiveToolFailures: 2, TotalTimeout: time.Second})
	res, err := l.Run(context.Background(), &tools.Context{}, nil, "hi", nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.StopReason != "tool_failures" {
		t.Fatalf("want tool_failures, got %+v", res)
	}
}

func TestRun_ParallelToolCalls(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	llmCalls := 0

	withFakeLLM(t, func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error) {
		llmCalls++
		if llmCalls == 1 {
			return &clients.ToolChatResult{
				ToolCalls: []clients.ToolCall{
					{ID: "c1", Type: "function", Function: clients.ToolCallFunction{Name: "a", Arguments: "{}"}},
					{ID: "c2", Type: "function", Function: clients.ToolCallFunction{Name: "b", Arguments: "{}"}},
				},
				FinishReason: "tool_calls",
			}, nil
		}
		return &clients.ToolChatResult{Content: "done", FinishReason: "stop"}, nil
	})

	l, _ := newLoopWithTools(t, []tools.Tool{
		&barrierTool{name: "a", started: started, release: release},
		&barrierTool{name: "b", started: started, release: release},
	}, Config{MaxSteps: 4, MaxRepeatActions: 100, MaxParallelToolCalls: 2, TotalTimeout: 5 * time.Second})

	type runResult struct {
		res *Result
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		res, err := l.Run(context.Background(), &tools.Context{}, nil, "hi", nil)
		done <- runResult{res: res, err: err}
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("tool calls did not start in parallel")
		}
	}
	close(release)

	r := <-done
	if r.err != nil {
		t.Fatalf("run failed: %v", r.err)
	}
	if r.res.StopReason != "final_answer" {
		t.Fatalf("want final_answer, got %+v", r.res)
	}
}

func TestRun_ParallelToolFailureDoesNotAbortSiblings(t *testing.T) {
	llmCalls := 0
	withFakeLLM(t, func(ctx context.Context, messages []clients.ToolMessage) (*clients.ToolChatResult, error) {
		llmCalls++
		if llmCalls == 1 {
			return &clients.ToolChatResult{
				ToolCalls: []clients.ToolCall{
					{ID: "c1", Type: "function", Function: clients.ToolCallFunction{Name: "echo", Arguments: "{}"}},
					{ID: "c2", Type: "function", Function: clients.ToolCallFunction{Name: "fail", Arguments: "{}"}},
				},
				FinishReason: "tool_calls",
			}, nil
		}
		return &clients.ToolChatResult{Content: "recovered", FinishReason: "stop"}, nil
	})

	l, rec := newLoopWithTools(t, []tools.Tool{&echoTool{}, &failTool{}},
		Config{MaxSteps: 4, MaxRepeatActions: 100, MaxConsecutiveToolFailures: 3, MaxParallelToolCalls: 2, TotalTimeout: 2 * time.Second})

	res, err := l.Run(context.Background(), &tools.Context{}, nil, "hi", nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.StopReason != "final_answer" || res.Content != "recovered" {
		t.Fatalf("one failed tool should not abort the batch, got %+v", res)
	}
	if len(rec.steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(rec.steps))
	}
	if !strings.Contains(rec.steps[0].Observation, "echo-ok") ||
		!strings.Contains(rec.steps[0].Observation, "工具执行失败") {
		t.Fatalf("step observation should contain both outcomes, got %q", rec.steps[0].Observation)
	}
}
