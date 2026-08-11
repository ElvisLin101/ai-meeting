package singleflight

import (
	"context"
	"testing"
	"time"
)

// ============================================================
// watchCancel 双通道取消逻辑测试
// 换主时从节点 Publish 到 cancelChan → 旧主毫秒级取消(推送为主)
// cancelKey 轮询 5s 兜底(消息丢失场景)
// ============================================================

// TestWatchCancel_PubSubPush Pub/Sub 推送取消: 换主信号毫秒级生效
func TestWatchCancel_PubSubPush(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		key        = "key-1"
		nodeID     = "leader-1"
		cancelKey  = "sf:cancel:key-1"
		cancelChan = "sf:cancelchan:key-1"
	)

	go g.watchCancel(ctx, cancelKey, cancelChan, cancel, key, nodeID)

	// 从节点换主 → 反复推送取消信号(覆盖订阅建立前的窗口, 保证送达)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			g.redis.Publish(context.Background(), cancelChan, nodeID)
			time.Sleep(50 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		// cancel 生效, watchCancel 已退出
	case <-time.After(3 * time.Second):
		t.Fatal("cancel not triggered by Pub/Sub push within 3s")
	}
}

// TestWatchCancel_IgnoreOtherNode 只响应自己的取消信号, 不误杀新主
func TestWatchCancel_IgnoreOtherNode(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		key        = "key-2"
		nodeID     = "leader-2"
		cancelKey  = "sf:cancel:key-2"
		cancelChan = "sf:cancelchan:key-2"
	)

	go g.watchCancel(ctx, cancelKey, cancelChan, cancel, key, nodeID)
	time.Sleep(100 * time.Millisecond) // 等订阅建立

	// 发给别的主节点的取消信号 → 不取消自己
	g.redis.Publish(context.Background(), cancelChan, "other-leader")
	time.Sleep(300 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("should not cancel for other leader's signal")
	}

	// 自己的信号才取消
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			g.redis.Publish(context.Background(), cancelChan, nodeID)
			time.Sleep(50 * time.Millisecond)
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel not triggered by own signal")
	}
}
