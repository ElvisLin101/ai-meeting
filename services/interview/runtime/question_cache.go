package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// ============================================================
// QuestionCache 面试题目/建议/简历上下文的 Redis 读写
// questions/suggestions/followUpQuestions: Hash(题号→题面)
// resumeContext: JSON 字符串
// resumeScore/direction: 简单 Value
// ============================================================

// QuestionCache 题目材料缓存
type QuestionCache struct {
	rdb *redis.Client
}

// NewQuestionCache 创建 QuestionCache
func NewQuestionCache(rdb *redis.Client) *QuestionCache {
	return &QuestionCache{rdb: rdb}
}

// SaveQuestions 写入主问题列表（题号→题面 Hash）
func (c *QuestionCache) SaveQuestions(ctx context.Context, sessionID string, questions map[string]string) error {
	key := questionsKey(sessionID)
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, key)
	if len(questions) > 0 {
		args := make([]interface{}, 0, len(questions)*2)
		for k, v := range questions {
			args = append(args, k, v)
		}
		pipe.HSet(ctx, key, args...)
	}
	pipe.Expire(ctx, key, cacheTTLHours*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

// GetQuestion 按题号读取题面
func (c *QuestionCache) GetQuestion(ctx context.Context, sessionID, questionNumber string) (string, error) {
	val, err := c.rdb.HGet(ctx, questionsKey(sessionID), questionNumber).Result()
	if err == redis.Nil {
		// 主问题 miss → 查追问题
		val, err = c.rdb.HGet(ctx, followUpQuestionsKey(sessionID), questionNumber).Result()
		if err == redis.Nil {
			return "", nil
		}
	}
	return val, err
}

// GetAllQuestions 读取全部主问题（题号→题面）
func (c *QuestionCache) GetAllQuestions(ctx context.Context, sessionID string) (map[string]string, error) {
	result, err := c.rdb.HGetAll(ctx, questionsKey(sessionID)).Result()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SaveFollowUpQuestion 写入追问题（题号→题面 Hash）
func (c *QuestionCache) SaveFollowUpQuestion(ctx context.Context, sessionID, questionNumber, content string) error {
	key := followUpQuestionsKey(sessionID)
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, key, questionNumber, content)
	pipe.Expire(ctx, key, cacheTTLHours*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

// SaveSuggestions 写入建议列表（题号→建议 Hash）
func (c *QuestionCache) SaveSuggestions(ctx context.Context, sessionID string, suggestions map[string]string) error {
	if len(suggestions) == 0 {
		return nil
	}
	key := suggestionsKey(sessionID)
	pipe := c.rdb.Pipeline()
	args := make([]interface{}, 0, len(suggestions)*2)
	for k, v := range suggestions {
		args = append(args, k, v)
	}
	pipe.HSet(ctx, key, args...)
	pipe.Expire(ctx, key, cacheTTLHours*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

// GetSuggestions 读取全部建议（题号→建议）
func (c *QuestionCache) GetSuggestions(ctx context.Context, sessionID string) (map[string]string, error) {
	result, err := c.rdb.HGetAll(ctx, suggestionsKey(sessionID)).Result()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SaveResumeContext 写入简历上下文（JSON 字符串）
func (c *QuestionCache) SaveResumeContext(ctx context.Context, sessionID string, resumeCtx map[string]interface{}) error {
	if len(resumeCtx) == 0 {
		return nil
	}
	payload, err := json.Marshal(resumeCtx)
	if err != nil {
		return err
	}
	key := resumeContextKey(sessionID)
	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, key, payload, cacheTTLHours*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}

// GetResumeContextText 读取简历上下文文本（用于评分 prompt，clip 2000）
func (c *QuestionCache) GetResumeContextText(ctx context.Context, sessionID string) (string, error) {
	payload, err := c.rdb.Get(ctx, resumeContextKey(sessionID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return payload, nil
}

// SaveResumeScore 写入简历评分
func (c *QuestionCache) SaveResumeScore(ctx context.Context, sessionID string, score int) error {
	return c.rdb.Set(ctx, resumeScoreKey(sessionID), strconv.Itoa(score), cacheTTLHours*time.Hour).Err()
}

// GetResumeScore 读取简历评分
func (c *QuestionCache) GetResumeScore(ctx context.Context, sessionID string) (int, error) {
	val, err := c.rdb.Get(ctx, resumeScoreKey(sessionID)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// SaveDirection 写入面试方向
func (c *QuestionCache) SaveDirection(ctx context.Context, sessionID, direction string) error {
	if direction == "" {
		return nil
	}
	return c.rdb.Set(ctx, directionKey(sessionID), direction, cacheTTLHours*time.Hour).Err()
}

// GetDirection 读取面试方向
func (c *QuestionCache) GetDirection(ctx context.Context, sessionID string) (string, error) {
	val, err := c.rdb.Get(ctx, directionKey(sessionID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}
