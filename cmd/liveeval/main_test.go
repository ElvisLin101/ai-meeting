package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentrun "ai-meeting/services/agent/run"
)

func TestSanitizeLabel(t *testing.T) {
	cases := map[string]string{
		"eval-r1":        "eval-r1",
		"eval/r1":        "eval-r1",
		"  ":             "eval",
		"中文":             "eval",
		"run 1_ok":       "run-1_ok",
		"../escape/path": "escape-path",
	}
	for in, want := range cases {
		if got := sanitizeLabel(in); got != want {
			t.Fatalf("sanitizeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResetSandboxOnlyEvalRuns(t *testing.T) {
	if err := resetSandbox("/tmp/not-eval/sandbox"); err == nil {
		t.Fatal("non eval-runs path should be rejected")
	}

	root := filepath.Join(t.TempDir(), "eval-runs", "r1", "sandbox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "old.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := resetSandbox(root); err != nil {
		t.Fatalf("reset: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("sandbox should be empty, got %d entries", len(entries))
	}
	if !strings.Contains(filepath.ToSlash(root), "/eval-runs/") {
		t.Fatalf("test path should contain eval-runs: %s", root)
	}
}

func TestSeedCaseFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "eval-runs", "r1", "sandbox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := seedCaseFiles(root, map[string]string{
		"resume.md":    "resume content",
		"docs/jd.md":   "jd content",
		"nested/a.txt": "a",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for rel, want := range map[string]string{
		"resume.md":    "resume content",
		"docs/jd.md":   "jd content",
		"nested/a.txt": "a",
	} {
		got, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestSeedCaseFilesRejectsEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "eval-runs", "r1", "sandbox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, rel := range []string{"../escape.md", "../../escape.md", "/tmp/escape.md"} {
		if err := seedCaseFiles(root, map[string]string{rel: "x"}); err == nil {
			t.Fatalf("path %q should be rejected", rel)
		}
	}
}

func TestParseJudgePayload(t *testing.T) {
	got, err := parseJudgePayload("```json\n{\"items\":[true,false],\"reason\":\"ok\"}\n```", 2)
	if err != nil {
		t.Fatalf("parse bool items: %v", err)
	}
	if got.Score != 0.5 || got.Reason != "ok" {
		t.Fatalf("unexpected result: %+v", got)
	}

	got, err = parseJudgePayload(`{"items":[{"passed":true},{"match":true}]}`, 2)
	if err != nil {
		t.Fatalf("parse object items: %v", err)
	}
	if got.Score != 1 {
		t.Fatalf("unexpected object score: %+v", got)
	}

	if _, err := parseJudgePayload(`{"items":[true]}`, 2); err == nil {
		t.Fatal("item count mismatch should fail")
	}
	if _, err := parseJudgePayload(`not json`, 2); err == nil {
		t.Fatal("invalid JSON should fail")
	}

	got, err = parseJudgePayload(`{"score":1.5,"reason":"clamped"}`, 2)
	if err != nil {
		t.Fatalf("parse score fallback: %v", err)
	}
	if got.Score != 1 {
		t.Fatalf("score should be clamped to 1, got %v", got.Score)
	}
}

func TestScoreCasePlanStepLimit(t *testing.T) {
	c := Case{ID: "p", MaxSteps: 10, MaxPlanSteps: 24}
	res := &agentrun.StartResult{PlanMode: "plan", Steps: 16, Output: "ok"}
	if got := scoreCase(c, res, nil); !got.Passed {
		t.Fatalf("plan steps within max_plan_steps should pass: %+v", got)
	}

	res.Steps = 25
	if got := scoreCase(c, res, nil); got.Passed {
		t.Fatalf("plan steps over max_plan_steps should fail: %+v", got)
	}
}

func TestScoreCaseRejectsEmptyOutput(t *testing.T) {
	got := scoreCase(Case{ID: "empty"}, &agentrun.StartResult{Output: "  "}, nil)
	if got.Passed {
		t.Fatalf("empty output should fail: %+v", got)
	}
	if len(got.Missing) == 0 || got.Missing[0] != "empty_output" {
		t.Fatalf("empty output should be reported: %+v", got)
	}
}
