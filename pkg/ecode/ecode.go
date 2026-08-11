package ecode

import (
	"errors"
	"fmt"
	"net/http"
)

// ============================================================
// 统一错误码
//
// 风格参考 bilibili ecode: 错误码为负整数, 每个错误码附带人类可读消息。
// 支持通过 fmt.Errorf("...: %w", err) 包装, Cause 用 errors.As 穿透
// 包装链找回 ecode.Error; 无法识别时兜底 ServerErr 但保留原文消息。
// ============================================================

// 标准错误码
const (
	Success      = 0    // 成功
	ServerErr    = -500 // 服务器内部错误
	RequestErr   = -400 // 参数错误
	NotLogin     = -101 // 未登录
	NoPermission = -403 // 无权限
	NotExist     = -404 // 资源不存在
)

// bizStatusMap 业务码 → HTTP 状态码 补充映射(由 ecode_biz.go 注册)
var bizStatusMap = map[int]int{}

// RegisterHTTPStatus 注册业务码的 HTTP 状态码映射(init 时调用)
func RegisterHTTPStatus(code, status int) {
	bizStatusMap[code] = status
}

// Error 带错误码的错误
type Error struct {
	code    int
	message string
}

// New 创建带错误码的错误
func New(code int, msg string) *Error {
	return &Error{code: code, message: msg}
}

// Error 实现 error 接口
func (e *Error) Error() string {
	return e.message
}

// Code 返回错误码
func (e *Error) Code() int {
	if e == nil {
		return Success
	}
	return e.code
}

// Message 返回错误消息
func (e *Error) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

// WithMessage 替换错误消息(保留错误码), 用于动态消息
func (e *Error) WithMessage(msg string) *Error {
	if e == nil {
		return New(ServerErr, msg)
	}
	return &Error{code: e.code, message: msg}
}

// HTTPStatus 错误码 → HTTP 状态码映射
// 标准码映射语义状态; 业务码走注册表, 未注册的统一 500(与改造前行为一致)
func (e *Error) HTTPStatus() int {
	switch e.Code() {
	case NotLogin:
		return http.StatusUnauthorized
	case NoPermission:
		return http.StatusForbidden
	case NotExist:
		return http.StatusNotFound
	case RequestErr:
		return http.StatusBadRequest
	}
	if status, ok := bizStatusMap[e.Code()]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// Cause 从错误链中提取 ecode.Error; 非 ecode 错误兜底为 ServerErr(保留原文)
func Cause(err error) *Error {
	if err == nil {
		return New(Success, "")
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return New(ServerErr, err.Error())
}

// Is 判断错误是否携带指定错误码
func Is(err error, code int) bool {
	return Cause(err).Code() == code
}

// Wrap 用 %w 包装 ecode 错误并附加上下文, 保留错误码可被 Cause 提取
func Wrap(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}
