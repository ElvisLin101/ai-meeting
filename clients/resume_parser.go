package clients

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ============================================================
// PDF 简历解析
// 使用 ledongthuc/pdf 提取 PDF 文本内容
// ============================================================

// ParsePDFFromPath 从 PDF 文件路径提取文本
func ParsePDFFromPath(filePath string) (string, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开 PDF 失败: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	totalPages := r.NumPage()
	for i := 1; i <= totalPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue // 单页解析失败跳过, 不中断整体
		}
		buf.WriteString(text)
		buf.WriteString("\n")
	}

	result := strings.TrimSpace(buf.String())
	if result == "" {
		return "", fmt.Errorf("PDF 内容为空或无法提取文本")
	}
	return result, nil
}

// ParsePDFFromReader 从 io.ReaderAt 提取 PDF 文本
// size 是数据总长度
func ParsePDFFromReader(reader io.ReaderAt, size int64) (string, error) {
	r, err := pdf.NewReader(reader, size)
	if err != nil {
		return "", fmt.Errorf("读取 PDF 失败: %w", err)
	}

	var buf bytes.Buffer
	totalPages := r.NumPage()
	for i := 1; i <= totalPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
	}

	result := strings.TrimSpace(buf.String())
	if result == "" {
		return "", fmt.Errorf("PDF 内容为空或无法提取文本")
	}
	return result, nil
}
