//go:build integration

package singleflight

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

// 真实 Redis 端到端验证: 并发同 key 只执行一次、结果一致、资源清理干净
//
// 运行方式:
//
//	SF_TEST_REDIS_ADDR=localhost:6379 SF_TEST_REDIS_PASSWORD=123456 \
//	  go test -tags=integration -count=1 -run TestDo_RealRedis -v ./pkg/singleflight/
func TestDo_RealRedis_Distributed(t *testing.T) {
	addr := os.Getenv("SF_TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("SF_TEST_REDIS_ADDR must be set for integration test")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("SF_TEST_REDIS_PASSWORD")})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("cannot reach real redis at %s: %v", addr, err)
	}

	const key = "it-key"
	for _, k := range []string{"sf:lock:" + key, "sf:result:" + key, "sf:progress:" + key, "sf:cancel:" + key} {
		_ = rdb.Del(ctx, k)
	}

	g := NewDistributedGroup(rdb)
	var calls int32
	fn := func(ctx context.Context, w *StreamWriter) (interface{}, error) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte("c1"))
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("c2"))
		return "real-redis-ok", nil
	}

	const n = 8
	results := make([]interface{}, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = g.Do(ctx, key, fn)
		}(i)
	}
	wg.Wait()

	// 1. 结果一致且无错误
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("Do[%d] error: %v", i, errs[i])
		}
		if results[i] != "real-redis-ok" {
			t.Errorf("Do[%d] result = %v, want real-redis-ok", i, results[i])
		}
	}

	// 2. 8 个并发请求, fn 只执行 1 次(分布式去重生效)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("fn executed %d times, want 1", got)
	}

	// 3. 资源清理: 锁已释放、进度已删、结果缓存保留且带 TTL
	if n, _ := rdb.Exists(ctx, "sf:lock:"+key).Result(); n != 0 {
		t.Errorf("lock not cleaned up, exists=%d", n)
	}
	if n, _ := rdb.Exists(ctx, "sf:progress:"+key).Result(); n != 0 {
		t.Errorf("progress not cleaned up, exists=%d", n)
	}
	if n, _ := rdb.Exists(ctx, "sf:result:"+key).Result(); n != 1 {
		t.Errorf("result key missing, exists=%d", n)
	}
	if ttl, _ := rdb.TTL(ctx, "sf:result:"+key).Result(); ttl <= 0 || ttl > ResultTTL {
		t.Errorf("result TTL = %v, want (0, %v]", ttl, ResultTTL)
	}
}
