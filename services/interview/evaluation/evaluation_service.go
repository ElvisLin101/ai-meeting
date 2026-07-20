package evaluation

import (
	"ai-meeting/clients"
	"context"
	"errors"
	"fmt"
	"strings"
)

// ============================================================
// EvaluationService 面试评分服务
// 调 DeepSeek 流式评分，返回归一化的 EvaluationResult
// 并发控制由 pipeline 的题级锁保证（同一题同时只允许一个评分）
// ============================================================

// EvaluationResult 评分结果（归一化后）
type EvaluationResult struct {
	Score            int      // [0,100]
	Feedback         string   // 反馈
	FollowUpNeeded   bool     // 是否需要追问
	FollowUpQuestion string   // 追问题面（FollowUpNeeded 为 true 时非空）
	MissingPoints    []string // 缺失点
	LogicOK          bool     // 答案逻辑是否正确
}

// EvaluationService 评分服务（无状态）
type EvaluationService struct{}

// NewEvaluationService 创建评分服务
func NewEvaluationService() *EvaluationService {
	return &EvaluationService{}
}

// EvaluateAnswer 调 DeepSeek 流式评分
// 输入: 题面、候选人答案、简历上下文
func (s *EvaluationService) EvaluateAnswer(ctx context.Context, questionContent, answerContent, resumeContext string) (*EvaluationResult, error) {
	messages := buildScorePromptMessages(questionContent, answerContent, resumeContext)

	var replyBuilder strings.Builder
	err := clients.CallConfiguredAIChatStream(ctx, 0, messages, scoreTemperature, func(chunk clients.ChatStreamChunk) error {
		if chunk.Content != "" {
			replyBuilder.WriteString(chunk.Content)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	reply := replyBuilder.String()
	parsed := ExtractStructuredResult(reply, "score", "feedback", "follow_up_needed", "missing_points")
	if parsed == nil {
		return nil, errors.New("failed to parse evaluation result from AI response")
	}
	return s.normalizeResult(parsed), nil
}

// normalizeResult 字段别名归一化
func (s *EvaluationService) normalizeResult(m map[string]interface{}) *EvaluationResult {
	result := &EvaluationResult{
		Score:            ParseScore(extractByAliases(m, "score", "total_score", "composite_score")),
		Feedback:         AsString(extractByAliases(m, "feedback", "comment", "suggestion")),
		FollowUpQuestion: AsString(extractByAliases(m, "follow_up_question", "followUpQuestion", "ask_to_user", "ask")),
		MissingPoints:    AsStringList(extractByAliases(m, "missing_points", "missingPoints", "lack_points")),
		LogicOK:          AsBoolean(extractByAliases(m, "logic_ok", "logicOk")),
	}

	// follow_up_needed: 优先取 AI 返回，缺失时推断
	followUpNeededVal := extractByAliases(m, "follow_up_needed", "followUpNeeded")
	if followUpNeededVal != nil {
		result.FollowUpNeeded = AsBoolean(followUpNeededVal)
	} else {
		result.FollowUpNeeded = s.inferFollowUpNeeded(result)
	}

	// 补默认值
	if result.MissingPoints == nil {
		result.MissingPoints = []string{}
	}

	return result
}

// inferFollowUpNeeded 缺失 follow_up_needed 时推断
// 规则: 逻辑不正确 OR 有缺失点 OR 有追问题 → true
func (s *EvaluationService) inferFollowUpNeeded(r *EvaluationResult) bool {
	return !r.LogicOK || len(r.MissingPoints) > 0 || r.FollowUpQuestion != ""
}

// hashContent 简单 hash 用于 singleflight key
func hashContent(parts ...string) string {
	h := fmt.Sprintf("%v", parts)
	if len(h) > 64 {
		h = h[:64]
	}
	return h
}
