package tools

import (
	"ai-meeting/pkg/ecode"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================
// 文件工具沙箱
//
// 所有文件操作限制在 sandbox_root 内:
//   1. 拒绝绝对路径与 ".." 逃逸
//   2. 解析符号链接后再次校验, 防止 symlink 指向沙箱外
//   3. 只允许文本扩展名白名单
// ============================================================

// textExtensions 允许读写的文本扩展名白名单
var textExtensions = map[string]bool{
	".txt": true, ".md": true, ".json": true, ".yaml": true, ".yml": true,
	".csv": true, ".log": true, ".go": true, ".py": true, ".js": true,
	".ts": true, ".java": true, ".c": true, ".h": true, ".cpp": true,
	".hpp": true, ".sql": true, ".xml": true, ".html": true, ".css": true,
}

// ResolveSandboxPath 把用户提供的相对路径解析为沙箱内的绝对路径。
// 任何越界、绝对路径、符号链接逃逸都会被拒绝。
func ResolveSandboxPath(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", ecode.New(ecode.ErrAgentInvalidInput, "路径不能为空")
	}
	if filepath.IsAbs(rel) {
		return "", ecode.New(ecode.ErrAgentSandboxViolation, "不允许绝对路径")
	}

	cleaned := filepath.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", ecode.New(ecode.ErrAgentSandboxViolation, "路径越界")
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", ecode.Wrap(err, "解析沙箱根目录失败")
	}
	if err := os.MkdirAll(rootAbs, 0o755); err != nil {
		return "", ecode.Wrap(err, "创建沙箱根目录失败")
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", ecode.Wrap(err, "解析沙箱根目录符号链接失败")
	}

	candidate := filepath.Join(rootResolved, cleaned)
	resolved, err := resolveExistingOrParent(candidate)
	if err != nil {
		return "", ecode.Wrap(err, "解析目标路径失败")
	}

	relCheck, err := filepath.Rel(rootResolved, resolved)
	if err != nil {
		return "", ecode.Wrap(err, "校验目标路径失败")
	}
	if relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(os.PathSeparator)) || filepath.IsAbs(relCheck) {
		return "", ecode.New(ecode.ErrAgentSandboxViolation, "路径越界(符号链接逃逸)")
	}
	return resolved, nil
}

// resolveExistingOrParent 对不存在的文件, 解析其最近已存在父目录的符号链接
func resolveExistingOrParent(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	resolvedParent, err := resolveExistingOrParent(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

// EnsureTextExtension 校验文本扩展名白名单
func EnsureTextExtension(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if !textExtensions[ext] {
		return ecode.New(ecode.ErrAgentInvalidInput,
			fmt.Sprintf("不支持的文件类型: %s", ext))
	}
	return nil
}
