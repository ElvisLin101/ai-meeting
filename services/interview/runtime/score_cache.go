package runtime

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-redis/redis/v8"
)

// ============================================================
// ScoreCache 面试分数的 Redis 读写
// 三个 key: score(平均分) / score_sum(累计和) / score_count(计分次数)
// 用 Lua 脚本原子更新: INCRBY sum + score, INCRBY count +1, SET avg=round(sum/count)
// 追问题不计分(调用方判断 isFollowUp 后决定是否调 AddScore)
// ============================================================

// scoreAggregateScript 原子更新分数聚合
// KEYS: [scoreSumKey, scoreCountKey, scoreKey]
// ARGV: [score, ttlSeconds]
const scoreAggregateScript = `
local sum = redis.call('INCRBY', KEYS[1], tonumber(ARGV[1]))
local cnt = redis.call('INCRBY', KEYS[2], 1)
local avg = math.floor((sum / cnt) + 0.5)
redis.call('SET', KEYS[3], tostring(avg))
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]))
redis.call('EXPIRE', KEYS[2], tonumber(ARGV[2]))
redis.call('EXPIRE', KEYS[3], tonumber(ARGV[2]))
return {sum, cnt, avg}
`

// 恢复分数时保留已存在的聚合，避免覆盖其他请求刚写入的成绩。
const restoreScoreAggregateScript = `
local sum = tonumber(redis.call('GET', KEYS[1]))
local cnt = tonumber(redis.call('GET', KEYS[2]))
if not sum or not cnt or cnt <= 0 then
    sum = tonumber(ARGV[1])
    cnt = tonumber(ARGV[2])
end
local ttl = tonumber(ARGV[3])
redis.call('SET', KEYS[1], sum, 'EX', ttl)
redis.call('SET', KEYS[2], cnt, 'EX', ttl)
redis.call('SET', KEYS[3], math.floor((sum / cnt) + 0.5), 'EX', ttl)
return 1
`

// ScoreCache 面试分数缓存
type ScoreCache struct {
	rdb *redis.Client
}

// NewScoreCache 创建 ScoreCache
func NewScoreCache(rdb *redis.Client) *ScoreCache {
	return &ScoreCache{rdb: rdb}
}

// AddScore 累加一次主问题得分，返回 [sum, count, avg]
func (c *ScoreCache) AddScore(ctx context.Context, sessionID string, score int) (sum, count, avg int, err error) {
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	result, err := redis.NewScript(scoreAggregateScript).Run(ctx, c.rdb,
		[]string{scoreSumKey(sessionID), scoreCountKey(sessionID), scoreKey(sessionID)},
		score, cacheTTLHours*3600,
	).Result()
	if err != nil {
		return 0, 0, 0, err
	}

	vals := result.([]interface{})
	sum = int(vals[0].(int64))
	count = int(vals[1].(int64))
	avg = int(vals[2].(int64))
	return sum, count, avg, nil
}

// readAggregate 一次读取同一计分时点的累计分和计分次数。
func (c *ScoreCache) readAggregate(ctx context.Context, sessionID string) (sum, count int, err error) {
	values, err := c.rdb.MGet(ctx, scoreSumKey(sessionID), scoreCountKey(sessionID)).Result()
	if err != nil {
		return 0, 0, err
	}
	sumText, sumOK := values[0].(string)
	countText, countOK := values[1].(string)
	if !sumOK || !countOK {
		return 0, 0, nil
	}
	sum, err = strconv.Atoi(sumText)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cached score sum: %w", err)
	}
	count, err = strconv.Atoi(countText)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cached score count: %w", err)
	}
	return sum, count, nil
}

func (c *ScoreCache) restoreAggregate(ctx context.Context, sessionID string, sum, count int) error {
	return redis.NewScript(restoreScoreAggregateScript).Run(ctx, c.rdb,
		[]string{scoreSumKey(sessionID), scoreCountKey(sessionID), scoreKey(sessionID)},
		sum, count, cacheTTLHours*3600,
	).Err()
}

// GetTotalScore 读取当前平均分
func (c *ScoreCache) GetTotalScore(ctx context.Context, sessionID string) (int, error) {
	val, err := c.rdb.Get(ctx, scoreKey(sessionID)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return val, nil
}

// ResetScore 清零分数（出题成功后调用）
func (c *ScoreCache) ResetScore(ctx context.Context, sessionID string) error {
	_, err := c.rdb.Del(ctx, scoreKey(sessionID), scoreSumKey(sessionID), scoreCountKey(sessionID)).Result()
	return err
}
