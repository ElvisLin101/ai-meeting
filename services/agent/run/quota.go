package run

import (
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"

	"ai-meeting/pkg/ecode"
	"ai-meeting/repositories"
)

// ============================================================
// Agent 配额
//
// 并发 run 用 Redis 计数 + TTL 兜底；每日 token 用量用 Redis 计数。
// Redis 不可用时降级放行，不阻断主链路。
// ============================================================

const quotaTTL = 48 * time.Hour

var acquireSlotScript = redis.NewScript(`
	local current = tonumber(redis.call("GET", KEYS[1]) or "0")
	if current >= tonumber(ARGV[1]) then
		return 0
	end
	local next = redis.call("INCR", KEYS[1])
	if next == 1 then
		redis.call("EXPIRE", KEYS[1], ARGV[2])
	end
	return 1
`)

var releaseSlotScript = redis.NewScript(`
	local current = tonumber(redis.call("GET", KEYS[1]) or "0")
	if current <= 0 then
		return 0
	end
	return redis.call("DECR", KEYS[1])
`)

func (s *Service) runWithQuota(ctx context.Context, user string, fn func() (*StartResult, error)) (*StartResult, error) {
	release, err := acquireRunSlot(ctx, user, s.quota.MaxConcurrentRuns)
	if err != nil {
		return nil, err
	}
	defer release()

	if err := checkDailyTokenQuota(ctx, user, s.quota.DailyTokenLimit); err != nil {
		return nil, err
	}

	res, runErr := fn()
	if res != nil {
		addDailyTokens(user, res.Tokens)
	}
	return res, runErr
}

func acquireRunSlot(ctx context.Context, user string, max int) (func(), error) {
	noop := func() {}
	if max <= 0 || user == "" || repositories.RedisClient == nil {
		return noop, nil
	}
	key := "agent:quota:" + user + ":concurrent"
	ok, err := acquireSlotScript.Run(ctx, repositories.RedisClient, []string{key}, max, int(10*time.Minute/time.Second)).Int()
	if err != nil {
		// Redis 故障降级放行，避免配额系统把主链路拖死。
		logrus.Warnf("[agent-quota] acquire slot failed, allow request: %v", err)
		return noop, nil
	}
	if ok == 0 {
		return nil, ecode.New(ecode.ErrAgentQuotaExceeded, "并发任务数已达上限，请稍后重试")
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := releaseSlotScript.Run(releaseCtx, repositories.RedisClient, []string{key}).Err(); err != nil {
			logrus.Warnf("[agent-quota] release slot failed: %v", err)
		}
	}
	return release, nil
}

func checkDailyTokenQuota(ctx context.Context, user string, limit int) error {
	if limit <= 0 || user == "" || repositories.RedisClient == nil {
		return nil
	}
	key := dailyTokenKey(user)
	raw, err := repositories.RedisClient.Get(ctx, key).Result()
	if err != nil && err != redis.Nil {
		logrus.Warnf("[agent-quota] read daily tokens failed, allow request: %v", err)
		return nil
	}
	used, _ := strconv.ParseInt(raw, 10, 64)
	if used >= int64(limit) {
		return ecode.New(ecode.ErrAgentQuotaExceeded, "今日 token 用量已达上限")
	}
	return nil
}

func addDailyTokens(user string, tokens int) {
	if tokens <= 0 || user == "" || repositories.RedisClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := dailyTokenKey(user)
	pipe := repositories.RedisClient.Pipeline()
	pipe.IncrBy(ctx, key, int64(tokens))
	pipe.Expire(ctx, key, quotaTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		logrus.Warnf("[agent-quota] add daily tokens failed: %v", err)
	}
}

func dailyTokenKey(user string) string {
	return "agent:quota:" + user + ":tokens:" + time.Now().Format("20060102")
}
