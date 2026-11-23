package callback

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// ============================================================
// Agent 生命周期回调
//
// 设计目标:
//  1. 与 SSE 的 loop.Event 解耦：Event 面向终端用户，Callback 面向日志/指标/追踪。
//  2. 与业务代码解耦：Handler 只读事件，不参与循环控制，不修改运行结果。
//  3. 可组合：Multi 分发到多个 Handler，后续新增 metric/tracing 不需要改 loop。
//
// 约定:
//  - 事件只携带标识、耗时、数量、错误等元数据，不携带完整 prompt/工具结果，
//    避免日志泄漏用户内容；需要内容时由 Handler 自行决定。
//  - 所有方法都必须是并发安全的：plan 模式下多个子任务可能同时回调。
// ============================================================

// RunStartEvent 一次 Agent 循环开始。
type RunStartEvent struct {
	RunID      string
	SessionID  string
	AgentID    uint
	InputRunes int
}

// RunEndEvent 一次 Agent 循环结束。
type RunEndEvent struct {
	RunID      string
	SessionID  string
	Steps      int
	Tokens     int
	StopReason string
	Duration   time.Duration
	Err        error
}

// LLMStartEvent 单步模型调用开始。
type LLMStartEvent struct {
	RunID        string
	SessionID    string
	Step         int
	AgentID      uint
	MessageCount int
	ToolCount    int
}

// LLMEndEvent 单步模型调用结束。
type LLMEndEvent struct {
	RunID        string
	SessionID    string
	Step         int
	Duration     time.Duration
	Tokens       int
	ToolCalls    []string
	ContentRunes int
	Err          error
}

// ToolStartEvent 单次工具调用开始。
type ToolStartEvent struct {
	RunID     string
	SessionID string
	Step      int
	CallID    string
	ToolName  string
	ArgsBytes int
}

// ToolEndEvent 单次工具调用结束。
type ToolEndEvent struct {
	RunID       string
	SessionID   string
	Step        int
	CallID      string
	ToolName    string
	Duration    time.Duration
	ResultBytes int
	Success     bool
	Err         error
}

// Handler 是 Agent 生命周期回调接口。
// 实现方可以只关心自己需要的方法，其余方法通过嵌入 Base 获得空实现。
type Handler interface {
	OnRunStart(ctx context.Context, ev RunStartEvent)
	OnRunEnd(ctx context.Context, ev RunEndEvent)
	OnLLMStart(ctx context.Context, ev LLMStartEvent)
	OnLLMEnd(ctx context.Context, ev LLMEndEvent)
	OnToolStart(ctx context.Context, ev ToolStartEvent)
	OnToolEnd(ctx context.Context, ev ToolEndEvent)
}

// Base 提供 Handler 的空实现，供业务 Handler 嵌入。
type Base struct{}

func (Base) OnRunStart(context.Context, RunStartEvent)   {}
func (Base) OnRunEnd(context.Context, RunEndEvent)       {}
func (Base) OnLLMStart(context.Context, LLMStartEvent)   {}
func (Base) OnLLMEnd(context.Context, LLMEndEvent)       {}
func (Base) OnToolStart(context.Context, ToolStartEvent) {}
func (Base) OnToolEnd(context.Context, ToolEndEvent)     {}

// Multi 将同一个事件分发给多个 Handler，nil 会被忽略。
// 调用顺序与注册顺序一致；某个 Handler 不应假设其他 Handler 已经执行。
type Multi []Handler

func (m Multi) OnRunStart(ctx context.Context, ev RunStartEvent) {
	for _, h := range m {
		if h != nil {
			h.OnRunStart(ctx, ev)
		}
	}
}

func (m Multi) OnRunEnd(ctx context.Context, ev RunEndEvent) {
	for _, h := range m {
		if h != nil {
			h.OnRunEnd(ctx, ev)
		}
	}
}

func (m Multi) OnLLMStart(ctx context.Context, ev LLMStartEvent) {
	for _, h := range m {
		if h != nil {
			h.OnLLMStart(ctx, ev)
		}
	}
}

func (m Multi) OnLLMEnd(ctx context.Context, ev LLMEndEvent) {
	for _, h := range m {
		if h != nil {
			h.OnLLMEnd(ctx, ev)
		}
	}
}

func (m Multi) OnToolStart(ctx context.Context, ev ToolStartEvent) {
	for _, h := range m {
		if h != nil {
			h.OnToolStart(ctx, ev)
		}
	}
}

func (m Multi) OnToolEnd(ctx context.Context, ev ToolEndEvent) {
	for _, h := range m {
		if h != nil {
			h.OnToolEnd(ctx, ev)
		}
	}
}

// OrNop 把 nil Handler 归一成空实现，避免调用方到处判空。
func OrNop(h Handler) Handler {
	if h == nil {
		return Base{}
	}
	return h
}

// Logging 是基于 logrus 的默认日志 Handler。
// 只记录元数据，不记录 prompt/工具结果正文。
type Logging struct {
	logger *logrus.Logger
}

// NewLogging 创建默认日志 Handler；logger 为 nil 时使用标准 logger。
func NewLogging(logger *logrus.Logger) *Logging {
	if logger == nil {
		logger = logrus.StandardLogger()
	}
	return &Logging{logger: logger}
}

func (l *Logging) OnRunStart(_ context.Context, ev RunStartEvent) {
	l.logger.WithFields(logrus.Fields{
		"run_id":      ev.RunID,
		"session_id":  ev.SessionID,
		"agent_id":    ev.AgentID,
		"input_runes": ev.InputRunes,
	}).Info("[agent-callback] run start")
}

func (l *Logging) OnRunEnd(_ context.Context, ev RunEndEvent) {
	fields := logrus.Fields{
		"run_id":      ev.RunID,
		"session_id":  ev.SessionID,
		"steps":       ev.Steps,
		"tokens":      ev.Tokens,
		"stop_reason": ev.StopReason,
		"duration_ms": ev.Duration.Milliseconds(),
	}
	if ev.Err != nil {
		fields["error"] = ev.Err.Error()
		l.logger.WithFields(fields).Warn("[agent-callback] run end with error")
		return
	}
	l.logger.WithFields(fields).Info("[agent-callback] run end")
}

func (l *Logging) OnLLMStart(_ context.Context, ev LLMStartEvent) {
	l.logger.WithFields(logrus.Fields{
		"run_id":        ev.RunID,
		"session_id":    ev.SessionID,
		"step":          ev.Step,
		"agent_id":      ev.AgentID,
		"message_count": ev.MessageCount,
		"tool_count":    ev.ToolCount,
	}).Debug("[agent-callback] llm start")
}

func (l *Logging) OnLLMEnd(_ context.Context, ev LLMEndEvent) {
	fields := logrus.Fields{
		"run_id":        ev.RunID,
		"session_id":    ev.SessionID,
		"step":          ev.Step,
		"duration_ms":   ev.Duration.Milliseconds(),
		"tokens":        ev.Tokens,
		"tool_calls":    ev.ToolCalls,
		"content_runes": ev.ContentRunes,
	}
	if ev.Err != nil {
		fields["error"] = ev.Err.Error()
		l.logger.WithFields(fields).Warn("[agent-callback] llm end with error")
		return
	}
	l.logger.WithFields(fields).Info("[agent-callback] llm end")
}

func (l *Logging) OnToolStart(_ context.Context, ev ToolStartEvent) {
	l.logger.WithFields(logrus.Fields{
		"run_id":     ev.RunID,
		"session_id": ev.SessionID,
		"step":       ev.Step,
		"call_id":    ev.CallID,
		"tool":       ev.ToolName,
		"args_bytes": ev.ArgsBytes,
	}).Debug("[agent-callback] tool start")
}

func (l *Logging) OnToolEnd(_ context.Context, ev ToolEndEvent) {
	fields := logrus.Fields{
		"run_id":       ev.RunID,
		"session_id":   ev.SessionID,
		"step":         ev.Step,
		"call_id":      ev.CallID,
		"tool":         ev.ToolName,
		"duration_ms":  ev.Duration.Milliseconds(),
		"result_bytes": ev.ResultBytes,
		"success":      ev.Success,
	}
	if ev.Err != nil {
		fields["error"] = ev.Err.Error()
		l.logger.WithFields(fields).Warn("[agent-callback] tool end with error")
		return
	}
	l.logger.WithFields(fields).Info("[agent-callback] tool end")
}
