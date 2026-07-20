package singleflight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// ============================================================
// 分布式 Singleflight（支持 AI 流式输出心跳检测）
//
// 场景：AI 接口是流式返回的（SSE），但最终封装成 JSON 一次性返回给用户。
// 利用 AI 的流式输出作为天然心跳——主节点每收到一段输出就写入 Redis，
// 从节点定期检查 Redis 内容是否变化：
//   - 有变化 → 主节点在干活，继续等
//   - 无变化 → 主节点卡死了，换主
//   - 收到 done → 主节点完成，读最终结果
// ============================================================

const (
	LockTTL         = 30 * time.Second  // 锁过期时间
	Heartbeat       = 10 * time.Second  // 心跳续期间隔
	MaxExecTime     = 120 * time.Second // 主节点最大执行时间
	ResultTTL       = 5 * time.Minute   // 结果缓存时间
	StallThreshold  = 30 * time.Second  // 输出停滞阈值：超过这么久没新内容则认为卡死
	FollowerPoll    = 10 * time.Second  // 从节点检查间隔
)

// StreamWriter 主节点用它把 AI 流式输出进度写入 Redis 作为心跳。
// 只刷新进度时间戳,不写流内容——follower 仅靠 progressKey 判停滞,不消费流数据。
type StreamWriter struct {
	redis       *redis.Client
	ctx         context.Context
	progressKey string // sf:progress:{key}，存累计字节数 + 当前时间戳
	totalBytes  int
}

// Write 每收到一段 AI 输出就调用,刷新 progressKey 作为心跳。
// redis 为 nil 时(本地降级路径的 dummyWriter)直接 no-op,避免 panic。
func (w *StreamWriter) Write(chunk []byte) (int, error) {
	if w.redis == nil {
		return len(chunk), nil
	}

	// 更新进度:累计字节数 + 当前时间戳,从节点用这个判断主节点是否卡死
	w.totalBytes += len(chunk)
	progress := fmt.Sprintf("%d:%d", w.totalBytes, time.Now().UnixMilli())
	w.redis.Set(w.ctx, w.progressKey, progress, LockTTL*2)

	return len(chunk), nil
}

// callResult 主节点执行完成后写入 Redis 的最终结果
type callResult struct {
	Val interface{} `json:"val"`
	Err string      `json:"err,omitempty"`
}

// DistributedGroup 分布式 singleflight
type DistributedGroup struct {
	redis     *redis.Client
	local     *localGroup
	lockTTL   time.Duration
	heartbeat time.Duration
	maxExec   time.Duration
	resultTTL time.Duration
}

func NewDistributedGroup(rdb *redis.Client) *DistributedGroup {
	return &DistributedGroup{
		redis:     rdb,
		local:     &localGroup{},
		lockTTL:   LockTTL,
		heartbeat: Heartbeat,
		maxExec:   MaxExecTime,
		resultTTL: ResultTTL,
	}
}

// StreamFn 主节点执行的函数签名
// ctx: 带超时的 context
// writer: 用于写入 AI 流式输出（每段输出调 writer.Write(chunk)）
// 返回最终封装的 JSON 结果
type StreamFn func(ctx context.Context, writer *StreamWriter) (interface{}, error)

// Do 相同 key 的请求只执行一次 fn，其余等待复用结果
func (g *DistributedGroup) Do(ctx context.Context, key string, fn StreamFn) (interface{}, error) {
	for {
		// 1. Redis 不可用 → 降级为本地 singleflight
		if !g.redisAvailable(ctx) {
			fmt.Printf("[singleflight] Redis 不可用，降级为本地去重 key=%s\n", key)
			return g.local.Do(key, func() (interface{}, error) {
				dummyWriter := &StreamWriter{}
				return fn(ctx, dummyWriter)
			})
		}

		nodeID := uuid.New().String()
		lockKey := "sf:lock:" + key
		resultKey := "sf:result:" + key
		channel := "sf:channel:" + key
		progressKey := "sf:progress:" + key
		cancelKey := "sf:cancel:" + key // 换主时写入，通知旧主停止

		// 2. 尝试成为主节点
		acquired, err := g.tryAcquireLock(ctx, lockKey, nodeID)
		if err != nil {
			fmt.Printf("[singleflight] Redis 操作失败，降级为本地去重 key=%s err=%v\n", key, err)
			return g.local.Do(key, func() (interface{}, error) {
				dummyWriter := &StreamWriter{}
				return fn(ctx, dummyWriter)
			})
		}

		if acquired {
			// 3. 我是主节点 → 执行 fn
			return g.runAsLeader(ctx, key, lockKey, resultKey, channel, progressKey, cancelKey, nodeID, fn)
		}

		// 4. 我是从节点 → 等待结果
		result, reason, err := g.runAsFollower(ctx, lockKey, resultKey, channel, progressKey, cancelKey)
		if err != nil {
			return nil, err
		}

		switch reason {
		case followerGotResult:
			return result, nil
		case followerLeaderTimeout:
			continue // 主节点卡死/超时，重新抢锁换主
		case followerCtxDone:
			return nil, ctx.Err()
		}
	}
}

// ============================================================
// 主节点逻辑
// ============================================================

func (g *DistributedGroup) runAsLeader(
	ctx context.Context, key, lockKey, resultKey, channel, progressKey, cancelKey, nodeID string,
	fn StreamFn,
) (interface{}, error) {
	execCtx, cancel := context.WithTimeout(ctx, g.maxExec)
	defer cancel()

	// 心跳续期协程
	hbCtx, hbCancel := context.WithCancel(context.Background())
	defer hbCancel()
	go g.heartbeatLoop(hbCtx, lockKey, nodeID)

	// 取消监听协程：被换主时 cancel execCtx，DeepSeek SDK 自动断开
	go g.watchCancel(execCtx, cancelKey, cancel, key, nodeID)

	// 清理上一次可能残留的进度数据和取消标记（换主场景）
	g.redis.Del(execCtx, progressKey, cancelKey)

	// 创建流式写入器，主节点每收到一段 AI 输出就刷新 progressKey 作心跳
	writer := &StreamWriter{
		redis:       g.redis,
		ctx:         execCtx,
		progressKey: progressKey,
	}

	fmt.Printf("[singleflight] 主节点开始执行 key=%s nodeID=%s\n", key, nodeID)

	// 执行 fn（AI 流式调用，fn 内部会不断调 writer.Write）
	result, err := fn(execCtx, writer)

	// 写入最终结果
	cr := callResult{Val: result}
	if err != nil {
		cr.Err = err.Error()
	}
	resultBytes, _ := json.Marshal(cr)

	pipe := g.redis.Pipeline()
	pipe.Set(execCtx, resultKey, resultBytes, g.resultTTL)
	if err != nil {
		pipe.Publish(execCtx, channel, "error:"+err.Error())
	} else {
		pipe.Publish(execCtx, channel, "done")
	}
	pipe.Exec(execCtx)

	// 清理进度数据
	g.redis.Del(context.Background(), progressKey)

	// 释放锁
	g.releaseLock(context.Background(), lockKey, nodeID)

	fmt.Printf("[singleflight] 主节点执行完成 key=%s err=%v\n", key, err)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (g *DistributedGroup) heartbeatLoop(ctx context.Context, lockKey, nodeID string) {
	ticker := time.NewTicker(g.heartbeat)
	defer ticker.Stop()

	renewScript := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("expire", KEYS[1], ARGV[2])
		end
		return 0
	`)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewScript.Run(ctx, g.redis, []string{lockKey}, nodeID, int(g.lockTTL.Seconds()))
		}
	}
}

// watchCancel 监听取消标记，被换主时 cancel execCtx 停止 AI 调用
// 只有 cancelKey 的 value 等于自己的 nodeID 才取消——防止新主误读旧主的取消信号
// DeepSeek SDK 收到 ctx 取消会自动断开 SSE 连接，不浪费 token
func (g *DistributedGroup) watchCancel(ctx context.Context, cancelKey string, cancel context.CancelFunc, key, nodeID string) {
	ticker := time.NewTicker(5 * time.Second) // 每 5s 检查一次取消标记
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// execCtx 结束（正常完成或超时），停止监听
			return
		case <-ticker.C:
			val, err := g.redis.Get(ctx, cancelKey).Result()
			if err != nil {
				continue // key 不存在或 Redis 出错
			}
			if val == nodeID {
				fmt.Printf("[singleflight] 主节点收到取消信号，停止 AI 调用 key=%s\n", key)
				cancel() // 取消 execCtx，DeepSeek SDK 自动断开
				return
			}
			// value 不等于自己的 nodeID → 这是给别的主的取消信号，忽略
		}
	}
}

// ============================================================
// 从节点逻辑
// ============================================================

type followerReason int

const (
	followerGotResult     followerReason = iota // 主节点完成，拿到结果
	followerLeaderTimeout                       // 主节点卡死/超时，需要换主
	followerCtxDone                             // 自己超时/取消
)

func (g *DistributedGroup) runAsFollower(ctx context.Context, lockKey, resultKey, channel, progressKey, cancelKey string) (interface{}, followerReason, error) {
	pubsub := g.redis.Subscribe(ctx, channel)
	defer pubsub.Close()
	msgCh := pubsub.Channel()

	ticker := time.NewTicker(FollowerPoll)
	defer ticker.Stop()

	// 记录上次检查到的进度，用于判断主节点输出是否在变化
	var lastProgress string

	for {
		select {
		case <-ctx.Done():
			return nil, followerCtxDone, ctx.Err()

		case msg := <-msgCh:
			if msg.Payload == "done" {
				result, err := g.readResult(ctx, resultKey)
				if err != nil {
					return nil, followerLeaderTimeout, nil
				}
				return result, followerGotResult, nil
			}
			if len(msg.Payload) > 6 && msg.Payload[:6] == "error:" {
				return nil, followerGotResult, errors.New(msg.Payload[6:])
			}

		case <-ticker.C:
			// 检查主节点状态
			reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, &lastProgress)
			if reason != 0 { // 0 表示主节点正常，继续等
				return nil, reason, nil
			}
		}
	}
}

// checkLeader 检查主节点是否健康
// 返回 0 = 主节点正常，继续等待
// 返回 followerGotResult = 主节点已完成（兜底读到结果）
// 返回 followerLeaderTimeout = 主节点卡死/超时
func (g *DistributedGroup) checkLeader(ctx context.Context, lockKey, resultKey, progressKey, cancelKey string, lastProgress *string) followerReason {
	// 1. 检查锁是否还在
	lockExists, err := g.redis.Exists(ctx, lockKey).Result()
	if err != nil {
		return 0 // Redis 出错，不急着换主，继续等
	}
	if lockExists == 0 {
		// 锁不在了，检查结果是否已写入（主节点可能完成了但 Pub 消息丢了）
		result, rerr := g.readResult(ctx, resultKey)
		if rerr == nil {
			_ = result
			return followerGotResult
		}
		// 锁没了结果也没有，主节点超时了
		// 此时拿不到旧主 nodeID，写空串——旧主的 execCtx 也会因超时退出
		g.notifyCancel(ctx, cancelKey, "")
		fmt.Printf("[singleflight] 主节点锁已消失，写入取消标记，准备换主\n")
		return followerLeaderTimeout
	}

	// 从锁的 value 读取旧主 nodeID（用于精确取消）
	leaderNodeID, _ := g.redis.Get(ctx, lockKey).Result()

	// 2. 检查 AI 输出是否还在变化（核心：利用流式输出做心跳）
	currentProgress, err := g.redis.Get(ctx, progressKey).Result()
	if err != nil {
		if *lastProgress == "" {
			*lastProgress = "init" // 第一次检查，跳过一个周期给 leader 启动时间
			return 0
		}
		// 后续检查 progressKey 仍不存在 → leader 长时间无输出，换主
		g.notifyCancel(ctx, cancelKey, leaderNodeID)
		fmt.Printf("[singleflight] 主节点进度信息丢失，写入取消标记，准备换主\n")
		return followerLeaderTimeout
	}

	if currentProgress != *lastProgress {
		// 进度有变化，主节点在干活
		*lastProgress = currentProgress
		return 0
	}

	// 进度没变化，检查是否超过停滞阈值
	parts := splitProgress(currentProgress)
	if len(parts) != 2 {
		return 0
	}
	lastUpdateMs := parts[1]
	stalledDuration := time.Since(time.UnixMilli(lastUpdateMs))
	if stalledDuration > StallThreshold {
		g.notifyCancel(ctx, cancelKey, leaderNodeID)
		fmt.Printf("[singleflight] 主节点输出停滞 %v，超过阈值 %v，写入取消标记，准备换主\n",
			stalledDuration, StallThreshold)
		return followerLeaderTimeout
	}

	return 0
}

// notifyCancel 写入取消标记，通知旧主节点停止 AI 调用
// targetNodeID 为旧主的 nodeID（锁的 value），watchCancel 只在 value 匹配自己时才 cancel
func (g *DistributedGroup) notifyCancel(ctx context.Context, cancelKey, targetNodeID string) {
	g.redis.Set(ctx, cancelKey, targetNodeID, MaxExecTime)
}

// splitProgress 解析 "totalBytes:timestampMs" 格式
func splitProgress(p string) [2]int64 {
	var parts [2]int64
	var idx int
	var num int64
	for _, c := range p {
		if c == ':' {
			parts[idx] = num
			idx++
			num = 0
			if idx >= 2 {
				break
			}
		} else if c >= '0' && c <= '9' {
			num = num*10 + int64(c-'0')
		}
	}
	if idx < 2 {
		parts[idx] = num
	}
	return parts
}

func (g *DistributedGroup) readResult(ctx context.Context, resultKey string) (interface{}, error) {
	resultBytes, err := g.redis.Get(ctx, resultKey).Bytes()
	if err != nil {
		return nil, fmt.Errorf("读取结果失败: %w", err)
	}
	var cr callResult
	if err := json.Unmarshal(resultBytes, &cr); err != nil {
		return nil, fmt.Errorf("反序列化结果失败: %w", err)
	}
	if cr.Err != "" {
		return nil, errors.New(cr.Err)
	}
	return cr.Val, nil
}

// ============================================================
// Redis 操作封装
// ============================================================

func (g *DistributedGroup) redisAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return g.redis.Ping(ctx).Err() == nil
}

func (g *DistributedGroup) tryAcquireLock(ctx context.Context, lockKey, nodeID string) (bool, error) {
	ok, err := g.redis.SetNX(ctx, lockKey, nodeID, g.lockTTL).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (g *DistributedGroup) releaseLock(ctx context.Context, lockKey, nodeID string) {
	script := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		end
		return 0
	`)
	script.Run(ctx, g.redis, []string{lockKey}, nodeID)
}

// ============================================================
// 本地降级 singleflight
// ============================================================

type localCall struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

type localGroup struct {
	mu sync.Mutex
	m  map[string]*localCall
}

func (g *localGroup) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*localCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := &localCall{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}
