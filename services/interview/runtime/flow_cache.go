package runtime

import (
	"ai-meeting/models"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// ============================================================
// FlowCache 面试流程状态的 Redis Hash 读写 + CAS 乐观锁
// key: interview:flow:session:{sid}, 7 个 field, TTL 24h
// ============================================================

const flowCASMaxRetries = 5

// flowCASUpdateScript CAS 乐观锁更新 flow
// ARGV: [expectedVersion, status, currentIndex, currentQuestionNumber,
//        totalQuestions, followUpCount, maxFollowUp, newVersion, ttlSeconds]
const flowCASUpdateScript = `
local current = redis.call('HGET', KEYS[1], 'version')
if current == false or tostring(current) ~= tostring(ARGV[1]) then
	return 0
end
redis.call('HSET', KEYS[1],
	'status', ARGV[2],
	'currentIndex', ARGV[3],
	'currentQuestionNumber', ARGV[4],
	'totalQuestions', ARGV[5],
	'followUpCount', ARGV[6],
	'maxFollowUp', ARGV[7],
	'version', ARGV[8])
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[9]))
return 1
`

// FlowCache 面试流程状态缓存
type FlowCache struct {
	rdb *redis.Client
}

// NewFlowCache 创建 FlowCache
func NewFlowCache(rdb *redis.Client) *FlowCache {
	return &FlowCache{rdb: rdb}
}

// GetFlow 读取当前 flow 状态
func (c *FlowCache) GetFlow(ctx context.Context, sessionID string) (*models.InterviewFlowState, error) {
	key := flowKey(sessionID)
	result, err := c.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil // flow 不存在
	}
	return parseFlowState(result)
}

// SaveFlow 直接覆盖写入 flow（用于初始化和回滚，不走 CAS）
func (c *FlowCache) SaveFlow(ctx context.Context, sessionID string, state *models.InterviewFlowState) error {
	key := flowKey(sessionID)
	_, err := c.rdb.HSet(ctx, key,
		"status", string(state.Status),
		"currentIndex", state.CurrentIndex,
		"currentQuestionNumber", state.CurrentQuestionNumber,
		"totalQuestions", state.TotalQuestions,
		"followUpCount", state.FollowUpCount,
		"maxFollowUp", state.MaxFollowUp,
		"version", state.Version,
	).Result()
	if err != nil {
		return err
	}
	return c.rdb.Expire(ctx, key, cacheTTLHours*time.Hour).Err()
}

// MutateFlow 读→改→CAS 写，最多重试 flowCASMaxRetries 次
// mutator 在读取的 state 基础上做修改，返回修改后的 state
func (c *FlowCache) MutateFlow(ctx context.Context, sessionID string, mutator func(*models.InterviewFlowState) (*models.InterviewFlowState, error)) (*models.InterviewFlowState, error) {
	key := flowKey(sessionID)

	for attempt := 0; attempt < flowCASMaxRetries; attempt++ {
		current, err := c.GetFlow(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, fmt.Errorf("flow not found for session %s", sessionID)
		}

		next, err := mutator(current)
		if err != nil {
			return nil, err
		}
		next.Version = current.Version + 1

		ok, err := c.casUpdate(ctx, key, current.Version, next)
		if err != nil {
			return nil, err
		}
		if ok {
			return next, nil
		}
		// CAS 失败（版本冲突），退避后重试
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}

	// 重试用尽，读最新返回
	return c.GetFlow(ctx, sessionID)
}

// casUpdate 用 Lua 脚本做 CAS 更新
func (c *FlowCache) casUpdate(ctx context.Context, key string, expectedVersion int, state *models.InterviewFlowState) (bool, error) {
	result, err := redis.NewScript(flowCASUpdateScript).Run(ctx, c.rdb,
		[]string{key},
		expectedVersion,
		string(state.Status),
		state.CurrentIndex,
		state.CurrentQuestionNumber,
		state.TotalQuestions,
		state.FollowUpCount,
		state.MaxFollowUp,
		state.Version,
		cacheTTLHours*3600,
	).Result()
	if err != nil {
		return false, err
	}
	return result.(int64) == 1, nil
}

// parseFlowState 从 Redis Hash map 解析 InterviewFlowState
func parseFlowState(m map[string]string) (*models.InterviewFlowState, error) {
	currentIndex, _ := strconv.Atoi(m["currentIndex"])
	totalQuestions, _ := strconv.Atoi(m["totalQuestions"])
	followUpCount, _ := strconv.Atoi(m["followUpCount"])
	maxFollowUp, _ := strconv.Atoi(m["maxFollowUp"])
	version, _ := strconv.Atoi(m["version"])

	return &models.InterviewFlowState{
		Status:              models.InterviewFlowStatus(m["status"]),
		CurrentIndex:        currentIndex,
		CurrentQuestionNumber: m["currentQuestionNumber"],
		TotalQuestions:      totalQuestions,
		FollowUpCount:       followUpCount,
		MaxFollowUp:         maxFollowUp,
		Version:             version,
	}, nil
}
