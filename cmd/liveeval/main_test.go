package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
