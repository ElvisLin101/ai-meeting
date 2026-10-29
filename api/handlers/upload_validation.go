package handlers

import (
	"ai-meeting/pkg/ecode"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

type uploadKind int

const (
	uploadPDF uploadKind = iota
	uploadImage
	uploadText
	uploadZip
)

var agentUploadTypes = map[string]uploadKind{
	".pdf":  uploadPDF,
	".png":  uploadImage,
	".jpg":  uploadImage,
	".jpeg": uploadImage,
	".gif":  uploadImage,
	".txt":  uploadText,
	".md":   uploadText,
	".csv":  uploadText,
	".json": uploadText,
	".docx": uploadZip,
	".zip":  uploadZip,
}

// validateAgentUpload 同时校验扩展名和 magic bytes，防止仅改后缀的伪装文件。
func validateAgentUpload(file *multipart.FileHeader) error {
	if file == nil || file.Size <= 0 {
		return ecode.New(ecode.RequestErr, "文件为空")
	}
	kind, ok := agentUploadTypes[strings.ToLower(filepath.Ext(file.Filename))]
	if !ok {
		return ecode.New(ecode.RequestErr, "不支持的文件类型")
	}
	f, err := file.Open()
	if err != nil {
		return ecode.Wrap(err, "读取上传文件失败")
	}
	defer f.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return ecode.Wrap(err, "读取上传文件失败")
	}
	return validateUploadMagic(kind, head[:n])
}

func validateUploadMagic(kind uploadKind, head []byte) error {
	switch kind {
	case uploadPDF:
		if !bytes.HasPrefix(head, []byte("%PDF-")) {
			return ecode.New(ecode.RequestErr, "文件内容不是有效 PDF")
		}
	case uploadImage:
		mime := http.DetectContentType(head)
		if !strings.HasPrefix(mime, "image/") {
			return ecode.New(ecode.RequestErr, "文件内容不是有效图片")
		}
	case uploadText:
		mime := http.DetectContentType(head)
		if !strings.HasPrefix(mime, "text/") && mime != "application/json" {
			return ecode.New(ecode.RequestErr, "文件内容不是有效文本")
		}
	case uploadZip:
		if !bytes.HasPrefix(head, []byte("PK\x03\x04")) && !bytes.HasPrefix(head, []byte("PK\x05\x06")) {
			return ecode.New(ecode.RequestErr, "文件内容不是有效 ZIP/DOCX")
		}
	default:
		return ecode.New(ecode.RequestErr, "不支持的文件类型")
	}
	return nil
}
