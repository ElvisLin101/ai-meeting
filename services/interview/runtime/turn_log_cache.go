package runtime

import (
	"ai-meeting/models"
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
)

// ============================================================
// TurnLogCache 面试轮次日志的 Redis List 读写
// key: interview:turns:session:{sid}, RPUSH 追加, 保留最近 200 条
// 幂等: turnRequestKey Set 记 requestId, 重复不 push
// ============================================================

const maxTurnLogs = 200

// TurnLogCache 轮次日志缓存
type TurnLogCache struct {
	rdb *redis.Client
}

// NewTurnLogCache 创建 TurnLogCache
func NewTurnLogCache(rdb *redis.Client) *TurnLogCache {
	return &TurnLogCache{rdb: rdb}
}

// AppendTurnIfAbsent 追加一轮日志（requestId 幂等），返回是否实际写入
func (c *TurnLogCache) AppendTurnIfAbsent(ctx context.Context, sessionID string, turn *models.InterviewTurnLog) (bool, error) {
	reqKey := turnRequestKey(sessionID)

	// 幂等检查: requestId 已存在则跳过
	added, err := c.rdb.SAdd(ctx, reqKey, turn.RequestID).Result()
	if err != nil {
		return false, err
	}
	if added == 0 {
		return false, nil // 已存在
	}
	c.rdb.Expire(ctx, reqKey, cacheTTLHours*time.Hour)

	payload, err := json.Marshal(turn)
	if err != nil {
		return false, err
	}

	key := turnsKey(sessionID)
	pipe := c.rdb.Pipeline()
	pipe.RPush(ctx, key, payload)
	pipe.LTrim(ctx, key, -maxTurnLogs, -1) // 保留最近 200 条
	pipe.Expire(ctx, key, cacheTTLHours*time.Hour)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetTurns 读取全部轮次日志
func (c *TurnLogCache) GetTurns(ctx context.Context, sessionID string) ([]models.InterviewTurnLog, error) {
	key := turnsKey(sessionID)
	results, err := c.rdb.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	turns := make([]models.InterviewTurnLog, 0, len(results))
	for _, raw := range results {
		var turn models.InterviewTurnLog
		if err := json.Unmarshal([]byte(raw), &turn); err != nil {
			continue
		}
		turns = append(turns, turn)
	}
	return turns, nil
}
