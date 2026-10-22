package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveSandboxPath 表驱动验证沙箱路径校验
func TestResolveSandboxPath(t *testing.T) {
	root := t.TempDir()

	valid := []string{"a.txt", "dir/b.txt", "./c.txt"}
	for _, rel := range valid {
		if _, err := ResolveSandboxPath(root, rel); err != nil {
			t.Errorf("valid path %q rejected: %v", rel, err)
		}
	}

	invalid := []string{"", "/etc/passwd", "../escape.txt", "a/../../b.txt", ".."}
	for _, rel := range invalid {
		if _, err := ResolveSandboxPath(root, rel); err == nil {
			t.Errorf("invalid path %q should be rejected", rel)
		}
	}
}

// TestResolveSandboxPath_SymlinkEscape 符号链接逃逸必须被拒绝
func TestResolveSandboxPath_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if _, err := ResolveSandboxPath(root, "link/secret.txt"); err == nil {
		t.Fatal("symlink escape should be rejected")
	}
}

// TestEnsureTextExtension 扩展名白名单
func TestEnsureTextExtension(t *testing.T) {
	for _, p := range []string{"a.txt", "a.md", "a.json", "a.go"} {
		if err := EnsureTextExtension(p); err != nil {
			t.Errorf("%s should be allowed: %v", p, err)
		}
	}
	for _, p := range []string{"a.pdf.exe", "a.bin", "a.sh"} {
		if err := EnsureTextExtension(p); err == nil {
			t.Errorf("%s should be rejected", p)
		}
	}
}

// TestValidateArgs required 与类型校验
func TestValidateArgs(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{"path":{"type":"string"},"count":{"type":"number"}},
		"required":["path"]
	}`)

	if err := ValidateArgs(schema, json.RawMessage(`{"path":"a.txt"}`)); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
	if err := ValidateArgs(schema, json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing required should fail")
	}
	if err := ValidateArgs(schema, json.RawMessage(`{"path":123}`)); err == nil {
		t.Fatal("wrong type should fail")
	}
	if err := ValidateArgs(schema, json.RawMessage(`[]`)); err == nil {
		t.Fatal("non-object should fail")
	}
}
