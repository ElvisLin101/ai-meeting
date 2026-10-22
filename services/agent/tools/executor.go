package tools

import (
	"ai-meeting/clients"
	"ai-meeting/pkg/ecode"
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/sirupsen/logrus"
)

// ============================================================
// 工具执行器: 统一做参数校验、超时、panic 恢复、结果截断与审计
// ============================================================

// MetricFunc 工具调用指标回调
type MetricFunc func(tool string, success bool, durationMs int64, errType string)

// Executor 工具执行器
type Executor struct {
	registry       *Registry
	toolTimeout    time.Duration
	maxResultBytes int
	onMetric       MetricFunc
}

// NewExecutor 创建执行器
func NewExecutor(registry *Registry, toolTimeout time.Duration, maxResultBytes int) *Executor {
	if toolTimeout <= 0 {
		toolTimeout = 15 * time.Second
	}
	if maxResultBytes <= 0 {
		maxResultBytes = 32 * 1024
	}
	return &Executor{
		registry:       registry,
		toolTimeout:    toolTimeout,
		maxResultBytes: maxResultBytes,
	}
}

// SetMetricFunc 注入指标回调
func (e *Executor) SetMetricFunc(fn MetricFunc) { e.onMetric = fn }

// Execute 执行一次工具调用
func (e *Executor) Execute(ctx context.Context, tc *Context, call clients.ToolCall) (Result, error) {
	tool, ok := e.registry.Get(call.Function.Name)
	if !ok {
		return Result{}, ecode.New(ecode.ErrAgentToolNotFound,
			fmt.Sprintf("工具不存在: %s", call.Function.Name))
	}

	args := json.RawMessage(call.Function.Arguments)
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	if err := ValidateArgs(tool.Parameters(), args); err != nil {
		return Result{}, ecode.Wrap(err, fmt.Sprintf("工具参数非法: %s", tool.Name()))
	}

	execCtx, cancel := context.WithTimeout(ctx, e.toolTimeout)
	defer cancel()

	start := time.Now()
	result, err := e.executeSafe(execCtx, tool, tc, args)
	durationMs := time.Since(start).Milliseconds()

	if e.onMetric != nil {
		errType := ""
		if err != nil {
			errType = "tool_error"
		}
		e.onMetric(tool.Name(), err == nil, durationMs, errType)
	}

	if err != nil {
		logrus.Warnf("[agent-tool] tool=%s failed duration=%dms err=%v", tool.Name(), durationMs, err)
		if execCtx.Err() == context.DeadlineExceeded {
			return Result{}, ecode.New(ecode.ErrAgentToolTimeout,
				fmt.Sprintf("工具执行超时: %s", tool.Name()))
		}
		return Result{}, ecode.Wrap(err, fmt.Sprintf("工具执行失败: %s", tool.Name()))
	}

	logrus.Infof("[agent-tool] tool=%s ok duration=%dms resultBytes=%d", tool.Name(), durationMs, len(result.Content))

	if len(result.Content) > e.maxResultBytes {
		result.Content = truncateRunes(result.Content, e.maxResultBytes)
		result.Truncated = true
	}
	return result, nil
}

// executeSafe 捕获工具 panic, 避免单个工具拖垮整个 Agent run
func (e *Executor) executeSafe(ctx context.Context, tool Tool, tc *Context, args json.RawMessage) (result Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[agent-tool] tool=%s panic: %v\n%s", tool.Name(), r, debug.Stack())
			err = fmt.Errorf("tool panic: %v", r)
		}
	}()
	return tool.Execute(ctx, tc, args)
}

// truncateRunes 按 rune 截断, 避免切坏中文 UTF-8
func truncateRunes(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	runes := []rune(text)
	// 粗略按 rune 数截断 (maxBytes 当作字节预算, 中文最多 3 字节)
	budget := maxBytes / 3
	if budget < 1 {
		budget = 1
	}
	if len(runes) <= budget {
		return text
	}
	return string(runes[:budget]) + "\n...[结果已截断]"
}

// ============================================================
// 轻量 JSON Schema 校验（只校验顶层 required 与类型, 不引入额外依赖）
// ============================================================

type schemaSpec struct {
	Type       string                    `json:"type"`
	Properties map[string]schemaProperty `json:"properties"`
	Required   []string                  `json:"required"`
}

type schemaProperty struct {
	Type string `json:"type"`
}

// ValidateArgs 校验工具入参（JSON object + required + 顶层类型）
func ValidateArgs(schema json.RawMessage, args json.RawMessage) error {
	var spec schemaSpec
	if len(schema) > 0 {
		if err := json.Unmarshal(schema, &spec); err != nil {
			return fmt.Errorf("invalid tool schema: %w", err)
		}
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(args, &obj); err != nil {
		return fmt.Errorf("arguments must be a JSON object: %w", err)
	}

	for _, key := range spec.Required {
		val, ok := obj[key]
		if !ok || val == nil {
			return fmt.Errorf("missing required argument: %s", key)
		}
	}

	for key, prop := range spec.Properties {
		val, ok := obj[key]
		if !ok || val == nil {
			continue
		}
		if !matchesType(val, prop.Type) {
			return fmt.Errorf("argument %s must be %s", key, prop.Type)
		}
	}
	return nil
}

func matchesType(val interface{}, want string) bool {
	switch want {
	case "", "any":
		return true
	case "string":
		_, ok := val.(string)
		return ok
	case "number", "integer":
		_, ok := val.(float64)
		return ok
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "array":
		_, ok := val.([]interface{})
		return ok
	case "object":
		_, ok := val.(map[string]interface{})
		return ok
	default:
		return true
	}
}
