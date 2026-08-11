package ecode

import "net/http"

// ============================================================
// 业务错误码
// 码段: 用户 -100x / 面试 -200x / AI -300x / Agent -400x
// 动态消息场景用 ecode.New(code, fmt.Sprintf(...)) 构造
// ============================================================

const (
	// 用户
	ErrWrongPassword  = -1001 // 用户名或密码错误
	ErrUsernameExists = -1002 // 用户名已存在
	ErrUserNotFound   = -1003 // 用户不存在

	// 面试
	ErrInterviewNotInitialized = -2001 // 面试流程未初始化
	ErrInterviewCompleted      = -2002 // 面试已结束
	ErrQuestionExpired         = -2003 // 题号已过期
	ErrIdempotencyProcessing   = -2004 // 请求正在处理中
	ErrQuestionLocked          = -2005 // 当前题目正在被处理
	ErrNoResume                = -2006 // 未上传简历
	ErrResumeNotPDF            = -2007 // 仅支持 PDF 格式简历
	ErrQuestionsEmpty          = -2008 // AI 出题返回空题目列表
	ErrNoTurnRecord            = -2009 // 无轮次记录
	ErrEvaluationParse         = -2010 // 评分解析失败
	ErrFollowUpParse           = -2011 // 追问解析失败
	ErrSessionNotFound         = -2012 // 会话不存在或无权限

	// AI
	ErrAiConversationNotFound = -3001 // 会话不存在或无权限
	ErrEmptyAiMessageContent  = -3002 // 消息内容不能为空

	// Agent
	ErrAgentPropertyNotFound = -4001 // 智能体配置不存在
	ErrDeepSeekCall          = -4002 // 调用 DeepSeek 失败
)

// init 注册业务码的 HTTP 状态码
// 原则: "预期内的业务失败"(状态/参数/冲突/上游) 映射到语义化 4xx/5xx,
// 避免全部落到 500 让客户端只能靠 body code 区分。
func init() {
	RegisterHTTPStatus(ErrWrongPassword, http.StatusUnauthorized)
	RegisterHTTPStatus(ErrUserNotFound, http.StatusUnauthorized)
	RegisterHTTPStatus(ErrUsernameExists, http.StatusBadRequest)
	RegisterHTTPStatus(ErrEmptyAiMessageContent, http.StatusBadRequest)

	// 面试: 未初始化/已完成/题号过期 → 400; 幂等处理中/题级锁冲突 → 409; 无记录 → 404
	RegisterHTTPStatus(ErrInterviewNotInitialized, http.StatusBadRequest)
	RegisterHTTPStatus(ErrInterviewCompleted, http.StatusBadRequest)
	RegisterHTTPStatus(ErrQuestionExpired, http.StatusBadRequest)
	RegisterHTTPStatus(ErrIdempotencyProcessing, http.StatusConflict)
	RegisterHTTPStatus(ErrQuestionLocked, http.StatusConflict)
	RegisterHTTPStatus(ErrNoResume, http.StatusNotFound)
	RegisterHTTPStatus(ErrResumeNotPDF, http.StatusBadRequest)
	RegisterHTTPStatus(ErrNoTurnRecord, http.StatusNotFound)
	RegisterHTTPStatus(ErrSessionNotFound, http.StatusNotFound)

	RegisterHTTPStatus(ErrAiConversationNotFound, http.StatusNotFound)

	// Agent: 配置不存在 → 404; DeepSeek 上游失败 → 502
	RegisterHTTPStatus(ErrAgentPropertyNotFound, http.StatusNotFound)
	RegisterHTTPStatus(ErrDeepSeekCall, http.StatusBadGateway)
}
