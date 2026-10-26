package repositories

import (
	"context"
	"testing"

	"ai-meeting/internal/testutil"
	"ai-meeting/models"
)

func TestRebuildRecentMessagesKeepsChronologicalOrder(t *testing.T) {
	old := RedisClient
	RedisClient = testutil.NewTestRedis(t)
	t.Cleanup(func() { RedisClient = old })

	ctx := context.Background()
	messages := []models.Message{
		{Sequence: 1, Role: "user", Content: "old"},
		{Sequence: 2, Role: "assistant", Content: "mid"},
		{Sequence: 3, Role: "user", Content: "new"},
	}
	if err := RebuildRecentMessages(ctx, "s1", messages); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	got, err := GetRecentMessages(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 3 || got[0].Sequence != 1 || got[1].Sequence != 2 || got[2].Sequence != 3 {
		t.Fatalf("recent should be chronological, got %+v", got)
	}
}
