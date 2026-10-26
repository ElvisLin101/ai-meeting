package plan

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ai-meeting/models"
	"ai-meeting/services/agent/loop"
	"ai-meeting/services/agent/tools"
)

type testRecorder struct {
	steps []models.AgentStep
}

func (r *testRecorder) RecordStep(step *models.AgentStep) error {
	r.steps = append(r.steps, *step)
	return nil
}

type testTool struct {
	err error
}

func (t *testTool) Name() string        { return "echo" }
func (t *testTool) Description() string { return "echo text" }
func (t *testTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`)
}
func (t *testTool) Execute(_ context.Context, _ *tools.Context, args json.RawMessage) (tools.Result, error) {
	if t.err != nil {
		return tools.Result{}, t.err
	}
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(args, &in)
	return tools.Result{Content: "echo:" + in.Text}, nil
}

func newTestTaskService(tool *testTool) *Service {
	registry := tools.NewRegistry()
	registry.Register(tool)
	return NewService(Config{Enabled: true, MaxTasks: 4, TaskTimeout: time.Second}, registry,
		tools.NewExecutor(registry, time.Second, 1024))
}

func TestRunToolTaskRecordsStep(t *testing.T) {
	svc := newTestTaskService(&testTool{})
	rec := &testRecorder{}
	var gotTaskID string

	res := svc.runTask(context.Background(), Input{
		UserID: "u", Username: "u", SessionID: "s", RunID: "r",
		RecorderFor: func(taskID string) loop.Recorder {
			gotTaskID = taskID
			return rec
		},
	}, Task{
		ID: "t1", Type: TaskTypeTool, Tool: "echo", Goal: "say hi",
		Params: map[string]interface{}{"text": "hi"},
	}, nil)

	if res.Status != StatusDone {
		t.Fatalf("task should succeed, got %+v", res)
	}
	if gotTaskID != "t1" {
		t.Fatalf("recorder factory got task id %q, want t1", gotTaskID)
	}
	if len(rec.steps) != 1 {
		t.Fatalf("expected 1 recorded step, got %d", len(rec.steps))
	}
	step := rec.steps[0]
	if step.StepIndex != 1 || step.Action != "echo" || step.Status != models.AgentStepStatusOK {
		t.Fatalf("unexpected step: %+v", step)
	}
	if step.Observation != "echo:hi" {
		t.Fatalf("unexpected observation: %q", step.Observation)
	}
	if step.Thought != "say hi" {
		t.Fatalf("unexpected thought: %q", step.Thought)
	}
}

func TestRunToolTaskRecordsFailure(t *testing.T) {
	svc := newTestTaskService(&testTool{err: errors.New("boom")})
	rec := &testRecorder{}

	res := svc.runTask(context.Background(), Input{
		UserID: "u", Username: "u", SessionID: "s", RunID: "r",
		RecorderFor: func(string) loop.Recorder { return rec },
	}, Task{
		ID: "t2", Type: TaskTypeTool, Tool: "echo", Goal: "fail",
		Params: map[string]interface{}{"text": "hi"},
	}, nil)

	if res.Status != StatusFailed {
		t.Fatalf("task should fail, got %+v", res)
	}
	if len(rec.steps) != 1 {
		t.Fatalf("expected 1 recorded step, got %d", len(rec.steps))
	}
	step := rec.steps[0]
	if step.Status != models.AgentStepStatusFailed || step.Error == "" {
		t.Fatalf("failure step not recorded correctly: %+v", step)
	}
}

func TestRunToolTaskWithoutRecorder(t *testing.T) {
	svc := newTestTaskService(&testTool{})
	res := svc.runTask(context.Background(), Input{
		UserID: "u", Username: "u", SessionID: "s", RunID: "r",
	}, Task{
		ID: "t3", Type: TaskTypeTool, Tool: "echo", Goal: "say hi",
		Params: map[string]interface{}{"text": "hi"},
	}, nil)
	if res.Status != StatusDone {
		t.Fatalf("task should succeed without recorder, got %+v", res)
	}
}
