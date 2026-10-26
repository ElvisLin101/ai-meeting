package run

import (
	"context"
	"testing"

	"ai-meeting/internal/testutil"
	"ai-meeting/repositories"
)

func withQuotaRedis(t *testing.T) {
	t.Helper()
	old := repositories.RedisClient
	repositories.RedisClient = testutil.NewTestRedis(t)
	t.Cleanup(func() { repositories.RedisClient = old })
}

func TestAcquireRunSlot(t *testing.T) {
	withQuotaRedis(t)
	ctx := context.Background()

	release, err := acquireRunSlot(ctx, "u1", 1)
	if err != nil {
		t.Fatalf("first slot should acquire: %v", err)
	}
	if _, err := acquireRunSlot(ctx, "u1", 1); err == nil {
		t.Fatal("second slot should be rejected")
	}
	release()

	release2, err := acquireRunSlot(ctx, "u1", 1)
	if err != nil {
		t.Fatalf("slot should be reusable after release: %v", err)
	}
	release2()
}

func TestDailyTokenQuota(t *testing.T) {
	withQuotaRedis(t)
	ctx := context.Background()

	if err := checkDailyTokenQuota(ctx, "u1", 10); err != nil {
		t.Fatalf("empty usage should pass: %v", err)
	}
	addDailyTokens("u1", 10)
	if err := checkDailyTokenQuota(ctx, "u1", 10); err == nil {
		t.Fatal("usage at limit should be rejected")
	}
	if err := checkDailyTokenQuota(ctx, "u1", 20); err != nil {
		t.Fatalf("usage under higher limit should pass: %v", err)
	}
}
