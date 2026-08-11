package runtime

import (
	"ai-meeting/models"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// ============================================================
// FlowCache 面试流程状态的 Redis Hash 读写 + CAS 乐观锁
// key: interview:flow:session:{sid}, 7 个 field, TTL 24h
// ============================================================

const flowCASMaxRetries = 5

// flowCASUpdateScript CAS 乐观锁更新 flow
// ARGV: [expectedVersion, status, currentIndex, currentQuestionNumber,
//
//	totalQuestions, followUpCount, maxFollowUp, newVersion, ttlSeconds]
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

// flowRollbackScript 条件回滚 flow
// 仅当当前 version == ARGV[1]（调用方自己推进后的版本）才覆盖写回快照,
// 否则说明 flow 已被其他请求合法推进, 放弃回滚(返回 0), 防止回滚砸掉并发已提交的进度。
// ARGV: [expectedVersion, status, currentIndex, currentQuestionNumber,
//
//	totalQuestions, followUpCount, maxFollowUp, snapshotVersion, ttlSeconds]
const flowRollbackScript = `
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

// RollbackFlowCAS 条件回滚: 仅当当前 version == expectedVersion 时覆盖写回快照。
// expectedVersion 为调用方推进 flow 后拿到的 version——只有"当前仍是自己推进后的版本"
// 才允许回滚; 若已被其他请求推进, 返回 (false, nil) 表示放弃回滚。
func (c *FlowCache) RollbackFlowCAS(ctx context.Context, sessionID string, expectedVersion int, state *models.InterviewFlowState) (bool, error) {
	key := flowKey(sessionID)
	args := []interface{}{
		fmt.Sprintf("%d", expectedVersion),
		string(state.Status),
		fmt.Sprintf("%d", state.CurrentIndex),
		state.CurrentQuestionNumber,
		fmt.Sprintf("%d", state.TotalQuestions),
		fmt.Sprintf("%d", state.FollowUpCount),
		fmt.Sprintf("%d", state.MaxFollowUp),
		fmt.Sprintf("%d", state.Version),
		fmt.Sprintf("%d", cacheTTLHours*3600),
	}
	result, err := redis.NewScript(flowRollbackScript).Run(ctx, c.rdb, []string{key}, args...).Result()
	if err != nil {
		return false, err
	}
	r, ok := result.(int64)
	return ok && r == 1, nil
}

// SaveFlow 直接覆盖写入 flow（用于初始化, 不走 CAS; 回滚请用 RollbackFlowCAS）
func (c *FlowCache) SaveFlow(ctx context.Context, sessionID string, state *models.InterviewFlowState) error {
	key := flowKey(sessionID)
	_, err := c.rdb.HSet(ctx, key,
		"status", string(state.Status),
		"currentIndex", fmt.Sprintf("%d", state.CurrentIndex),
		"currentQuestionNumber", state.CurrentQuestionNumber,
		"totalQuestions", fmt.Sprintf("%d", state.TotalQuestions),
		"followUpCount", fmt.Sprintf("%d", state.FollowUpCount),
		"maxFollowUp", fmt.Sprintf("%d", state.MaxFollowUp),
		"version", fmt.Sprintf("%d", state.Version),
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
		// 注意: mutator 可能返回 current 本身（同一指针），所以先存原始 version
		expectedVersion := current.Version
		next.Version = expectedVersion + 1

		ok, err := c.casUpdate(ctx, key, expectedVersion, next)
		if err != nil {
			return nil, err
		}
		if ok {
			return next, nil
		}
		// CAS 失败（版本冲突），退避后重试
		logrus.Debugf("[flow_cache] CAS failed (attempt %d), session=%s, expected_version=%d", attempt+1, sessionID, current.Version)
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}

	// 重试用尽，报错——调用方必须知道 CAS 失败了
	return nil, fmt.Errorf("flow CAS failed after %d retries, session=%s", flowCASMaxRetries, sessionID)
}

// casUpdate 用 Lua 脚本做 CAS 更新
func (c *FlowCache) casUpdate(ctx context.Context, key string, expectedVersion int, state *models.InterviewFlowState) (bool, error) {
	args := []interface{}{
		fmt.Sprintf("%d", expectedVersion),
		string(state.Status),
		fmt.Sprintf("%d", state.CurrentIndex),
		state.CurrentQuestionNumber,
		fmt.Sprintf("%d", state.TotalQuestions),
		fmt.Sprintf("%d", state.FollowUpCount),
		fmt.Sprintf("%d", state.MaxFollowUp),
		fmt.Sprintf("%d", state.Version),
		fmt.Sprintf("%d", cacheTTLHours*3600),
	}
	result, err := redis.NewScript(flowCASUpdateScript).Run(ctx, c.rdb, []string{key}, args...).Result()
	if err != nil {
		return false, err
	}
	r, ok := result.(int64)
	return ok && r == 1, nil
}

// parseFlowState 从 Redis Hash map 解析 InterviewFlowState
func parseFlowState(m map[string]string) (*models.InterviewFlowState, error) {
	currentIndex, _ := strconv.Atoi(m["currentIndex"])
	totalQuestions, _ := strconv.Atoi(m["totalQuestions"])
	followUpCount, _ := strconv.Atoi(m["followUpCount"])
	maxFollowUp, _ := strconv.Atoi(m["maxFollowUp"])
	version, _ := strconv.Atoi(m["version"])

	return &models.InterviewFlowState{
		Status:                models.InterviewFlowStatus(m["status"]),
		CurrentIndex:          currentIndex,
		CurrentQuestionNumber: m["currentQuestionNumber"],
		TotalQuestions:        totalQuestions,
		FollowUpCount:         followUpCount,
		MaxFollowUp:           maxFollowUp,
		Version:               version,
	}, nil
}
