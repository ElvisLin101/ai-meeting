package agentctx

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"ai-meeting/clients"
	"ai-meeting/models"
	"ai-meeting/repositories"
)

func TestConsolidateAppliesDiffAndMarksCandidate(t *testing.T) {
	withTestRedis(t)
	mock := withSQLMock(t)
	ctx := context.Background()
	sessionID, userID := "s1", "u1"

	profile := &UserProfileData{Version: 1}
	rawProfile, _ := json.Marshal(profile)
	if err := repositories.SetUserProfile(ctx, userID, string(rawProfile)); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if err := repositories.RebuildRecentMessages(ctx, sessionID, []models.Message{
		{Sequence: 1, Role: "user", Content: "我想找后端"},
		{Sequence: 2, Role: "assistant", Content: "好的"},
		{Sequence: 3, Role: "user", Content: "帮我准备面试"},
	}); err != nil {
		t.Fatalf("seed recent: %v", err)
	}
	seeded, seedErr := repositories.GetRecentMessages(ctx, sessionID)
	if seedErr != nil || len(seeded) != 3 {
		t.Fatalf("redis seed failed: err=%v len=%d %+v", seedErr, len(seeded), seeded)
	}

	mock.ExpectQuery("SELECT \\* FROM `profile_candidates` WHERE user_id = .+ AND status = .+ ORDER BY id ASC LIMIT 50").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "session_id", "message_id", "field", "value", "value_hash",
			"confidence", "quote", "rule", "status", "created_at", "updated_at",
		}).AddRow(10, userID, sessionID, 1, "goal", "", "hash-10", 0.7, "我想找后端", "goal_keyword", "pending", time.Now(), time.Now()))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `messages` WHERE session_id = .+ AND user_id = .+ AND sequence > .+ AND role = .+").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectExec("UPDATE `user_profiles` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `profile_candidates` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `profile_events`").
		WillReturnResult(sqlmock.NewResult(1, 1))

	orig := callConsolidateLLM
	callConsolidateLLM = func(ctx context.Context, aiID uint, messages []clients.PromptMessage, temperature float64) (string, error) {
		return `{"reason":"goal","add":[{"field":"goal","key":"target_role","value":"后端","confidence":0.9,"quote":"我想找后端"}],"used":[10]}`, nil
	}
	t.Cleanup(func() { callConsolidateLLM = orig })

	if err := ConsolidateIfNeeded(ctx, userID, sessionID); err != nil {
		t.Fatalf("consolidate: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}

	raw, ok := repositories.GetUserProfile(ctx, userID)
	if !ok {
		t.Fatal("profile should be written to Redis")
	}
	var got UserProfileData
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	active := got.ActiveFacts()
	if len(active) != 1 || active[0].Value != "后端" {
		t.Fatalf("unexpected active facts: %+v", active)
	}
	if got.LastConsolidatedSeq != 3 {
		t.Fatalf("last consolidated seq = %d, want 3", got.LastConsolidatedSeq)
	}
}
