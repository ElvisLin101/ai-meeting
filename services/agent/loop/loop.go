package loop

import (
	"ai-meeting/clients"
	"ai-meeting/models"
	"ai-meeting/pkg/ecode"
	"ai-meeting/services/agent/tools"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ============================================================
// Agent 循环（ReAct: Thought -> Action -> Observation）
//
// 终止条件:
//   1. 模型给出最终回答（无 tool_calls）
//   2. 达到 MaxSteps
//   3. 连续重复同一工具+参数（防死循环）
//   4. 工具连续失败达到上限
//   5. ctx 取消/总超时
// ============================================================

// EventType 事件类型
type EventType string

const (
	EventStep       EventType = "step"
	EventRoute      EventType = "route"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventMessage    EventType = "message"
	EventDone       EventType = "done"
	EventError      EventType = "error"

	// Plan 编排事件。由 services/agent/plan 发出，复用同一个 Event 结构，
	// 避免为 plan 单独维护一套 SSE 协议。
	EventPlanReady       EventType = "plan_ready"
	EventPlanTaskStart   EventType = "plan_task_start"
	EventPlanTaskDone    EventType = "plan_task_done"
	EventPlanTaskFailed  EventType = "plan_task_failed"
	EventPlanTaskSkipped EventType = "plan_task_skipped"
	EventPlanReplanned   EventType = "plan_replanned"
	EventPlanDone        EventType = "plan_done"
)

// Event 循环事件（用于 SSE 推送与 step 落库）
type Event struct {
	Type       EventType
	StepIndex  int
	ToolName   string
	ToolInput  string
	ToolResult string
	Content    string
	StopReason string
	Err        string
	// CallID 对应模型返回的 tool_call id；同一步内并发调用时用于前端关联。
	CallID string
	// Plan 事件字段。
	TaskID     string
	TaskGoal   string
	TaskStatus string
	PlanJSON   string
	Thought    string
}

// Emitter 事件回调
type Emitter func(Event)

// Config 循环配置
type Config struct {
	AIID                       uint
	MaxSteps                   int
	TotalTimeout               time.Duration
	MaxRepeatActions           int
	MaxConsecutiveToolFailures int
	// MaxParallelToolCalls 是单步内并行工具调用上限；<=0 时默认 4。
	MaxParallelToolCalls int
	Temperature          float64
}

// Recorder 步骤记录接口（由 run 服务实现）
type Recorder interface {
	RecordStep(step *models.AgentStep) error
}

// Result 运行结果
type Result struct {
	Content    string
	Steps      int
	StopReason string
	Tokens     int
	Transcript []clients.ToolMessage
}

// Loop Agent 循环
type Loop struct {
	registry *tools.Registry
	batch    *tools.BatchExecutor
	cfg      Config
	recorder Recorder
}

// New 创建循环
func New(registry *tools.Registry, executor *tools.Executor, cfg Config, recorder Recorder) *Loop {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 8
	}
	if cfg.TotalTimeout <= 0 {
		cfg.TotalTimeout = 120 * time.Second
	}
	if cfg.MaxRepeatActions <= 0 {
		cfg.MaxRepeatActions = 3
	}
	if cfg.MaxConsecutiveToolFailures <= 0 {
		cfg.MaxConsecutiveToolFailures = 3
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}
	if cfg.MaxParallelToolCalls <= 0 {
		cfg.MaxParallelToolCalls = 4
	}
	return &Loop{
		registry: registry,
		batch:    tools.NewBatchExecutor(registry, executor, cfg.MaxParallelToolCalls),
		cfg:      cfg,
		recorder: recorder,
	}
}

// callLLM 抽成变量便于单测注入假模型
var callLLM = clients.CallConfiguredAIChatWithToolsStream

// Run 执行一次 Agent 任务
func (l *Loop) Run(ctx context.Context, tc *tools.Context, initial []clients.ToolMessage, userInput string, emit Emitter) (*Result, error) {
	runCtx, cancel := context.WithTimeout(ctx, l.cfg.TotalTimeout)
	defer cancel()

	messages := append([]clients.ToolMessage{}, initial...)
	messages = append(messages, clients.ToolMessage{Role: "user", Content: userInput})

	toolDefs := l.registry.Defs()
	result := &Result{}
	recentActions := make(map[string]int)
	consecutiveFailures := 0

	for step := 1; step <= l.cfg.MaxSteps; step++ {
		if err := runCtx.Err(); err != nil {
			result.StopReason = stopReasonFromCtx(err)
			result.Transcript = messages
			return result, nil
		}

		stepStart := time.Now()
		resp, err := callLLM(runCtx, l.cfg.AIID, messages, toolDefs, l.cfg.Temperature,
			func(delta clients.ToolStreamDelta) error {
				if delta.Content != "" && emit != nil {
					emit(Event{Type: EventMessage, StepIndex: step, Content: delta.Content})
				}
				return nil
			})
		if err != nil {
			stepRecord := &models.AgentStep{
				StepIndex:  step,
				Status:     models.AgentStepStatusFailed,
				DurationMs: time.Since(stepStart).Milliseconds(),
				Error:      err.Error(),
			}
			l.recordStep(stepRecord)
			result.StopReason = "llm_error"
			result.Transcript = messages
			return result, ecode.Wrap(err, "调用模型失败")
		}
		if resp.Usage != nil {
			result.Tokens += resp.Usage.TotalTokens
		}

		// 无工具调用 → 最终回答
		if len(resp.ToolCalls) == 0 {
			final := resp.Content
			messages = append(messages, clients.ToolMessage{Role: "assistant", Content: final})
			l.recordStep(&models.AgentStep{
				StepIndex:  step,
				Thought:    final,
				Status:     models.AgentStepStatusOK,
				DurationMs: time.Since(stepStart).Milliseconds(),
			})
			if emit != nil {
				emit(Event{Type: EventStep, StepIndex: step, Content: final})
				emit(Event{Type: EventDone, StepIndex: step, Content: final, StopReason: "final_answer"})
			}
			result.Content = final
			result.Steps = step
			result.StopReason = "final_answer"
			result.Transcript = messages
			return result, nil
		}

		// 重复动作检测（同一工具+参数连续出现）
		for _, call := range resp.ToolCalls {
			sig := actionSignature(call)
			recentActions[sig]++
			if recentActions[sig] >= l.cfg.MaxRepeatActions {
				result.StopReason = "repeated_action"
				result.Content = resp.Content
				result.Steps = step
				result.Transcript = messages
				l.recordStep(&models.AgentStep{
					StepIndex:   step,
					Thought:     resp.Content,
					Action:      strings.Join(toolNames(resp.ToolCalls), ","),
					ActionInput: toolInputs(resp.ToolCalls),
					Observation: "检测到重复动作，已停止",
					Status:      models.AgentStepStatusFailed,
					Error:       "repeated_action",
					DurationMs:  time.Since(stepStart).Milliseconds(),
				})
				if emit != nil {
					emit(Event{Type: EventDone, StepIndex: step, StopReason: "repeated_action", Content: resp.Content})
				}
				return result, nil
			}
		}

		// 回填 assistant(tool_calls)
		messages = append(messages, clients.ToolMessage{
			Role:             "assistant",
			Content:          resp.Content,
			ToolCalls:        resp.ToolCalls,
			ReasoningContent: resp.ReasoningContent,
		})

		// 同一 assistant 消息内的 tool calls 交给批调度器：
		// 全部并发安全 → 并行；否则整批串行。结果按原始顺序回填，
		// 保证 tool message 与 tool_call_id 一一对应。
		for _, call := range resp.ToolCalls {
			if emit != nil {
				emit(Event{Type: EventToolCall, StepIndex: step, CallID: call.ID,
					ToolName: call.Function.Name, ToolInput: call.Function.Arguments})
			}
		}
		outcomes := l.batch.Execute(runCtx, tc, resp.ToolCalls)

		var observations []string
		for _, outcome := range outcomes {
			call := outcome.Call
			observation := outcome.Result.Content
			if outcome.Err != nil {
				consecutiveFailures++
				observation = "工具执行失败: " + outcome.Err.Error()
			} else {
				consecutiveFailures = 0
			}
			observations = append(observations, fmt.Sprintf("[%s] %s", call.Function.Name, observation))
			messages = append(messages, clients.ToolMessage{
				Role:       "tool",
				Content:    observation,
				ToolCallID: call.ID,
				Name:       call.Function.Name,
			})
			if emit != nil {
				emit(Event{Type: EventToolResult, StepIndex: step, CallID: call.ID,
					ToolName: call.Function.Name, ToolResult: observation})
			}
		}

		// ctx 在批次执行期间被取消/超时：不再进入下一轮 LLM 调用，
		// 也不要把取消误计为连续工具失败。
		if err := runCtx.Err(); err != nil {
			result.StopReason = stopReasonFromCtx(err)
			result.Steps = step
			result.Transcript = messages
			return result, nil
		}

		l.recordStep(&models.AgentStep{
			StepIndex:   step,
			Thought:     resp.Content,
			Action:      strings.Join(toolNames(resp.ToolCalls), ","),
			ActionInput: toolInputs(resp.ToolCalls),
			Observation: strings.Join(observations, "\n"),
			Status:      models.AgentStepStatusOK,
			DurationMs:  time.Since(stepStart).Milliseconds(),
		})
		if emit != nil {
			emit(Event{Type: EventStep, StepIndex: step, Thought: resp.Content, ToolName: strings.Join(toolNames(resp.ToolCalls), ",")})
		}

		if consecutiveFailures >= l.cfg.MaxConsecutiveToolFailures {
			result.StopReason = "tool_failures"
			result.Content = "工具连续执行失败，已停止。请检查工具配置或补充信息后重试。"
			result.Steps = step
			result.Transcript = messages
			if emit != nil {
				emit(Event{Type: EventDone, StepIndex: step, StopReason: "tool_failures", Content: result.Content})
			}
			return result, nil
		}
	}

	result.StopReason = "max_steps"
	result.Content = "已达到最大推理步数，未能完成任务。"
	result.Steps = l.cfg.MaxSteps
	result.Transcript = messages
	if emit != nil {
		emit(Event{Type: EventDone, StepIndex: l.cfg.MaxSteps, StopReason: "max_steps", Content: result.Content})
	}
	return result, nil
}

func (l *Loop) recordStep(step *models.AgentStep) {
	if l.recorder == nil {
		return
	}
	if err := l.recorder.RecordStep(step); err != nil {
		logrus.Warnf("[agent-loop] record step failed: %v", err)
	}
}

func actionSignature(call clients.ToolCall) string {
	sum := sha256.Sum256([]byte(call.Function.Name + "|" + call.Function.Arguments))
	return call.Function.Name + ":" + hex.EncodeToString(sum[:8])
}

func toolNames(calls []clients.ToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.Function.Name)
	}
	return names
}

func toolInputs(calls []clients.ToolCall) string {
	parts := make([]string, 0, len(calls))
	for _, c := range calls {
		parts = append(parts, c.Function.Name+":"+c.Function.Arguments)
	}
	return strings.Join(parts, "\n")
}

func stopReasonFromCtx(err error) string {
	if err == context.Canceled {
		return "canceled"
	}
	if err == context.DeadlineExceeded {
		return "timeout"
	}
	return err.Error()
}
