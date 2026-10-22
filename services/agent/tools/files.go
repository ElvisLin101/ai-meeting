package tools

import (
	"ai-meeting/pkg/ecode"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxReadBytes  = 1 << 20 // 1MB
	maxWriteBytes = 2 << 20 // 2MB
	backupDirName = ".backups"
)

// ============================================================
// 沙箱文件工具: file_list / file_read / file_write / file_edit
// ============================================================

// FileListTool 列出沙箱目录
type FileListTool struct{ sandboxRoot string }

// NewFileListTool 创建目录列举工具
func NewFileListTool(root string) *FileListTool { return &FileListTool{sandboxRoot: root} }

func (t *FileListTool) Name() string { return "file_list" }
func (t *FileListTool) Description() string {
	return "列出沙箱目录下的文件。path 为相对沙箱根目录的路径，默认为 .。返回名称、类型和字节数。"
}
func (t *FileListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"path": {"type": "string", "description": "相对目录，默认 ."}}
	}`)
}

// Meta 声明只读且可并发：目录列举不修改状态。
func (t *FileListTool) Meta() ToolMeta {
	return ToolMeta{ReadOnly: true, ConcurrencySafe: true}
}

func (t *FileListTool) Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &input)
	if input.Path == "" {
		input.Path = "."
	}
	dir, err := ResolveSandboxPath(t.sandboxRoot, input.Path)
	if err != nil {
		return Result{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Result{}, ecode.Wrap(err, "读取目录失败")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "目录: %s\n", input.Path)
	for _, e := range entries {
		if e.IsDir() {
			fmt.Fprintf(&b, "[DIR]  %s/\n", e.Name())
			continue
		}
		info, err := e.Info()
		size := int64(0)
		if err == nil {
			size = info.Size()
		}
		fmt.Fprintf(&b, "[FILE] %s (%d bytes)\n", e.Name(), size)
	}
	return Result{Content: b.String(), Meta: map[string]interface{}{"path": input.Path}}, nil
}

// FileReadTool 读取沙箱内文本文件
type FileReadTool struct{ sandboxRoot string }

// NewFileReadTool 创建读取工具
func NewFileReadTool(root string) *FileReadTool { return &FileReadTool{sandboxRoot: root} }

func (t *FileReadTool) Name() string { return "file_read" }
func (t *FileReadTool) Description() string {
	return "读取沙箱内的文本文件，最大 1MB。path 为相对沙箱根目录的路径。"
}
func (t *FileReadTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"path": {"type": "string"}},
		"required": ["path"]
	}`)
}

// Meta 声明只读且可并发：读取不同文件互不影响。
func (t *FileReadTool) Meta() ToolMeta {
	return ToolMeta{ReadOnly: true, ConcurrencySafe: true}
}

func (t *FileReadTool) Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{}, ecode.Wrap(err, "解析参数失败")
	}
	full, err := ResolveSandboxPath(t.sandboxRoot, input.Path)
	if err != nil {
		return Result{}, err
	}
	if err := EnsureTextExtension(full); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "文件不存在")
	}
	if info.Size() > maxReadBytes {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "文件超过 1MB 读取上限")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return Result{}, ecode.Wrap(err, "读取文件失败")
	}
	return Result{Content: string(data), Meta: map[string]interface{}{"path": input.Path, "bytes": info.Size()}}, nil
}

// FileWriteTool 写入沙箱内文本文件（原子写 + 写前备份）
type FileWriteTool struct{ sandboxRoot string }

// NewFileWriteTool 创建写入工具
func NewFileWriteTool(root string) *FileWriteTool { return &FileWriteTool{sandboxRoot: root} }

func (t *FileWriteTool) Name() string { return "file_write" }
func (t *FileWriteTool) Description() string {
	return "在沙箱内写入文本文件（覆盖）。自动备份原文件并原子写入，最大 2MB。path 为相对沙箱根目录的路径。"
}
func (t *FileWriteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"},
			"content": {"type": "string"}
		},
		"required": ["path", "content"]
	}`)
}
func (t *FileWriteTool) Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{}, ecode.Wrap(err, "解析参数失败")
	}
	if len(input.Content) > maxWriteBytes {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "写入内容超过 2MB 上限")
	}
	full, err := ResolveSandboxPath(t.sandboxRoot, input.Path)
	if err != nil {
		return Result{}, err
	}
	if err := EnsureTextExtension(full); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return Result{}, ecode.Wrap(err, "创建目录失败")
	}

	before := int64(0)
	backup := ""
	if info, statErr := os.Stat(full); statErr == nil {
		before = info.Size()
		backup, _ = backupFile(t.sandboxRoot, full)
	}

	if err := atomicWrite(full, []byte(input.Content)); err != nil {
		return Result{}, ecode.Wrap(err, "写入文件失败")
	}
	return Result{
		Content: fmt.Sprintf("已写入 %s（原 %d 字节 -> 新 %d 字节）", input.Path, before, len(input.Content)),
		Meta: map[string]interface{}{
			"path": input.Path, "before_bytes": before,
			"after_bytes": len(input.Content), "backup": backup,
		},
	}, nil
}

// FileEditTool 在沙箱文件内做字符串替换
type FileEditTool struct{ sandboxRoot string }

// NewFileEditTool 创建编辑工具
func NewFileEditTool(root string) *FileEditTool { return &FileEditTool{sandboxRoot: root} }

func (t *FileEditTool) Name() string { return "file_edit" }
func (t *FileEditTool) Description() string {
	return "在沙箱内文本文件中把 old_string 替换为 new_string。replace_all=false 时只替换第一处。自动备份并原子写入。"
}
func (t *FileEditTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"},
			"old_string": {"type": "string"},
			"new_string": {"type": "string"},
			"replace_all": {"type": "boolean"}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}
func (t *FileEditTool) Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{}, ecode.Wrap(err, "解析参数失败")
	}
	if input.OldString == "" {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "old_string 不能为空")
	}
	full, err := ResolveSandboxPath(t.sandboxRoot, input.Path)
	if err != nil {
		return Result{}, err
	}
	if err := EnsureTextExtension(full); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "文件不存在")
	}
	if info.Size() > maxReadBytes {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "文件超过 1MB 编辑上限")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return Result{}, ecode.Wrap(err, "读取文件失败")
	}

	content := string(data)
	count := strings.Count(content, input.OldString)
	if count == 0 {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "未找到 old_string")
	}
	n := count
	if !input.ReplaceAll {
		n = 1
	}
	updated := strings.Replace(content, input.OldString, input.NewString, n)
	if len(updated) > maxWriteBytes {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "编辑后内容超过 2MB 上限")
	}

	backup, _ := backupFile(t.sandboxRoot, full)
	if err := atomicWrite(full, []byte(updated)); err != nil {
		return Result{}, ecode.Wrap(err, "写入文件失败")
	}
	return Result{
		Content: fmt.Sprintf("已替换 %d 处（匹配 %d 处），文件 %s 已更新", n, count, input.Path),
		Meta: map[string]interface{}{
			"path": input.Path, "replaced": n, "matched": count, "backup": backup,
		},
	}, nil
}

// atomicWrite 同目录临时文件 + rename，保证写入原子性
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// backupFile 写前备份到沙箱内 .backups 目录
func backupFile(root, path string) (string, error) {
	backupDir := filepath.Join(root, backupDirName)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s_%s", time.Now().Format("20060102-150405.000000000"), filepath.Base(path))
	dest := filepath.Join(backupDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}
