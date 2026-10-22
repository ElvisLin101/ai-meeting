package plan

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TaskRunner 执行单个任务。upstream 只包含已成功的直接依赖结果。
type TaskRunner func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult

// RetryPolicy 控制任务级重试。只对 Retryable=true 的失败生效。
type RetryPolicy struct {
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

// Scheduler 执行任务图：按依赖分层、有界并行、失败向依赖方传播。
//
// 设计约束:
//   - 计划修改只发生在轮次边界（RunRound 返回 NeedsReplan 后由上层串行处理），
//     调度器内部不并发修改 Plan；
//   - 同一批多个任务失败/hint 会合并成一次 replan 信号，避免每个任务各改一次；
//   - 幂等账本 + 任务级重试保证失败重试和 replan 重新加入时不重复执行；
//   - 已完成任务在后续轮次冻结，不会再次执行。
type Scheduler struct {
	MaxParallel int
	TaskTimeout time.Duration
	RetryPolicy RetryPolicy
	Ledger      *Ledger
	// ReplanEnabled 为 true 时，一批任务结束后若出现失败/ReplanHint，
	// RunRound 提前返回，让上层决定是否 replan。
	ReplanEnabled bool
}

// RoundResult 是一轮调度的结果。NeedsReplan 为 true 时，Results 是当前快照，
// 上层可以应用 diff 后带着 Results 再调用 RunRound。
type RoundResult struct {
	Results     map[string]TaskResult
	NeedsReplan bool
	Reasons     []string
}

// Run 执行整个计划（兼容旧调用）；不触发 replan。
func (s *Scheduler) Run(ctx context.Context, p *Plan, run TaskRunner) []TaskResult {
	rr := s.RunRound(ctx, p, nil, run)
	out := make([]TaskResult, 0, len(p.Tasks))
	for _, t := range p.Tasks {
		if r, ok := rr.Results[t.ID]; ok {
			out = append(out, r)
		}
	}
	return out
}

// RunRound 从 prior 结果继续执行，直到计划完成或需要 replan。
func (s *Scheduler) RunRound(ctx context.Context, p *Plan, prior map[string]TaskResult, run TaskRunner) RoundResult {
	if p == nil || len(p.Tasks) == 0 {
		return RoundResult{Results: map[string]TaskResult{}}
	}
	if run == nil {
		run = func(context.Context, Task, map[string]TaskResult) TaskResult {
			return TaskResult{Status: StatusFailed, Error: "task runner is nil"}
		}
	}
	maxParallel := s.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 2
	}

	var mu sync.Mutex
	results := make(map[string]TaskResult, len(p.Tasks))
	for id, r := range prior {
		results[id] = r
	}
	setResult := func(r TaskResult) {
		mu.Lock()
		results[r.ID] = r
		mu.Unlock()
	}
	snapshot := func() map[string]TaskResult {
		mu.Lock()
		defer mu.Unlock()
		out := make(map[string]TaskResult, len(results))
		for k, v := range results {
			out[k] = v
		}
		return out
	}

	byID := make(map[string]Task, len(p.Tasks))
	indeg := make(map[string]int, len(p.Tasks))
	adj := make(map[string][]string, len(p.Tasks))
	order := make([]string, 0, len(p.Tasks))
	pending := make(map[string]bool, len(p.Tasks))
	for _, t := range p.Tasks {
		byID[t.ID] = t
		order = append(order, t.ID)
		for _, dep := range t.DependsOn {
			adj[dep] = append(adj[dep], t.ID)
		}

		priorResult, hasPrior := prior[t.ID]
		if !hasPrior {
			pending[t.ID] = true
		} else {
			switch priorResult.Status {
			case StatusDone, StatusFailed, StatusSkipped, StatusCanceled:
				// 本轮不再执行；失败任务由上层决定是否 replan/更新后清除结果。
			default:
				pending[t.ID] = true
			}
		}

		// 只有「还没到终态」的依赖才计入入度；终态但非 done 的依赖会让下游
		// 变成 ready，然后在依赖检查里被标 skipped。
		for _, dep := range t.DependsOn {
			switch prior[dep].Status {
			case StatusDone, StatusFailed, StatusSkipped, StatusCanceled:
			default:
				indeg[t.ID]++
			}
		}
	}

	type readyItem struct {
		task     Task
		upstream map[string]TaskResult
	}

	for {
		if ctx.Err() != nil {
			break
		}
		current := snapshot()
		var ready []readyItem
		for _, id := range order {
			if !pending[id] || indeg[id] != 0 {
				continue
			}
			task := byID[id]
			skip := false
			upstream := make(map[string]TaskResult, len(task.DependsOn))
			for _, dep := range task.DependsOn {
				r, ok := current[dep]
				if !ok || r.Status != StatusDone {
					skip = true
					break
				}
				upstream[dep] = r
			}
			if skip {
				r := TaskResult{
					ID: id, Status: StatusSkipped, Error: "依赖任务未成功完成",
					IdempotencyKey: task.IdempotencyKey,
				}
				setResult(r)
				current[id] = r
				delete(pending, id)
				for _, down := range adj[id] {
					indeg[down]--
				}
				continue
			}
			ready = append(ready, readyItem{task: task, upstream: upstream})
		}

		if len(ready) == 0 {
			if len(pending) == 0 {
				break
			}
			// Validate 已排除环；这里兜底，避免死循环。
			for _, id := range order {
				if pending[id] {
					setResult(TaskResult{ID: id, Status: StatusSkipped, Error: "计划无法继续"})
					delete(pending, id)
				}
			}
			break
		}

		sem := make(chan struct{}, maxParallel)
		var wg sync.WaitGroup
		for _, item := range ready {
			item := item
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						setResult(TaskResult{
							ID: item.task.ID, Status: StatusFailed,
							Error:          fmt.Sprintf("task panic: %v", r),
							IdempotencyKey: item.task.IdempotencyKey,
						})
					}
				}()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					setResult(TaskResult{
						ID: item.task.ID, Status: StatusCanceled, Error: ctx.Err().Error(),
						IdempotencyKey: item.task.IdempotencyKey,
					})
					return
				}
				defer func() { <-sem }()

				taskCtx := ctx
				var cancel context.CancelFunc
				if s.TaskTimeout > 0 {
					taskCtx, cancel = context.WithTimeout(ctx, s.TaskTimeout)
					defer cancel()
				}

				res := s.executeTask(taskCtx, item.task, item.upstream, run)
				setResult(res)
			}()
		}
		wg.Wait()

		// replan 触发器：一批任务结束后统一评估，多个失败/hint 合并成一次。
		if s.ReplanEnabled {
			batch := snapshot()
			var reasons []string
			for _, item := range ready {
				r := batch[item.task.ID]
				switch {
				case r.Status == StatusFailed:
					reasons = append(reasons, fmt.Sprintf("task %s failed: %s", r.ID, r.Error))
				case r.ReplanHint != "":
					reasons = append(reasons, fmt.Sprintf("task %s requests replan: %s", r.ID, r.ReplanHint))
				}
			}
			if len(reasons) > 0 {
				return RoundResult{Results: batch, NeedsReplan: true, Reasons: reasons}
			}
		}

		for _, item := range ready {
			delete(pending, item.task.ID)
			for _, down := range adj[item.task.ID] {
				indeg[down]--
			}
		}
	}

	for _, id := range order {
		if !pending[id] {
			continue
		}
		st := StatusSkipped
		msg := "未执行"
		if ctx.Err() != nil {
			st = StatusCanceled
			msg = ctx.Err().Error()
		}
		setResult(TaskResult{ID: id, Status: st, Error: msg})
		delete(pending, id)
	}
	return RoundResult{Results: snapshot()}
}

// executeTask 在任务级预算内执行，按 Retryable 决定是否重试。
// 同一 IdempotencyKey 通过账本去重；失败不缓存，允许下一次尝试。
func (s *Scheduler) executeTask(ctx context.Context, task Task, upstream map[string]TaskResult, run TaskRunner) TaskResult {
	maxAttempts := s.RetryPolicy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	// 任务可以要求更少尝试（例如长任务只跑一次），但不能超过服务端上限。
	if task.MaxAttempts > 0 && task.MaxAttempts < maxAttempts {
		maxAttempts = task.MaxAttempts
	}

	var last TaskResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return TaskResult{
				ID: task.ID, Status: StatusCanceled, Error: ctx.Err().Error(),
				IdempotencyKey: task.IdempotencyKey, Attempts: attempt - 1,
			}
		}

		exec := func() TaskResult {
			return safeRun(run, ctx, task, upstream)
		}
		var res TaskResult
		if s.Ledger != nil {
			res, _ = s.Ledger.Do(task.IdempotencyKey, exec)
		} else {
			res = exec()
		}
		if res.ID == "" {
			res.ID = task.ID
		}
		if res.IdempotencyKey == "" {
			res.IdempotencyKey = task.IdempotencyKey
		}
		res.Attempts = attempt
		if res.Status == "" {
			if res.Error != "" {
				res.Status = StatusFailed
			} else {
				res.Status = StatusDone
			}
		}
		switch res.Status {
		case StatusDone, StatusFailed, StatusSkipped, StatusCanceled:
		default:
			res.Status = StatusFailed
			if res.Error == "" {
				res.Error = "task returned invalid status"
			}
		}
		last = res

		if res.Status != StatusFailed || !res.Retryable || attempt >= maxAttempts {
			return res
		}
		if ctx.Err() != nil {
			return res
		}

		backoff := s.backoff(attempt)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return TaskResult{
				ID: task.ID, Status: StatusCanceled, Error: ctx.Err().Error(),
				IdempotencyKey: task.IdempotencyKey, Attempts: attempt,
			}
		}
	}
	return last
}

func safeRun(run TaskRunner, ctx context.Context, task Task, upstream map[string]TaskResult) (res TaskResult) {
	defer func() {
		if r := recover(); r != nil {
			res = TaskResult{
				ID: task.ID, Status: StatusFailed,
				Error:          fmt.Sprintf("task panic: %v", r),
				IdempotencyKey: task.IdempotencyKey,
			}
		}
	}()
	return run(ctx, task, upstream)
}

func (s *Scheduler) backoff(attempt int) time.Duration {
	base := s.RetryPolicy.BaseBackoff
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	maxBackoff := s.RetryPolicy.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 5 * time.Second
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
