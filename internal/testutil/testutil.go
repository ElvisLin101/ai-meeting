// Package testutil 提供跨包共享的测试公共设施(仅 _test.go 引用)。
//
// 背景: miniredis 初始化曾在 flow/runtime/lock 等多个测试包重复实现,
// 统一收敛到这里, 消除复制; 测试文件按需 import 本包即可。
package testutil

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// NewTestRedis 启动内存 Redis(miniredis) 并返回客户端, 测试结束时自动关闭。
func NewTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}
