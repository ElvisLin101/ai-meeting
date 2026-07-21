package evaluation

import (
	"ai-meeting/clients"
	"ai-meeting/pkg/singleflight"
	"ai-meeting/repositories"
	"ai-meeting/services/metric"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
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
// 接入分布式 SingleFlight: 同一简历并发出题只调一次
// JSON mode + schema 校验，校验失败直接返回 error（重试由客户端发起新 singleflight）
func (s *ExtractionService) ExtractQuestions(ctx context.Context, resumeContent string) (*ExtractionResult, error) {
	sfKey := fmt.Sprintf("interview:extract:%s", hashContent(resumeContent))

	result, err := repositories.SingleFlight.Do(ctx, sfKey, func(ctx context.Context, writer *singleflight.StreamWriter) (interface{}, error) {
		messages := buildExtractionPromptMessages(resumeContent)
		start := time.Now()

		var replyBuilder strings.Builder
		err := clients.CallConfiguredAIChatStreamWithJSON(ctx, 0, messages, extractionTemperature, func(chunk clients.ChatStreamChunk) error {
			if chunk.Content != "" {
				replyBuilder.WriteString(chunk.Content)
				writer.Write([]byte(chunk.Content))
			}
			return nil
		})
		durationMs := time.Since(start).Milliseconds()

		if err != nil {
			metric.GetMetricService().RecordAICall("ai_call", "extract", "", false, "ai_call_fail", false, durationMs, "")
			return nil, err
		}

		reply := replyBuilder.String()
		parsed := ExtractStructuredResult(reply, "questions", "suggestions", "sugest", "type", "resumeScore")
		if parsed == nil {
			metric.GetMetricService().RecordAICall("ai_call", "extract", "", false, "parse_fail", false, durationMs, "")
			return nil, errors.New("failed to parse extraction result")
		}

		result := s.normalizeResult(parsed)

		// schema 校验: questions 必须非空
		if len(result.Questions) == 0 {
			metric.GetMetricService().RecordAICall("ai_call", "extract", "", false, "schema_fail", false, durationMs, "")
			return nil, errors.New("schema validation failed: questions is empty")
		}

		metric.GetMetricService().RecordAICall("ai_call", "extract", "", true, "", false, durationMs, "")
		return result, nil
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
