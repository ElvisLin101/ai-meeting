// Command sfprobe: SingleFlight 分布式去重并发实测工具
//
// 用途: 验证"同一 key 的 N 个并发请求只触发 1 次底层调用(如 AI)",
//       其余请求复用 leader 的结果。输出实测报告, 供面试/文档佐证。
//
// 用法:
//
//	go run ./cmd/sfprobe --addr localhost:6379 --password 123456 --concurrency 100
//
// 输出: 并发请求数 / 底层调用实际执行次数 / 去重率 / 结果一致性 / 总耗时 / 事件统计。
package main

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"ai-meeting/pkg/singleflight"

	"github.com/go-redis/redis/v8"
)

func main() {
	var (
		addr        = flag.String("addr", "localhost:6379", "Redis 地址")
		password    = flag.String("password", "", "Redis 密码")
		concurrency = flag.Int("concurrency", 100, "并发请求数(默认 100)")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: *addr, Password: *password})
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("无法连接 Redis %s: %v\n", *addr, err)
		return
	}

	key := fmt.Sprintf("sfprobe:%d", time.Now().UnixNano())
	g := singleflight.NewDistributedGroup(rdb)

	var (
		execCalls int32 // 底层调用实际执行次数(仅 leader 执行)
		events    sync.Map
	)
	g.SetMetricFunc(func(module, event string, success bool, extra string) {
		v, _ := events.LoadOrStore(event, new(int32))
		atomic.AddInt32(v.(*int32), 1)
	})

	// 模拟 AI 流式调用: 每 100ms 输出一段, 共 3 段, 单次耗时约 300ms
	fn := func(ctx context.Context, writer *singleflight.StreamWriter) (interface{}, error) {
		atomic.AddInt32(&execCalls, 1)
		for i := 0; i < 3; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
			if _, werr := writer.Write([]byte(fmt.Sprintf("chunk-%d", i))); werr != nil {
				return nil, werr
			}
		}
		return "probe-ok", nil
	}

	start := time.Now()
	results := make([]interface{}, *concurrency)
	errs := make([]error, *concurrency)
	var wg sync.WaitGroup
	wg.Add(*concurrency)
	for i := 0; i < *concurrency; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = g.Do(ctx, key, fn)
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	executed := atomic.LoadInt32(&execCalls)
	consistent, errCount := 0, 0
	for i := 0; i < *concurrency; i++ {
		if errs[i] != nil {
			errCount++
			continue
		}
		if results[i] == "probe-ok" {
			consistent++
		}
	}

	fmt.Println("===== SingleFlight 分布式去重实测 =====")
	fmt.Printf("Redis 地址:            %s\n", *addr)
	fmt.Printf("并发请求数:            %d\n", *concurrency)
	fmt.Printf("底层调用实际执行次数:  %d (期望 1)\n", executed)
	fmt.Printf("去重率:                %.1f%% (节省调用 %d 次)\n",
		float64(*concurrency-int(executed))/float64(*concurrency)*100,
		*concurrency-int(executed))
	fmt.Printf("结果一致:              %d/%d\n", consistent, *concurrency)
	fmt.Printf("请求失败:              %d\n", errCount)
	fmt.Printf("总耗时:                %v (约等于单次底层调用耗时, 证明并发被合并)\n", elapsed)

	fmt.Println("事件统计:")
	eventNames := []string{"leader_elected", "follower_waiting", "leader_completed", "leader_timeout"}
	for _, name := range eventNames {
		if v, ok := events.Load(name); ok {
			fmt.Printf("  %-20s %d\n", name, atomic.LoadInt32(v.(*int32)))
		}
	}

	// 清理探针 key
	rdb.Del(context.Background(),
		"sf:lock:"+key, "sf:result:"+key, "sf:progress:"+key, "sf:cancel:"+key,
		"sf:channel:"+key, "sf:cancelchan:"+key)
}
