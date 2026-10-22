package tools

import (
	"context"
	"fmt"
	"sync"

	"ai-meeting/clients"
)

// ============================================================
// BatchExecutor: 一次 assistant 消息内多个 tool call 的批调度
//
// 规则:
//  1. 所有工具都声明 ConcurrencySafe=true 时并行执行（信号量限制 maxParallel）
//  2. 任一工具未声明或声明为 false 时，整批串行，避免未知副作用并发
//  3. 结果按原始 calls 顺序返回，保证 tool message 与 tool_call_id 对齐
//  4. 单个工具失败不影响同批其他工具；错误随 outcome 返回
//  5. ctx 取消时停止派发新调用，已启动的调用通过 ctx 传递取消
//
// 这里只是「并行批次」，还不是带 depends_on 的完整 DAG。
// 有先后依赖的调用应由模型分步发起，后续再引入 GraphScheduler。
// ============================================================

// CallOutcome 是单个 tool call 的执行结果
type CallOutcome struct {
	Call   clients.ToolCall
	Result Result
	Err    error
}

// BatchExecutor 负责一批 tool call 的并发/串行调度
type BatchExecutor struct {
	registry    *Registry
	executor    *Executor
	maxParallel int
}

// NewBatchExecutor 创建批调度器；maxParallel <= 0 时默认 4
func NewBatchExecutor(registry *Registry, executor *Executor, maxParallel int) *BatchExecutor {
	if maxParallel <= 0 {
		maxParallel = 4
	}
	return &BatchExecutor{registry: registry, executor: executor, maxParallel: maxParallel}
}

// Execute 执行一批 tool call，返回顺序与 calls 一致。
func (b *BatchExecutor) Execute(ctx context.Context, tc *Context, calls []clients.ToolCall) []CallOutcome {
	outcomes := make([]CallOutcome, len(calls))
	if len(calls) == 0 {
		return outcomes
	}

	if b.maxParallel <= 1 || !b.allConcurrencySafe(calls) {
		for i, call := range calls {
			outcomes[i] = b.runOne(ctx, tc, call)
		}
		return outcomes
	}

	sem := make(chan struct{}, b.maxParallel)
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c clients.ToolCall) {
			defer wg.Done()
			// Executor 内部已 recover 工具 panic；这里再兜一层，
			// 防止调度层自身的意外 panic 拖垮整个 run。
			defer func() {
				if r := recover(); r != nil {
					outcomes[idx] = CallOutcome{Call: c, Err: fmt.Errorf("tool batch panic: %v", r)}
				}
			}()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				outcomes[idx] = CallOutcome{Call: c, Err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			outcomes[idx] = b.runOne(ctx, tc, c)
		}(i, call)
	}
	wg.Wait()
	return outcomes
}

func (b *BatchExecutor) runOne(ctx context.Context, tc *Context, call clients.ToolCall) CallOutcome {
	if err := ctx.Err(); err != nil {
		return CallOutcome{Call: call, Err: err}
	}
	res, err := b.executor.Execute(ctx, tc, call)
	return CallOutcome{Call: call, Result: res, Err: err}
}

// allConcurrencySafe 判断整批是否都可以并行。
// 未知工具返回 false：交给 Executor 报「工具不存在」，整批串行更保守。
func (b *BatchExecutor) allConcurrencySafe(calls []clients.ToolCall) bool {
	for _, call := range calls {
		t, ok := b.registry.Get(call.Function.Name)
		if !ok {
			return false
		}
		if !ToolMetaOf(t).ConcurrencySafe {
			return false
		}
	}
	return true
}
