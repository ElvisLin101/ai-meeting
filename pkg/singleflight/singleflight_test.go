package singleflight

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// newTestGroup 启动内存 Redis 并构造 DistributedGroup
func newTestGroup(t *testing.T) (*DistributedGroup, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewDistributedGroup(rdb), mr
}

// ============================================================
// 本地降级 singleflight
// ============================================================

func TestLocalGroup_Do_Deduplicates(t *testing.T) {
	var calls int32
	fn := func() (interface{}, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return "value", nil
	}

	g := &localGroup{}
	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	results := make([]interface{}, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = g.Do("key", fn)
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("fn executed %d times, want 1 (去重失败)", got)
	}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("Do[%d] error: %v", i, errs[i])
		}
		if results[i] != "value" {
			t.Errorf("Do[%d] result = %v, want value", i, results[i])
		}
	}
}

func TestLocalGroup_Do_ErrorAndReExecute(t *testing.T) {
	g := &localGroup{}
	var calls int32

	failFn := func() (interface{}, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New("boom")
	}
	if _, err := g.Do("key", failFn); err == nil {
		t.Fatal("expected error, got nil")
	}

	// 失败后 map 已清理, 再次调用会重新执行
	val, err := g.Do("key", func() (interface{}, error) { return "ok", nil })
	if err != nil || val != "ok" {
		t.Errorf("second Do = (%v, %v), want (ok, nil)", val, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("failFn executed %d times, want 1", got)
	}
}

// ============================================================
// 工具函数
// ============================================================

func TestSplitProgress(t *testing.T) {
	tests := []struct {
		in   string
		want [2]int64
	}{
		{"100:1234", [2]int64{100, 1234}},
		{"0:0", [2]int64{0, 0}},
		{"100", [2]int64{100, 0}},           // 无冒号: 只解析出字节数
		{"100:200:300", [2]int64{100, 200}}, // 多个冒号: 取前两段
		{"", [2]int64{0, 0}},
		{"abc", [2]int64{0, 0}},       // 非数字
		{"100:abc", [2]int64{100, 0}}, // 时间戳段非数字
		{"12a:34", [2]int64{12, 34}},  // 数字后跟字符仍解析数字
	}
	for _, tt := range tests {
		if got := splitProgress(tt.in); got != tt.want {
			t.Errorf("splitProgress(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// ============================================================
// 主/从节点 Redis 交互
// ============================================================

func TestStreamWriter_NoRedis(t *testing.T) {
	w := &StreamWriter{} // redis 为 nil: 本地降级路径的 dummyWriter
	n, err := w.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("Write = (%d, %v), want (5, nil)", n, err)
	}
}

func TestStreamWriter_WithRedis(t *testing.T) {
	g, mr := newTestGroup(t)
	w := &StreamWriter{redis: g.redis, ctx: context.Background(), progressKey: "sf:progress:test"}

	if _, err := w.Write([]byte("ab")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if _, err := w.Write([]byte("cd")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 格式 "累计字节数:毫秒时间戳", 两次写后累计 4
	val, _ := mr.Get("sf:progress:test")
	parts := splitProgress(val)
	if parts[0] != 4 {
		t.Errorf("totalBytes = %d, want 4", parts[0])
	}
	if parts[1] <= 0 {
		t.Errorf("timestamp = %d, want > 0", parts[1])
	}
	if ttl := mr.TTL("sf:progress:test"); ttl <= 0 {
		t.Errorf("progress TTL = %v, want > 0", ttl)
	}
}

func TestReadResult(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx := context.Background()

	// 结果不存在 → 错误
	if _, err := g.readResult(ctx, "sf:result:missing"); err == nil {
		t.Error("expected error for missing result, got nil")
	}

	// 正常结果
	okBytes, _ := json.Marshal(callResult{Val: "ok-value"})
	g.redis.Set(ctx, "sf:result:ok", okBytes, ResultTTL)
	val, err := g.readResult(ctx, "sf:result:ok")
	if err != nil {
		t.Fatalf("readResult failed: %v", err)
	}
	if val != "ok-value" {
		t.Errorf("val = %v, want ok-value", val)
	}

	// 结果携带错误 → 错误透出
	errBytes, _ := json.Marshal(callResult{Err: "ai timeout"})
	g.redis.Set(ctx, "sf:result:err", errBytes, ResultTTL)
	if _, err := g.readResult(ctx, "sf:result:err"); err == nil {
		t.Error("expected result error to surface, got nil")
	}
}

func TestLock_AcquireRelease(t *testing.T) {
	g, mr := newTestGroup(t)
	ctx := context.Background()
	lockKey := "sf:lock:test"

	ok, err := g.tryAcquireLock(ctx, lockKey, "node-1")
	if err != nil || !ok {
		t.Fatalf("first acquire = (%v, %v), want (true, nil)", ok, err)
	}
	// 已被占用
	ok, err = g.tryAcquireLock(ctx, lockKey, "node-2")
	if err != nil || ok {
		t.Fatalf("second acquire = (%v, %v), want (false, nil)", ok, err)
	}
	if got, _ := mr.Get(lockKey); got != "node-1" {
		t.Errorf("lock value = %q, want node-1", got)
	}
	// 错误身份释放无效
	g.releaseLock(ctx, lockKey, "node-2")
	if got, _ := mr.Get(lockKey); got != "node-1" {
		t.Errorf("lock released by wrong node, value = %q", got)
	}
	// 正确身份释放
	g.releaseLock(ctx, lockKey, "node-1")
	if mr.Exists(lockKey) {
		t.Error("lock still exists after release")
	}
}

func TestRedisAvailable(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx := context.Background()

	if !g.redisAvailable(ctx) {
		t.Error("redisAvailable = false for running redis, want true")
	}
	// 连接关闭后应判定不可用
	_ = g.redis.Close()
	if g.redisAvailable(ctx) {
		t.Error("redisAvailable = true for closed redis, want false")
	}
}

// ============================================================
// Do 端到端
// ============================================================

// TestDo_Distributed 并发同 key: 主节点只执行一次 fn, 从节点复用结果
func TestDo_Distributed(t *testing.T) {
	g, _ := newTestGroup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var calls int32
	fn := func(ctx context.Context, w *StreamWriter) (interface{}, error) {
		atomic.AddInt32(&calls, 1)
		if _, err := w.Write([]byte("chunk-1")); err != nil {
			return nil, err
		}
		time.Sleep(300 * time.Millisecond)
		if _, err := w.Write([]byte("chunk-2")); err != nil {
			return nil, err
		}
		return "result-ok", nil
	}

	const n = 2
	results := make([]interface{}, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = g.Do(ctx, "key-1", fn)
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("Do[%d] error: %v", i, errs[i])
		}
		if results[i] != "result-ok" {
			t.Errorf("Do[%d] result = %v, want result-ok", i, results[i])
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("fn executed %d times, want 1 (分布式去重失败)", got)
	}
}

// TestDo_FallsBackToLocalWhenRedisDown Redis 不可用时降级为本地去重
func TestDo_FallsBackToLocalWhenRedisDown(t *testing.T) {
	g, mr := newTestGroup(t)
	mr.Close() // 模拟 Redis 不可用
	ctx := context.Background()

	var calls int32
	fn := func(ctx context.Context, w *StreamWriter) (interface{}, error) {
		atomic.AddInt32(&calls, 1)
		return "local-result", nil
	}

	val, err := g.Do(ctx, "key-x", fn)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if val != "local-result" {
		t.Errorf("val = %v, want local-result", val)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("fn executed %d times, want 1", got)
	}
}
