package run

import (
	"strings"
	"sync/atomic"
	"testing"

	"ai-meeting/config"
	"ai-meeting/models"
	"ai-meeting/services/agent/context"
)

func TestContextPromptBudget(t *testing.T) {
	if got := contextPromptBudget(config.ContextConfig{}); got != 12000 {
		t.Fatalf("zero config should fall back to 12000, got %d", got)
	}
	got := contextPromptBudget(config.ContextConfig{
		ModelWindowTokens:  64000,
		MaxOutputTokens:    4096,
		SafetyMarginTokens: 1024,
		ReserveTokens:      2048,
		SoftLimitTokens:    16000,
	})
	if got != 16000 {
		t.Fatalf("soft limit should cap budget, got %d", got)
	}
	got = contextPromptBudget(config.ContextConfig{
		ModelWindowTokens:  8000,
		MaxOutputTokens:    4096,
		SafetyMarginTokens: 1024,
		ReserveTokens:      2048,
		SoftLimitTokens:    16000,
	})
	if got != 832 {
		t.Fatalf("budget = window-output-safety-reserve = 832, got %d", got)
	}
}

func TestDedupKey(t *testing.T) {
	a := StartRequest{Username: "u1", SessionID: "s1", Input: "hi"}
	if dedupKey(a) != dedupKey(a) {
		t.Fatal("dedup key should be stable")
	}
	b := StartRequest{Username: "u2", SessionID: "s1", Input: "hi"}
	if dedupKey(a) == dedupKey(b) {
		t.Fatal("different users should have different keys")
	}
}

func TestValidate(t *testing.T) {
	if err := validate(StartRequest{UserID: "u", Username: "u", Input: "hi"}); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := validate(StartRequest{Username: "u", Input: "hi"}); err == nil {
		t.Fatal("missing user id should fail")
	}
	if err := validate(StartRequest{UserID: "u", Username: "u"}); err == nil {
		t.Fatal("empty input should fail")
	}
	if err := validate(StartRequest{UserID: "u", Username: "u", Input: strings.Repeat("中", 4001)}); err == nil {
		t.Fatal("oversized input should fail")
	}
}

func TestTruncateTitle(t *testing.T) {
	if got := truncateTitle("  hi  "); got != "hi" {
		t.Fatalf("unexpected title: %q", got)
	}
	long := strings.Repeat("中", 40)
	if got := truncateTitle(long); len([]rune(got)) != 30 {
		t.Fatalf("title should be truncated to 30 runes, got %d", len([]rune(got)))
	}
	if got := truncateTitle("   "); got != "新会话" {
		t.Fatalf("empty title should fallback, got %q", got)
	}
}

func TestBuildContextHint(t *testing.T) {
	assembled := &agentctx.Assembled{Messages: []agentctx.Message{
		{Role: "system", Content: "identity"},
		{Role: "user", Content: "old user message"},
		{Role: "system", Content: "profile"},
	}}
	hint := buildContextHint(assembled)
	if !strings.Contains(hint, "identity") || !strings.Contains(hint, "profile") {
		t.Fatalf("system messages missing: %q", hint)
	}
	if strings.Contains(hint, "old user message") {
		t.Fatalf("recent user message should not be in hint: %q", hint)
	}
}

func TestDecodeStartResult(t *testing.T) {
	want := &StartResult{RunID: "r1", Status: "completed", PlanMode: "direct"}
	got, err := decodeStartResult(want)
	if err != nil || got != want {
		t.Fatalf("pointer result should pass through: %v %+v", err, got)
	}

	got, err = decodeStartResult(map[string]interface{}{
		"run_id": "r2", "status": "completed", "plan_mode": "plan", "steps": float64(2),
	})
	if err != nil {
		t.Fatalf("map result should decode: %v", err)
	}
	if got.RunID != "r2" || got.PlanMode != "plan" || got.Steps != 2 {
		t.Fatalf("unexpected decoded result: %+v", got)
	}

	if _, err := decodeStartResult(nil); err == nil {
		t.Fatal("nil result should error")
	}
	if _, err := decodeStartResult("bad"); err == nil {
		t.Fatal("unexpected type should error")
	}
}

func TestStepRecorderPreparePlanStep(t *testing.T) {
	var seq atomic.Int64
	rec := &stepRecorder{runID: "run-1", taskID: "t1", seq: &seq}

	first := &models.AgentStep{StepIndex: 1}
	second := &models.AgentStep{StepIndex: 1}
	rec.prepare(first)
	rec.prepare(second)

	if first.RunID != "run-1" || first.TaskID != "t1" || first.StepIndex != 1 {
		t.Fatalf("unexpected first step: %+v", first)
	}
	if second.StepIndex != 2 {
		t.Fatalf("plan steps should share a run-level sequence, got %d", second.StepIndex)
	}
}

func TestStepRecorderKeepsDirectStepIndex(t *testing.T) {
	rec := &stepRecorder{runID: "run-2"}
	step := &models.AgentStep{StepIndex: 3}
	rec.prepare(step)

	if step.RunID != "run-2" || step.TaskID != "" || step.StepIndex != 3 {
		t.Fatalf("direct run should keep loop step index: %+v", step)
	}
}

func TestBuildRegistryRegistersDeepSeekSearchAndFetch(t *testing.T) {
	oldKey := config.AppConfig.AI.DeepSeek.APIKey
	config.AppConfig.AI.DeepSeek.APIKey = "test-key"
	t.Cleanup(func() { config.AppConfig.AI.DeepSeek.APIKey = oldKey })

	registry := buildRegistry(config.AgentConfig{
		SandboxRoot: t.TempDir(),
		Search: config.SearchConfig{
			Provider:   "deepseek",
			MaxResults: 5,
		},
	})
	for _, name := range []string{"web_search", "web_fetch"} {
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("tool %s should be registered", name)
		}
	}
}
