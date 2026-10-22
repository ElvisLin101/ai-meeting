package clients

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ============================================================
// Function Calling 客户端（独立于现有 PromptMessage 调用链）
//
// 现有 CallConfiguredAIChat* 只支持 role/content, 供 AI 对话与面试评分使用;
// 这里新增 ToolMessage / ToolDef / ToolCall, 专门服务 Agent 的工具调用,
// 避免改动现有接口破坏既有调用方。
// ============================================================

// ToolFunctionDef 工具函数定义（JSON Schema）
type ToolFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolDef OpenAI tools 数组元素
type ToolDef struct {
	Type     string          `json:"type"` // 固定 "function"
	Function ToolFunctionDef `json:"function"`
}

// ToolCallFunction 模型返回的调用函数
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// ToolCall 模型发起的一次工具调用
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolMessage function calling 消息
type ToolMessage struct {
	Role             string     `json:"role"` // system/user/assistant/tool
	Content          string     `json:"content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	Name             string     `json:"name,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
}

// Usage token 用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolChatResult 一次模型调用的结果
type ToolChatResult struct {
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCall
	FinishReason     string
	Usage            *Usage
}

// ToolStreamDelta 流式增量回调
type ToolStreamDelta struct {
	Content          string
	ReasoningContent string
}

// CallConfiguredAIChatWithTools 非流式 function calling 调用
func CallConfiguredAIChatWithTools(ctx context.Context, aiID uint, messages []ToolMessage, tools []ToolDef, temperature float64) (*ToolChatResult, error) {
	prop, err := loadEnabledAiProperty(aiID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(prop.Endpoint) == "" {
		return nil, errors.New("enabled ai model has empty endpoint")
	}

	payload := map[string]interface{}{
		"model":       resolveAiModelName(prop),
		"temperature": temperature,
		"stream":      false,
		"messages":    messages,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	req, err := newAIChatRequest(ctx, prop, payload)
	if err != nil {
		return nil, err
	}

	resp, err := httpClientStd.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("ai tool chat status %d: %s", resp.StatusCode, string(body))
	}

	return parseToolChatResponse(body)
}

// CallConfiguredAIChatWithToolsStream 流式 function calling 调用
// onDelta 只在有文本增量时回调; 工具调用片段在内部累积, 结束后一次性返回。
func CallConfiguredAIChatWithToolsStream(ctx context.Context, aiID uint, messages []ToolMessage, tools []ToolDef, temperature float64, onDelta func(ToolStreamDelta) error) (*ToolChatResult, error) {
	prop, err := loadEnabledAiProperty(aiID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(prop.Endpoint) == "" {
		return nil, errors.New("enabled ai model has empty endpoint")
	}

	payload := map[string]interface{}{
		"model":       resolveAiModelName(prop),
		"temperature": temperature,
		"stream":      true,
		"messages":    messages,
		"stream_options": map[string]interface{}{
			"include_usage": true,
		},
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	req, err := newAIChatRequest(ctx, prop, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := httpClientStream.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, readErr
		}
		return nil, fmt.Errorf("ai tool chat stream status %d: %s", resp.StatusCode, string(body))
	}

	result := &ToolChatResult{}
	accumulator := newToolCallAccumulator()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk toolStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("parse tool stream chunk: %w", err)
		}
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			if choice.Delta.Content != "" {
				result.Content += choice.Delta.Content
			}
			if choice.Delta.ReasoningContent != "" {
				result.ReasoningContent += choice.Delta.ReasoningContent
			}
			if len(choice.Delta.ToolCalls) > 0 {
				accumulator.add(choice.Delta.ToolCalls)
			}
			if choice.FinishReason != "" {
				result.FinishReason = choice.FinishReason
			}
			if choice.Delta.Content != "" || choice.Delta.ReasoningContent != "" {
				if onDelta != nil {
					if err := onDelta(ToolStreamDelta{
						Content:          choice.Delta.Content,
						ReasoningContent: choice.Delta.ReasoningContent,
					}); err != nil {
						return nil, err
					}
				}
			}
		}
		if chunk.Usage != nil {
			result.Usage = chunk.Usage
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	result.ToolCalls = accumulator.calls()
	if result.FinishReason == "" && len(result.ToolCalls) > 0 {
		result.FinishReason = "tool_calls"
	}
	return result, nil
}

// ============================================================
// 非流式解析
// ============================================================

type toolChatResponse struct {
	Choices []struct {
		Message struct {
			Content          string     `json:"content"`
			ReasoningContent string     `json:"reasoning_content"`
			ToolCalls        []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

func parseToolChatResponse(body []byte) (*ToolChatResult, error) {
	var resp toolChatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse tool chat response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("ai tool chat response has no choices")
	}
	choice := resp.Choices[0]
	result := &ToolChatResult{
		Content:          choice.Message.Content,
		ReasoningContent: choice.Message.ReasoningContent,
		ToolCalls:        choice.Message.ToolCalls,
		FinishReason:     choice.FinishReason,
		Usage:            resp.Usage,
	}
	if result.Content == "" && len(result.ToolCalls) == 0 {
		return nil, errors.New("ai tool chat response empty")
	}
	return result, nil
}

// ============================================================
// 流式 tool_calls 拼接
// ============================================================

type toolStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string             `json:"content"`
			ReasoningContent string             `json:"reasoning_content"`
			ToolCalls        []toolCallFragment `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

type toolCallFragment struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolCallAccumulator struct {
	order []int
	byIdx map[int]*ToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{byIdx: make(map[int]*ToolCall)}
}

func (a *toolCallAccumulator) add(fragments []toolCallFragment) {
	for _, frag := range fragments {
		call, ok := a.byIdx[frag.Index]
		if !ok {
			call = &ToolCall{Type: "function"}
			a.byIdx[frag.Index] = call
			a.order = append(a.order, frag.Index)
		}
		if frag.ID != "" {
			call.ID = frag.ID
		}
		if frag.Type != "" {
			call.Type = frag.Type
		}
		if frag.Function.Name != "" {
			call.Function.Name += frag.Function.Name
		}
		call.Function.Arguments += frag.Function.Arguments
	}
}

func (a *toolCallAccumulator) calls() []ToolCall {
	calls := make([]ToolCall, 0, len(a.order))
	for _, idx := range a.order {
		if call, ok := a.byIdx[idx]; ok {
			calls = append(calls, *call)
		}
	}
	return calls
}
