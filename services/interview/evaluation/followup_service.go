package evaluation

import (
	"ai-meeting/clients"
	"context"
	"errors"
	"strings"
)

// ============================================================
// FollowUpService 面试追问生成服务
// 调 DeepSeek 流式生成针对性的追问问题
// ============================================================

// FollowUpResult 追问结果
type FollowUpResult struct {
	Question     string // 追问题面
	EndInterview bool   // 是否应结束面试不再追问
}

// FollowUpService 追问服务（无状态）
type FollowUpService struct{}

// NewFollowUpService 创建追问服务
func NewFollowUpService() *FollowUpService {
	return &FollowUpService{}
}

// GenerateFollowUp 调 DeepSeek 流式生成追问
// 输入: 题面、答案、缺失点、当前追问轮次、追问上限
// followUpCount 是已追问轮次（0 表示还没追问过）
func (s *FollowUpService) GenerateFollowUp(
	ctx context.Context,
	questionContent, answerContent string,
	missingPoints []string,
	followUpCount, maxFollowUp int,
) (*FollowUpResult, error) {
	// 次数兜底
	if maxFollowUp <= 0 {
		maxFollowUp = 2
	}
	if followUpCount >= maxFollowUp {
		return &FollowUpResult{EndInterview: true}, nil
	}

	messages := buildFollowUpPromptMessages(questionContent, answerContent, missingPoints, followUpCount, maxFollowUp)

	var replyBuilder strings.Builder
	err := clients.CallConfiguredAIChatStream(ctx, 0, messages, followUpTemperature, func(chunk clients.ChatStreamChunk) error {
		if chunk.Content != "" {
			replyBuilder.WriteString(chunk.Content)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	reply := replyBuilder.String()
	parsed := ExtractStructuredResult(reply, "ask_to_user", "end_interview")
	if parsed == nil {
		return nil, errors.New("failed to parse follow-up result from AI response")
	}

	result := &FollowUpResult{
		Question:     sanitizeFollowUpQuestion(AsString(extractByAliases(parsed, "ask_to_user", "ask"))),
		EndInterview: AsBoolean(extractByAliases(parsed, "end_interview", "endInterview")),
	}

	// 追问题为空视为结束
	if result.Question == "" {
		result.EndInterview = true
	}

	return result, nil
}

// sanitizeFollowUpQuestion 清洗追问题文本
// 过滤 none/null/N/A/-/__FINISH__，不以 ? 结尾则补 ?，clip 100
func sanitizeFollowUpQuestion(question string) string {
	normalized := strings.TrimSpace(question)
	if normalized == "" {
		return ""
	}

	lower := strings.ToLower(normalized)
	if lower == "none" || lower == "null" || lower == "n/a" || normalized == "-" || lower == "__finish__" {
		return ""
	}

	// 补问号
	if !strings.HasSuffix(normalized, "?") && !strings.HasSuffix(normalized, "？") {
		normalized = normalized + "?"
	}

	// clip 100
	if len(normalized) > 100 {
		normalized = normalized[:100]
	}

	return normalized
}
