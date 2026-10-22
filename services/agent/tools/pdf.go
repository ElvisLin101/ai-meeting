package tools

import (
	"ai-meeting/clients"
	"ai-meeting/pkg/ecode"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// PDFTool 解析沙箱内的 PDF 简历文本
type PDFTool struct {
	sandboxRoot string
}

// NewPDFTool 创建 PDF 工具
func NewPDFTool(sandboxRoot string) *PDFTool {
	return &PDFTool{sandboxRoot: sandboxRoot}
}

// Name 工具名
func (t *PDFTool) Name() string { return "pdf_parse" }

// Description 工具描述（会进入模型 prompt，需写清用途与限制）
func (t *PDFTool) Description() string {
	return "解析沙箱目录内的 PDF 文件并返回纯文本。仅支持 PDF，最大 10MB。path 为相对沙箱根目录的路径，例如 resume.pdf。"
}

// Parameters JSON Schema
func (t *PDFTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "相对沙箱根目录的 PDF 路径"}
		},
		"required": ["path"]
	}`)
}

// Meta 声明只读且可并发：不同 PDF 解析之间没有共享状态。
func (t *PDFTool) Meta() ToolMeta {
	return ToolMeta{ReadOnly: true, ConcurrencySafe: true}
}

// Execute 执行解析
func (t *PDFTool) Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{}, ecode.Wrap(err, "解析参数失败")
	}

	fullPath, err := ResolveSandboxPath(t.sandboxRoot, input.Path)
	if err != nil {
		return Result{}, err
	}
	if !strings.EqualFold(filepath.Ext(fullPath), ".pdf") {
		return Result{}, ecode.New(ecode.ErrResumeNotPDF, "仅支持 PDF 文件")
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "文件不存在")
	}
	if info.Size() > 10<<20 {
		return Result{}, ecode.New(ecode.ErrAgentInvalidInput, "PDF 超过 10MB 限制")
	}

	// magic bytes 校验, 防止伪装后缀
	f, err := os.Open(fullPath)
	if err != nil {
		return Result{}, ecode.Wrap(err, "打开 PDF 失败")
	}
	head := make([]byte, 5)
	n, _ := f.Read(head)
	f.Close()
	if n < 5 || string(head[:5]) != "%PDF-" {
		return Result{}, ecode.New(ecode.ErrResumeNotPDF, "文件不是有效 PDF")
	}

	text, err := clients.ParsePDFFromPath(fullPath)
	if err != nil {
		return Result{}, ecode.Wrap(err, "解析 PDF 失败")
	}
	return Result{
		Content: text,
		Meta:    map[string]interface{}{"path": input.Path, "bytes": info.Size()},
	}, nil
}
