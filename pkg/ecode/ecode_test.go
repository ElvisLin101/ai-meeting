package ecode

import (
	"errors"
	"fmt"
	"testing"
)

func TestNew_CodeAndMessage(t *testing.T) {
	e := New(-2002, "面试已结束")
	if e.Code() != -2002 {
		t.Errorf("Code() = %d, want -2002", e.Code())
	}
	if e.Message() != "面试已结束" {
		t.Errorf("Message() = %q, want 面试已结束", e.Message())
	}
	if e.Error() != "面试已结束" {
		t.Errorf("Error() = %q, want 面试已结束", e.Error())
	}
}

func TestWithMessage_PreservesCode(t *testing.T) {
	e := New(ErrQuestionExpired, "题号已过期")
	wrapped := e.WithMessage("题号已过期，当前题号为 3")
	if wrapped.Code() != ErrQuestionExpired {
		t.Errorf("WithMessage changed code = %d, want %d", wrapped.Code(), ErrQuestionExpired)
	}
	if wrapped.Message() != "题号已过期，当前题号为 3" {
		t.Errorf("WithMessage message = %q", wrapped.Message())
	}
}

func TestCause(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{"nil", nil, Success, ""},
		{"direct ecode", New(ErrInterviewCompleted, "面试已结束"), ErrInterviewCompleted, "面试已结束"},
		{"wrapped with ecode.Wrap", Wrap(New(ErrInterviewCompleted, "面试已结束"), "推进失败"), ErrInterviewCompleted, "面试已结束"},
		{"wrapped with fmt %w", fmt.Errorf("ensureRuntime 失败: %w", New(ErrQuestionLocked, "锁定")), ErrQuestionLocked, "锁定"},
		{"plain error falls back to server err", errors.New("oops"), ServerErr, "oops"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Cause(tt.err)
			if got.Code() != tt.wantCode {
				t.Errorf("Code() = %d, want %d", got.Code(), tt.wantCode)
			}
			if got.Message() != tt.wantMsg {
				t.Errorf("Message() = %q, want %q", got.Message(), tt.wantMsg)
			}
		})
	}
}

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		code int
		want int
	}{
		{"not login", NotLogin, 401},
		{"no permission", NoPermission, 403},
		{"not exist", NotExist, 404},
		{"request err", RequestErr, 400},
		{"business default 500", ErrInterviewCompleted, 500},
		{"wrong password registered 401", ErrWrongPassword, 401},
		{"username exists registered 400", ErrUsernameExists, 400},
		{"session not found registered 404", ErrSessionNotFound, 404},
		{"ai conversation not found registered 404", ErrAiConversationNotFound, 404},
		{"no resume registered 404", ErrNoResume, 404},
		{"empty message registered 400", ErrEmptyAiMessageContent, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.code, "msg").HTTPStatus(); got != tt.want {
				t.Errorf("HTTPStatus(%d) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestIs(t *testing.T) {
	if !Is(New(ErrInterviewCompleted, "面试已结束"), ErrInterviewCompleted) {
		t.Error("Is should match same code")
	}
	if Is(New(ErrInterviewCompleted, "面试已结束"), ErrQuestionExpired) {
		t.Error("Is should not match different code")
	}
	// 包装链穿透
	if !Is(Wrap(New(ErrQuestionLocked, "锁定"), "上下文"), ErrQuestionLocked) {
		t.Error("Is should penetrate wrap chain")
	}
	// 非 ecode 错误一律按 ServerErr 处理
	if !Is(errors.New("oops"), ServerErr) {
		t.Error("plain error should be classified as ServerErr")
	}
}

func TestWrap_NilSafe(t *testing.T) {
	if err := Wrap(nil, "上下文"); err != nil {
		t.Errorf("Wrap(nil) = %v, want nil", err)
	}
}
