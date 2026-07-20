package flow

import (
	"ai-meeting/dto"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// ============================================================
// IdempotencyService 答题幂等服务
// 两个 Redis key:
//   processing: SETNX 抢占"正在处理"标记 (TTL 300s)
//   replay: 成功结果缓存 (TTL 24h), 客户端重试时直接回放
// ============================================================

const (
	processingTTL = 300 * time.Second
	replayTTL     = 24 * time.Hour

	IdempotencyNew       = "NEW"        // 新请求, 正常处理
	IdempotencySucceeded = "SUCCEEDED"  // 已成功过, 回放旧结果
	IdempotencyProcessing = "PROCESSING" // 别人正在处理, 快速失败
)

// TryStartResult 幂等检查结果
type TryStartResult struct {
	Status   string
	Response *dto.InterviewAnswerRespDTO // SUCCEEDED 时回放旧结果
}

// IdempotencyService 幂等服务
type IdempotencyService struct {
	rdb *redis.Client
}

// NewIdempotencyService 创建幂等服务
func NewIdempotencyService(rdb *redis.Client) *IdempotencyService {
	return &IdempotencyService{rdb: rdb}
}

// TryStart 幂等检查
// 先查 replay key → 命中则 SUCCEEDED 回放; 否则 SetNX processing key → NEW 或 PROCESSING
func (s *IdempotencyService) TryStart(ctx context.Context, sessionID, requestID string) (*TryStartResult, error) {
	if requestID == "" {
		return &TryStartResult{Status: IdempotencyNew}, nil
	}

	// 1. 先查 replay key
	replayPayload, err := s.rdb.Get(ctx, replayKey(sessionID, requestID)).Result()
	if err == nil && replayPayload != "" {
		var resp dto.InterviewAnswerRespDTO
		if err := json.Unmarshal([]byte(replayPayload), &resp); err == nil {
			return &TryStartResult{Status: IdempotencySucceeded, Response: &resp}, nil
		}
	}
	if err != nil && err != redis.Nil {
		return nil, err
	}

	// 2. SetNX processing key
	started, err := s.rdb.SetNX(ctx, processingKey(sessionID, requestID), "1", processingTTL).Result()
	if err != nil {
		return nil, err
	}
	if started {
		return &TryStartResult{Status: IdempotencyNew}, nil
	}
	return &TryStartResult{Status: IdempotencyProcessing}, nil
}

// MarkSucceeded 标记成功: 写 replay key(24h) + 删 processing key
func (s *IdempotencyService) MarkSucceeded(ctx context.Context, sessionID, requestID string, resp *dto.InterviewAnswerRespDTO) error {
	if requestID == "" {
		return nil
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, replayKey(sessionID, requestID), payload, replayTTL).Err(); err != nil {
		return err
	}
	return s.rdb.Del(ctx, processingKey(sessionID, requestID)).Err()
}

// ClearProcessing 清除 processing key（失败时让客户端可重试）
func (s *IdempotencyService) ClearProcessing(ctx context.Context, sessionID, requestID string) error {
	if requestID == "" {
		return nil
	}
	return s.rdb.Del(ctx, processingKey(sessionID, requestID)).Err()
}

// NormalizeRequestId requestId 为空时生成 auto- + sha256(sessionId|questionNumber|answerContent)[:32]
func NormalizeRequestId(sessionID, questionNumber, answerContent string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", sessionID, questionNumber, answerContent)))
	return "auto-" + hex.EncodeToString(h[:16])
}

func processingKey(sessionID, requestID string) string {
	return fmt.Sprintf("interview:answer:idempotency:processing:%s:%s", sessionID, requestID)
}

func replayKey(sessionID, requestID string) string {
	return fmt.Sprintf("interview:answer:idempotency:replay:%s:%s", sessionID, requestID)
}
