package singleflight

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// ============================================================
// checkLeader 停滞/换主判定逻辑测试
//   返回 0                      = 主节点正常, 继续等待
//   followerGotResult          = 主节点已完成(兜底读到结果)
//   followerLeaderTimeout      = 主节点卡死/超时, 需换主
// ============================================================

func TestCheckLeader_Healthy(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()
	lockKey, resultKey, progressKey, cancelKey := "L", "R", "P", "C"

	g.redis.Set(ctx, lockKey, "node-1", LockTTL)
	g.redis.Set(ctx, progressKey, fmt.Sprintf("10:%d", now), LockTTL*2)

	last := ""
	// 首次检查: 跳过初始化周期
	if reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, "CCHAN", &last); reason != 0 {
		t.Fatalf("first check reason = %d, want 0", reason)
	}
	// 进度有推进 → 健康
	g.redis.Set(ctx, progressKey, fmt.Sprintf("20:%d", now+1), LockTTL*2)
	if reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, "CCHAN", &last); reason != 0 {
		t.Fatalf("advancing leader reason = %d, want 0", reason)
	}
	// 进度停滞但在阈值内 → 健康
	if reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, "CCHAN", &last); reason != 0 {
		t.Fatalf("stable-within-threshold reason = %d, want 0", reason)
	}
	if cancelKeyVal, _ := g.redis.Get(ctx, cancelKey).Result(); cancelKeyVal != "" {
		t.Errorf("cancel key should be empty, got %q", cancelKeyVal)
	}
}

func TestCheckLeader_StalledTakeover(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx := context.Background()
	lockKey, resultKey, progressKey, cancelKey := "L", "R", "P", "C"

	// 主节点锁还在, 但输出已停滞超过 StallThreshold
	staleTS := time.Now().Add(-StallThreshold - time.Second).UnixMilli()
	staleProgress := fmt.Sprintf("10:%d", staleTS)
	g.redis.Set(ctx, lockKey, "node-1", LockTTL)
	g.redis.Set(ctx, progressKey, staleProgress, LockTTL*2)

	last := staleProgress // 和上次读到的进度一致 → 无新输出
	reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, "CCHAN", &last)
	if reason != followerLeaderTimeout {
		t.Fatalf("stalled leader reason = %d, want followerLeaderTimeout(%d)", reason, followerLeaderTimeout)
	}
	// 换主前应写入精确取消标记(目标 = 旧主 nodeID)
	if cancelVal, _ := g.redis.Get(ctx, cancelKey).Result(); cancelVal != "node-1" {
		t.Errorf("cancel key = %q, want node-1 (精确取消旧主)", cancelVal)
	}
}

func TestCheckLeader_LockExpiredWithResult(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx := context.Background()
	lockKey, resultKey, progressKey, cancelKey := "L", "R", "P", "C"

	// 锁消失但结果已写入(Pub 消息丢失的兜底场景) → 直接拿结果
	respBytes, _ := json.Marshal(callResult{Val: "final"})
	g.redis.Set(ctx, resultKey, respBytes, ResultTTL)

	last := ""
	reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, "CCHAN", &last)
	if reason != followerGotResult {
		t.Fatalf("reason = %d, want followerGotResult(%d)", reason, followerGotResult)
	}
}

func TestCheckLeader_LockExpiredNoResult(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx := context.Background()
	lockKey, resultKey, progressKey, cancelKey := "L", "R", "P", "C"

	// 锁没了结果也没有 → 主节点超时, 准备换主
	reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, "CCHAN", nil)
	if reason != followerLeaderTimeout {
		t.Fatalf("reason = %d, want followerLeaderTimeout(%d)", reason, followerLeaderTimeout)
	}
	// 拿不到旧主 nodeID, 写空串取消标记
	if cancelVal, _ := g.redis.Get(ctx, cancelKey).Result(); cancelVal != "" {
		t.Errorf("cancel key = %q, want empty", cancelVal)
	}
}

func TestCheckLeader_ProgressLost(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx := context.Background()
	lockKey, resultKey, progressKey, cancelKey := "L", "R", "P", "C"

	g.redis.Set(ctx, lockKey, "node-1", LockTTL)

	last := ""
	// 首次检查: progressKey 不存在 → 跳过一周期给 leader 启动时间
	if reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, "CCHAN", &last); reason != 0 {
		t.Fatalf("first check reason = %d, want 0", reason)
	}
	// 后续 progressKey 仍不存在 → 换主
	reason := g.checkLeader(ctx, lockKey, resultKey, progressKey, cancelKey, "CCHAN", &last)
	if reason != followerLeaderTimeout {
		t.Fatalf("reason = %d, want followerLeaderTimeout(%d)", reason, followerLeaderTimeout)
	}
	if cancelVal, _ := g.redis.Get(ctx, cancelKey).Result(); cancelVal != "node-1" {
		t.Errorf("cancel key = %q, want node-1", cancelVal)
	}
}
