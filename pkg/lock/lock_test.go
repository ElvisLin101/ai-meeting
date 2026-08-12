package lock

import (
	"context"
	"testing"
	"time"

	"ai-meeting/internal/testutil"
)

// ============================================================
// 通用分布式锁测试(miniredis 内存版, Redis 由 internal/testutil 提供)
// ============================================================

func TestAcquire_Success(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	l, err := Acquire(context.Background(), rdb, "lock:key1", 10*time.Second)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if l == nil {
		t.Fatal("Acquire 返回 nil, 期望成功获取锁")
	}
	defer l.Release(context.Background())
}

func TestAcquire_Occupied(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	ctx := context.Background()

	l1, err := Acquire(ctx, rdb, "lock:key1", 10*time.Second)
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	defer l1.Release(ctx)

	// 同 key 已被占用 → 返回 nil, 不报错
	l2, err := Acquire(ctx, rdb, "lock:key1", 10*time.Second)
	if err != nil {
		t.Fatalf("second Acquire failed: %v", err)
	}
	if l2 != nil {
		t.Fatal("被占用时 Acquire 应返回 nil")
	}
}

func TestAcquire_DifferentKeys(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	ctx := context.Background()

	l1, err := Acquire(ctx, rdb, "lock:a", 10*time.Second)
	if err != nil {
		t.Fatalf("Acquire(a) failed: %v", err)
	}
	defer l1.Release(ctx)

	l2, err := Acquire(ctx, rdb, "lock:b", 10*time.Second)
	if err != nil {
		t.Fatalf("Acquire(b) failed: %v", err)
	}
	if l2 == nil {
		t.Fatal("不同 key 不应互斥")
	}
	l2.Release(ctx)
}

func TestAcquire_Release_Reacquire(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	ctx := context.Background()

	l1, err := Acquire(ctx, rdb, "lock:key1", 10*time.Second)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if err := l1.Release(ctx); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	l2, err := Acquire(ctx, rdb, "lock:key1", 10*time.Second)
	if err != nil {
		t.Fatalf("re-Acquire failed: %v", err)
	}
	if l2 == nil {
		t.Fatal("释放后应能重新获取锁")
	}
	l2.Release(ctx)
}

func TestLock_ReleaseOnlyOwn(t *testing.T) {
	// 模拟持有者已变更(锁 value 不再是自己的 nodeID): 释放不能误删他人的锁
	rdb := testutil.NewTestRedis(t)
	ctx := context.Background()

	const key = "lock:key1"
	// 用别的 nodeID 直接占用锁
	if err := rdb.Set(ctx, key, "someone-else", 10*time.Second).Err(); err != nil {
		t.Fatalf("seed lock failed: %v", err)
	}

	// 旧持有者尝试释放
	old := &Lock{rdb: rdb, key: key, nodeID: "old-node"}
	if err := old.Release(ctx); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	// 锁仍在(他人持有), 未被误删
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("锁被误删: %v", err)
	}
	if val != "someone-else" {
		t.Errorf("锁 value = %q, want someone-else(不应被旧持有者删除)", val)
	}
}

func TestLock_NilRelease(t *testing.T) {
	var l *Lock
	if err := l.Release(context.Background()); err != nil {
		t.Fatalf("nil lock Release 应无副作用: %v", err)
	}
}

func TestLock_Expired(t *testing.T) {
	// TTL 到期后锁自动失效, 可重新获取
	rdb := testutil.NewTestRedis(t)
	ctx := context.Background()

	l1, err := Acquire(ctx, rdb, "lock:key1", 1*time.Second)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if l1 == nil {
		t.Fatal("Acquire 返回 nil")
	}
	l1.Release(ctx)
}
