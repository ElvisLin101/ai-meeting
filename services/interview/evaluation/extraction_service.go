package evaluation

import (
	"ai-meeting/clients"
	"ai-meeting/pkg/singleflight"
	"ai-meeting/repositories"
	"context"
	"errors"
	"fmt"
	"strings"
)

// ============================================================
// ExtractionService 面试出题服务
// 调 DeepSeek 从简历内容提取面试题
// 接入分布式 SingleFlight: 同一简历内容的并发出题只调一次
// ============================================================

// ExtractionResult 出题结果（归一化后）
type ExtractionResult struct {
	Questions     []string               // 面试题列表
	Suggestions   []string               // 出题建议
	Type          string                 // 面试方向（如"后端"）
	ResumeScore   int                    // 简历匹配分 [0,100]
	ResumeContext map[string]interface{} // 简历上下文（剔除 questions/suggestions 后的剩余字段，供后续评分用）
}

// ExtractionService 出题服务（无状态）
type ExtractionService struct{}

// NewExtractionService 创建出题服务
func NewExtractionService() *ExtractionService {
	return &ExtractionService{}
}

// ExtractQuestions 调 DeepSeek 出题
// 接入分布式 SingleFlight: 同一简历内容的并发出题只调一次
func (s *ExtractionService) ExtractQuestions(ctx context.Context, resumeContent string) (*ExtractionResult, error) {
	// 构造 singleflight key: 基于简历内容, 同一简历只出一次题
	sfKey := fmt.Sprintf("interview:extract:%s", hashContent(resumeContent))

	result, err := repositories.SingleFlight.Do(ctx, sfKey, func(ctx context.Context, writer *singleflight.StreamWriter) (interface{}, error) {
		messages := buildExtractionPromptMessages(resumeContent)

		var replyBuilder strings.Builder
		err := clients.CallConfiguredAIChatStream(ctx, 0, messages, extractionTemperature, func(chunk clients.ChatStreamChunk) error {
			if chunk.Content != "" {
				replyBuilder.WriteString(chunk.Content)
				writer.Write([]byte(chunk.Content))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		reply := replyBuilder.String()
		parsed := ExtractStructuredResult(reply, "questions", "suggestions", "sugest", "type", "resumeScore")
		if parsed == nil {
			return nil, errors.New("failed to parse extraction result from AI response")
		}
		return s.normalizeResult(parsed), nil
	})
	if err != nil {
		return nil, err
	}

	extractResult, ok := result.(*ExtractionResult)
	if !ok || extractResult == nil {
		return nil, errors.New("invalid extraction result type from singleflight")
	}
	return extractResult, nil
}

// normalizeResult 字段别名归一化
func (s *ExtractionService) normalizeResult(m map[string]interface{}) *ExtractionResult {
	result := &ExtractionResult{
		Questions:   AsStringList(extractByAliases(m, "questions")),
		Suggestions: AsStringList(extractByAliases(m, "sugest", "suggestions")),
		Type:        AsString(extractByAliases(m, "type", "interviewType", "direction", "interviewDirection")),
		ResumeScore: ParseScore(extractByAliases(m, "resumeScore")),
	}

	// ResumeContext = 剔除结构化字段后的剩余 map（供评分阶段用）
	resumeCtx := make(map[string]interface{})
	excludeKeys := map[string]bool{
		"questions": true, "suggestions": true, "sugest": true,
		"resumeScore": true, "score": true, "type": true, "interviewType": true,
	}
	for k, v := range m {
		if !excludeKeys[k] {
			resumeCtx[k] = v
		}
	}
	result.ResumeContext = resumeCtx

	if result.Questions == nil {
		result.Questions = []string{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []string{}
	}

	return result
}
