package agentctx

import (
	"context"
	"sync"
)

// asyncPool 统一管理压缩/画像 consolidation 等后台任务。
// 进程停机时先 cancel，再等待 goroutine 退出，避免任务在 DB/Redis 关闭后才写库。
type asyncPool struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
}

func newAsyncPool(parent context.Context) *asyncPool {
	ctx, cancel := context.WithCancel(parent)
	return &asyncPool{ctx: ctx, cancel: cancel}
}

func (p *asyncPool) Go(fn func(context.Context)) bool {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return false
	}
	p.wg.Add(1)
	ctx := p.ctx
	p.mu.Unlock()

	go func() {
		defer p.wg.Done()
		fn(ctx)
	}()
	return true
}

func (p *asyncPool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.cancel()
	p.mu.Unlock()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var (
	asyncPoolMu sync.Mutex
	asyncPoolV  = newAsyncPool(context.Background())
)

// StartAsyncWorkers 由进程入口调用，把后台任务挂到可取消的父 context 上。
func StartAsyncWorkers(ctx context.Context) {
	asyncPoolMu.Lock()
	old := asyncPoolV
	asyncPoolV = newAsyncPool(ctx)
	asyncPoolMu.Unlock()
	if old != nil {
		_ = old.Shutdown(context.Background())
	}
}

// ShutdownAsyncWorkers 停止接收新任务并等待存量任务退出。
func ShutdownAsyncWorkers(ctx context.Context) error {
	asyncPoolMu.Lock()
	pool := asyncPoolV
	asyncPoolMu.Unlock()
	if pool == nil {
		return nil
	}
	return pool.Shutdown(ctx)
}

func runAsync(fn func(context.Context)) bool {
	asyncPoolMu.Lock()
	pool := asyncPoolV
	asyncPoolMu.Unlock()
	if pool == nil {
		return false
	}
	return pool.Go(fn)
}
