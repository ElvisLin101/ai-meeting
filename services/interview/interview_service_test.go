package interview

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================
// 文件上传/复制工具测试(copyFile / ctx_SaveUploadedFile)
// ============================================================

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()

	src, err := os.Create(filepath.Join(dir, "src.txt"))
	if err != nil {
		t.Fatalf("create src failed: %v", err)
	}
	defer src.Close()
	content := "hello copy 世界"
	if _, err := src.WriteString(content); err != nil {
		t.Fatalf("write src failed: %v", err)
	}
	if _, err := src.Seek(0, 0); err != nil {
		t.Fatalf("seek failed: %v", err)
	}

	dst, err := os.Create(filepath.Join(dir, "dst.txt"))
	if err != nil {
		t.Fatalf("create dst failed: %v", err)
	}
	defer dst.Close()

	n, err := copyFile(src, dst)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}
	if int(n) != len(content) {
		t.Errorf("copied = %d, want %d", n, len(content))
	}

	// 目标文件内容应与源一致
	dstContent, _ := os.ReadFile(filepath.Join(dir, "dst.txt"))
	if string(dstContent) != content {
		t.Errorf("dst = %q, want %q", dstContent, content)
	}
}

// errFile 模拟读取失败的 multipart.File
type errFile struct{}

func (errFile) Read([]byte) (int, error)                { return 0, errors.New("read failed") }
func (errFile) ReadAt(p []byte, off int64) (int, error) { return 0, errors.New("read failed") }
func (errFile) Seek(int64, int) (int64, error)          { return 0, nil }
func (errFile) Close() error                            { return nil }

func TestCopyFile_ReadError(t *testing.T) {
	dst, err := os.Create(filepath.Join(t.TempDir(), "dst.txt"))
	if err != nil {
		t.Fatalf("create dst failed: %v", err)
	}
	defer dst.Close()

	if _, err := copyFile(errFile{}, dst); err == nil {
		t.Error("源读取失败应返回错误")
	}
}

func TestCtxSaveUploadedFile(t *testing.T) {
	dir := t.TempDir()
	content := "上传的简历内容"

	// 构造 multipart 表单请求
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "resume.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write form file failed: %v", err)
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("ParseMultipartForm failed: %v", err)
	}
	fh := req.MultipartForm.File["file"][0]

	dst := filepath.Join(dir, "saved.pdf")
	if err := ctx_SaveUploadedFile(fh, dst); err != nil {
		t.Fatalf("ctx_SaveUploadedFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst failed: %v", err)
	}
	if string(got) != content {
		t.Errorf("saved = %q, want %q", got, content)
	}
}

func TestCtxSaveUploadedFile_InvalidPath(t *testing.T) {
	// 目标目录不存在 → 应返回错误
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "a.txt")
	fw.Write([]byte("x"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.ParseMultipartForm(1 << 20)
	fh := req.MultipartForm.File["file"][0]

	badDst := filepath.Join(t.TempDir(), "no-such-dir", "a.txt")
	if err := ctx_SaveUploadedFile(fh, badDst); err == nil {
		t.Error("目标目录不存在应返回错误")
	}
}
