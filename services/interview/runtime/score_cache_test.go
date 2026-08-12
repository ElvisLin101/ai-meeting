package runtime

import (
	"context"
	"testing"

	"ai-meeting/internal/testutil"
)

func TestScoreCache_AddScore(t *testing.T) {
	c := NewScoreCache(testutil.NewTestRedis(t))
	ctx := context.Background()

	sum, count, avg, err := c.AddScore(ctx, "s1", 80)
	if err != nil {
		t.Fatalf("AddScore failed: %v", err)
	}
	if sum != 80 || count != 1 || avg != 80 {
		t.Errorf("first AddScore = (%d,%d,%d), want (80,1,80)", sum, count, avg)
	}

	sum, count, avg, err = c.AddScore(ctx, "s1", 60)
	if err != nil {
		t.Fatalf("AddScore failed: %v", err)
	}
	if sum != 140 || count != 2 || avg != 70 {
		t.Errorf("second AddScore = (%d,%d,%d), want (140,2,70)", sum, count, avg)
	}

	total, err := c.GetTotalScore(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTotalScore failed: %v", err)
	}
	if total != 70 {
		t.Errorf("GetTotalScore = %d, want 70", total)
	}
}

func TestScoreCache_AddScore_Clamp(t *testing.T) {
	c := NewScoreCache(testutil.NewTestRedis(t))
	ctx := context.Background()

	if _, _, avg, err := c.AddScore(ctx, "s1", 150); err != nil || avg != 100 {
		t.Errorf("clamp high: avg=%d err=%v, want 100 nil", avg, err)
	}
	// -10 被夹到 0，(100+0)/2 = 50
	if _, _, avg, err := c.AddScore(ctx, "s1", -10); err != nil || avg != 50 {
		t.Errorf("clamp low: avg=%d err=%v, want 50 nil", avg, err)
	}
}

func TestScoreCache_Reset(t *testing.T) {
	c := NewScoreCache(testutil.NewTestRedis(t))
	ctx := context.Background()

	if _, _, _, err := c.AddScore(ctx, "s1", 90); err != nil {
		t.Fatalf("AddScore failed: %v", err)
	}
	if err := c.ResetScore(ctx, "s1"); err != nil {
		t.Fatalf("ResetScore failed: %v", err)
	}
	total, _ := c.GetTotalScore(ctx, "s1")
	if total != 0 {
		t.Errorf("total after reset = %d, want 0", total)
	}
}

func TestScoreCache_Empty(t *testing.T) {
	c := NewScoreCache(testutil.NewTestRedis(t))
	total, err := c.GetTotalScore(context.Background(), "no-score")
	if err != nil {
		t.Fatalf("GetTotalScore failed: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}
