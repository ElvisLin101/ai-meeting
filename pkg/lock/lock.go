package lock

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// ============================================================
// 通用分布式锁（基于 Redis SetNX + Lua 释放）
// 用于题级锁、幂等锁等"互斥/拒绝"语义场景
// 与 singleflight 的"去重"语义不同: 拿不到锁的请求直接失败, 不复用结果
// ============================================================

// releaseScript 释放锁: 只删自己的锁(检查 nodeID 防误删)
const releaseScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`

// Lock 分布式锁实例
type Lock struct {
	rdb    *redis.Client
	key    string
	nodeID string
}

// Acquire 尝试获取锁。成功返回 *Lock, 拿不到(已被占用)返回 nil, 出错返回 error
func Acquire(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) (*Lock, error) {
	nodeID := uuid.New().String()
	ok, err := rdb.SetNX(ctx, key, nodeID, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil // 被占用
	}
	return &Lock{rdb: rdb, key: key, nodeID: nodeID}, nil
}

// Release 释放锁（只删自己的, Lua 原子）
func (l *Lock) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	return redis.NewScript(releaseScript).Run(ctx, l.rdb, []string{l.key}, l.nodeID).Err()
}
