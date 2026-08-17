// Command sfcluster: SingleFlight 集群(多实例)压测工具
//
// 用途: 验证"跨实例的 N 个并发请求只触发 1 次底层调用(如 AI)"。
//
//	与 sfprobe(单进程多协程)不同, 本工具启动 N 个独立 worker 进程,
//	每个进程拥有独立的 DistributedGroup 实例(独立 nodeID/独立内存态),
//	仅共享同一个 Redis——等价于真实集群中负载均衡把请求打到不同实例。
//
// 两种角色:
//   - coordinator: 生成 runID, 等待全部 worker 注册 ready, 广播 start 信号,
//     收集并汇总各 worker 上报的统计, 输出压测报告。
//   - worker: 注册 ready 后阻塞等待 start, 收到后对同一个 key 发起并发请求,
//     底层调用次数通过 Redis INCR 统计(leader 是跨进程唯一执行点)。
//
// 故障注入(可选): 指定某个 worker 成为 leader 后 sleep 超过 StallThreshold,
//
//	期间不刷新心跳, 其余实例的 follower 检测到停滞后写 cancelKey 换主接管。
//	期望 exec=2(旧主一次 + 接管者一次), 所有请求最终拿到一致结果。
//
// 用法(建议通过 scripts/sfcluster-run.sh 一键运行):
//
//	go run ./cmd/sfcluster --role coordinator --instances 3 --concurrency 50 --run R1
//	go run ./cmd/sfcluster --role worker --id 0 --concurrency 50 --run R1 --addr localhost:6379 --password 123456
//	go run ./cmd/sfcluster --role worker --id 1 --concurrency 50 --run R1 --addr localhost:6379 --password 123456
//
// 输出: 实例数 / 每实例并发 / 底层调用实际执行次数 / 去重率 / 结果一致性 / 事件统计 / 执行实例分布。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"ai-meeting/pkg/singleflight"

	"github.com/go-redis/redis/v8"
)

const (
	// sfclusterRedisPrefix 压测协调所用 Redis key/频道统一前缀
	sfclusterRedisPrefix = "sfcluster:"
	// readyKeySuffix worker 注册计数(coordinator 等它到 instances)
	readyKeySuffix = ":ready"
	// doneKeySuffix worker 完成计数(coordinator 等它到 instances)
	doneKeySuffix = ":done"
	// execKeySuffix 底层调用实际执行次数(fn 内 INCR, 跨进程累计)
	execKeySuffix = ":exec"
	// okKeySuffix 结果一致请求数
	okKeySuffix = ":ok"
	// failKeySuffix 结果不一致/失败请求数
	failKeySuffix = ":fail"
	// startChanSuffix start 信号频道(coordinator Publish, worker 订阅)
	startChanSuffix = ":start"
	// leaderWorkersKeySuffix 哪些 worker 曾担任 leader(集合, 用于执行实例分布)
	leaderWorkersKeySuffix = ":leader_workers"

	// readyTimeout coordinator 等待全部 worker 就绪的超时
	readyTimeout = 60 * time.Second
	// doneTimeout coordinator 等待全部 worker 完成的超时(换主场景需覆盖 StallThreshold+余量)
	doneTimeout = 3 * time.Minute
	// startWaitTimeout worker 等待 start 信号的最长阻塞时间
	startWaitTimeout = 90 * time.Second
	// workerCtxTimeout 单个 worker 发起并等待全部请求返回的超时
	workerCtxTimeout = 150 * time.Second
	// failoverStall 故障注入 worker 成为 leader 后的卡死时长(> StallThreshold=30s)
	failoverStall = 40 * time.Second
)

func main() {
	var (
		role           = flag.String("role", "", "角色: coordinator | worker")
		runID          = flag.String("run", "", "压测批次 ID, coordinator 与全部 worker 必须一致")
		instances      = flag.Int("instances", 3, "实例数(仅 coordinator 用, 等待多少 worker 就绪)")
		id             = flag.Int("id", 0, "worker 实例 ID(仅 worker 用)")
		concurrency    = flag.Int("concurrency", 50, "每个实例发起的并发请求数")
		addr           = flag.String("addr", "localhost:6379", "Redis 地址")
		password       = flag.String("password", "", "Redis 密码")
		failoverWorker = flag.Int("failover-worker", -1, "故障注入: 指定 worker ID 成为 leader 后卡死触发换主(-1 关闭)")
	)
	flag.Parse()

	if *role == "" {
		fmt.Println("错误: 必须指定 --role coordinator|worker")
		flag.Usage()
		os.Exit(1)
	}
	if *runID == "" {
		fmt.Println("错误: 必须指定 --run (coordinator 与全部 worker 使用同一 runID)")
		os.Exit(1)
	}
	if *role == "coordinator" && *instances < 1 {
		fmt.Println("错误: coordinator 的 --instances 必须 >= 1")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), workerCtxTimeout)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: *addr, Password: *password})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("无法连接 Redis %s: %v\n", *addr, err)
		os.Exit(1)
	}

	switch *role {
	case "coordinator":
		runCoordinator(ctx, rdb, *runID, *instances)
	case "worker":
		runWorker(ctx, rdb, *runID, *id, *concurrency, *failoverWorker)
	default:
		fmt.Printf("未知角色 %q, 支持 coordinator | worker\n", *role)
		os.Exit(1)
	}
}

// ============================================================
// coordinator
// ============================================================

func runCoordinator(ctx context.Context, rdb *redis.Client, runID string, instances int) {
	fmt.Printf("[coordinator] run=%s 等待 %d 个实例就绪...\n", runID, instances)

	// 1. 等全部 worker 注册
	if !waitForCount(ctx, rdb, readyKey(runID), instances, readyTimeout) {
		fmt.Printf("[coordinator] 超时 %v: 仅 %d/%d 个实例就绪\n", readyTimeout, getCount(ctx, rdb, readyKey(runID)), instances)
		cleanup(ctx, rdb, runID)
		os.Exit(1)
	}
	fmt.Printf("[coordinator] %d 个实例全部就绪, 广播 start\n", instances)

	// 2. 广播 start
	if err := rdb.Publish(ctx, startChan(runID), "go").Err(); err != nil {
		fmt.Printf("[coordinator] 广播 start 失败: %v\n", err)
		cleanup(ctx, rdb, runID)
		os.Exit(1)
	}

	// 3. 等全部 worker 完成
	start := time.Now()
	if !waitForCount(ctx, rdb, doneKey(runID), instances, doneTimeout) {
		fmt.Printf("[coordinator] 超时 %v: 仅 %d/%d 个实例完成\n", doneTimeout, getCount(ctx, rdb, doneKey(runID)), instances)
		cleanup(ctx, rdb, runID)
		os.Exit(1)
	}
	elapsed := time.Since(start)

	// 4. 汇总
	executed := getCount(ctx, rdb, execKey(runID))
	okCount := getCount(ctx, rdb, okKey(runID))
	failCount := getCount(ctx, rdb, failKey(runID))
	total := okCount + failCount
	leaderWorkers := getLeaderWorkers(ctx, rdb, runID)

	// 事件计数
	events := map[string]int64{}
	for _, name := range []string{"leader_elected", "follower_waiting", "leader_completed", "leader_timeout"} {
		events[name] = int64(getCount(ctx, rdb, eventKey(runID, name)))
	}

	printReport(instances, total, executed, okCount, failCount, elapsed, events, leaderWorkers)
	cleanup(ctx, rdb, runID)
}

// printReport 输出集群压测报告(与 sfprobe 格式对齐)
func printReport(instances, total, executed, okCount, failCount int, elapsed time.Duration, events map[string]int64, leaderWorkers []int) {
	fmt.Println("===== SingleFlight 集群压测(多实例) =====")
	fmt.Printf("实例数:                %d\n", instances)
	fmt.Printf("总请求数:              %d\n", total)
	fmt.Printf("底层调用实际执行次数:  %d (期望 1, 换主场景期望 2)\n", executed)
	if total > 0 {
		fmt.Printf("去重率:                %.1f%% (节省调用 %d 次)\n",
			float64(total-executed)/float64(total)*100, total-executed)
	}
	fmt.Printf("结果一致:              %d/%d\n", okCount, total)
	fmt.Printf("请求失败:              %d\n", failCount)
	fmt.Printf("总耗时:                %v (约等于单次底层调用耗时, 证明并发被合并)\n", elapsed)

	fmt.Println("事件统计:")
	for _, name := range []string{"leader_elected", "follower_waiting", "leader_completed", "leader_timeout"} {
		fmt.Printf("  %-20s %d\n", name, events[name])
	}

	if len(leaderWorkers) > 0 {
		sort.Ints(leaderWorkers)
		fmt.Printf("执行实例分布:          %v\n", leaderWorkers)
	}
}

// waitForCount 轮询 key 直到计数 >= target 或超时
func waitForCount(ctx context.Context, rdb *redis.Client, key string, target int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if getCount(ctx, rdb, key) >= target {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return getCount(ctx, rdb, key) >= target
}

// getCount 读 Redis 整数计数(不存在视为 0)
func getCount(ctx context.Context, rdb *redis.Client, key string) int {
	val, err := rdb.Get(ctx, key).Int()
	if err != nil {
		return 0
	}
	return val
}

// getLeaderWorkers 读曾担任 leader 的 worker ID 集合
func getLeaderWorkers(ctx context.Context, rdb *redis.Client, runID string) []int {
	members, err := rdb.SMembers(ctx, leaderWorkersKey(runID)).Result()
	if err != nil {
		return nil
	}
	res := make([]int, 0, len(members))
	for _, m := range members {
		var id int
		if _, err := fmt.Sscanf(m, "worker-%d", &id); err == nil {
			res = append(res, id)
		}
	}
	return res
}

// cleanup 清理压测协调 key 与 singleflight 残留 key
func cleanup(ctx context.Context, rdb *redis.Client, runID string) {
	keys, _ := rdb.Keys(ctx, sfclusterRedisPrefix+runID+"*").Result()
	if len(keys) > 0 {
		_ = rdb.Del(ctx, keys...).Err()
	}
}

// ============================================================
// worker
// ============================================================

func runWorker(ctx context.Context, rdb *redis.Client, runID string, id, concurrency, failoverWorker int) {
	// 1. 先建立订阅并确认就绪(Pub/Sub 是异步的, 必须先订阅成功再注册,
	//    否则 coordinator 看到 ready 达标立即广播 start 时订阅还没建立, 信号会丢)
	sub := rdb.Subscribe(ctx, startChan(runID))
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		fmt.Printf("[worker-%d] 订阅 start 频道失败: %v\n", id, err)
		os.Exit(1)
	}
	ch := sub.Channel()

	fmt.Printf("[worker-%d] 注册就绪 run=%s\n", id, runID)
	// 2. 注册 ready
	if err := rdb.Incr(ctx, readyKey(runID)).Err(); err != nil {
		fmt.Printf("[worker-%d] 注册失败: %v\n", id, err)
		os.Exit(1)
	}

	fmt.Printf("[worker-%d] 等待 start 信号...\n", id)
	select {
	case <-ch:
	case <-time.After(startWaitTimeout):
		fmt.Printf("[worker-%d] 等待 start 超时 %v\n", id, startWaitTimeout)
		os.Exit(1)
	}

	// 3. 发起并发请求
	fmt.Printf("[worker-%d] 收到 start, 发起 %d 个并发请求\n", id, concurrency)
	g := singleflight.NewDistributedGroup(rdb)
	key := fmt.Sprintf("sfcluster:%s:req", runID)

	var (
		okCount, failCount int32
		failErrs           = make(chan string, concurrency)
	)
	g.SetMetricFunc(func(module, event string, success bool, extra string) {
		if event == "leader_elected" {
			// 跨进程记录"哪个实例执行了底层调用"(仅 leader 会触发此事件)
			_ = rdb.SAdd(ctx, leaderWorkersKey(runID), fmt.Sprintf("worker-%d", id)).Err()
		}
		_ = rdb.Incr(ctx, eventKey(runID, event)).Err()
	})

	// 模拟 AI 流式调用: 每 100ms 输出一段, 共 3 段, 单次耗时约 300ms
	fn := func(fnCtx context.Context, writer *singleflight.StreamWriter) (interface{}, error) {
		// 跨进程统计底层调用实际执行次数(仅 leader 执行到此)
		_ = rdb.Incr(fnCtx, execKey(runID)).Err()

		// 故障注入: 第一个成为 leader 的实例卡死(用 SETNX 全局标记判定, 无论哪个
		// worker 抢到锁都保证只触发一次), 不刷新心跳 → 其余实例检测停滞后换主接管。
		//
		// 注意两点:
		//   1. 必须监听 fnCtx.Done(): 被换主时 watchCancel 会 cancel execCtx,
		//      fn 需响应取消才能让旧主释放锁, 否则 heartbeatLoop 持续续期锁,
		//      follower 永远抢不到锁死循环。
		//   2. 返回成功结果而非 error: 旧主被换主后 runAsLeader 仍会把返回值
		//      写进 resultKey 并 Publish。返回 "cluster-ok" 与新主结果一致,
		//      follower 无论读到新旧主的结果都是 cluster-ok; 若返回 error 会
		//      Publish error 污染尚未换主的 follower。
		//   3. SETNX TTL 用较长时间(3min): 避免旧主 40s 假死期间标记过期,
		//      导致后续 leader 再次触发故障(级联换主)。
		if failoverWorker >= 0 {
			triggered, err := rdb.SetNX(fnCtx, failoverTriggerKey(runID), "1", 3*time.Minute).Result()
			if err == nil && triggered {
				fmt.Printf("[worker-%d] 故障注入: 成为首个 leader, 假死 %v(不刷新心跳)\n", id, failoverStall)
				select {
				case <-fnCtx.Done():
					// 被换主取消, 旧主响应释放锁; 返回成功结果避免 error 污染
					return "cluster-ok", nil
				case <-time.After(failoverStall):
				}
				return "cluster-ok", nil
			}
		}

		for i := 0; i < 3; i++ {
			select {
			case <-fnCtx.Done():
				return nil, fnCtx.Err()
			case <-time.After(100 * time.Millisecond):
			}
			if _, werr := writer.Write([]byte(fmt.Sprintf("chunk-%d", i))); werr != nil {
				return nil, werr
			}
		}
		return "cluster-ok", nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, workerCtxTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(concurrency)
	startTime := time.Now()
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			res, err := g.Do(reqCtx, key, fn)
			if err == nil && res == "cluster-ok" {
				atomic.AddInt32(&okCount, 1)
			} else {
				atomic.AddInt32(&failCount, 1)
				if err != nil {
					select {
					case failErrs <- err.Error():
					default:
					}
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(startTime)

	// 4. 上报本地统计
	ok := atomic.LoadInt32(&okCount)
	fail := atomic.LoadInt32(&failCount)
	_ = rdb.IncrBy(ctx, okKey(runID), int64(ok)).Err()
	_ = rdb.IncrBy(ctx, failKey(runID), int64(fail)).Err()
	_ = rdb.Incr(ctx, doneKey(runID)).Err()

	fmt.Printf("[worker-%d] 完成: 成功=%d 失败=%d 耗时=%v\n", id, ok, fail, elapsed)
	if fail > 0 {
		close(failErrs)
		for e := range failErrs {
			fmt.Printf("[worker-%d] 失败原因示例: %s\n", id, e)
		}
	}
}

// ============================================================
// key 构造
// ============================================================

func readyKey(runID string) string  { return sfclusterRedisPrefix + runID + readyKeySuffix }
func doneKey(runID string) string   { return sfclusterRedisPrefix + runID + doneKeySuffix }
func execKey(runID string) string   { return sfclusterRedisPrefix + runID + execKeySuffix }
func okKey(runID string) string     { return sfclusterRedisPrefix + runID + okKeySuffix }
func failKey(runID string) string   { return sfclusterRedisPrefix + runID + failKeySuffix }
func startChan(runID string) string { return sfclusterRedisPrefix + runID + startChanSuffix }
func eventKey(runID, event string) string {
	return sfclusterRedisPrefix + runID + ":event:" + event
}
func leaderWorkersKey(runID string) string {
	return sfclusterRedisPrefix + runID + leaderWorkersKeySuffix
}
func failoverTriggerKey(runID string) string {
	return sfclusterRedisPrefix + runID + ":failover_triggered"
}
