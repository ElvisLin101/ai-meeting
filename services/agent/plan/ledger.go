package plan

import "sync"

// Ledger 是单次 run 内的幂等账本。
//
// 语义:
//   - 同一个 IdempotencyKey 的并发执行只跑一次，其余等待并复用结果；
//   - 成功结果缓存，replan 重新加入同语义任务时直接复用；
//   - 失败结果不缓存，允许调度器重试；等待者仍然拿到这次失败结果。
//
// 这是「单进程 + 单 run」的账本；跨重启恢复需要把 TaskResult 落到
// MySQL（后续 agent_plan_tasks 表），当前 run 本身不支持断点恢复。
type Ledger struct {
	mu      sync.Mutex
	entries map[string]*ledgerEntry
}

type ledgerEntry struct {
	done   chan struct{}
	result TaskResult
}

// NewLedger 创建空账本。
func NewLedger() *Ledger {
	return &Ledger{entries: make(map[string]*ledgerEntry)}
}

// Do 以 key 为幂等键执行 fn；reused 表示结果来自已有执行。
func (l *Ledger) Do(key string, fn func() TaskResult) (result TaskResult, reused bool) {
	if key == "" || fn == nil {
		if fn == nil {
			return TaskResult{Status: StatusFailed, Error: "nil ledger function"}, false
		}
		return fn(), false
	}

	l.mu.Lock()
	if entry, ok := l.entries[key]; ok {
		l.mu.Unlock()
		<-entry.done
		res := entry.result
		res.Reused = true
		return res, true
	}
	entry := &ledgerEntry{done: make(chan struct{})}
	l.entries[key] = entry
	l.mu.Unlock()

	res := fn()
	l.mu.Lock()
	entry.result = res
	close(entry.done)
	// 失败不缓存，允许同一 key 重试；成功缓存，供 replan 复用。
	if res.Status != StatusDone {
		delete(l.entries, key)
	}
	l.mu.Unlock()
	return res, false
}
