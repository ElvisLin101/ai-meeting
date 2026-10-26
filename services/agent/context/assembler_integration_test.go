package agentctx

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ai-meeting/internal/testutil"
	"ai-meeting/models"
	"ai-meeting/repositories"
)

// withTestRedis 把 repositories.RedisClient 替换为 miniredis，测试结束恢复。
func withTestRedis(t *testing.T) {
	t.Helper()
	old := repositories.RedisClient
	repositories.RedisClient = testutil.NewTestRedis(t)
	t.Cleanup(func() { repositories.RedisClient = old })
}

func seedAssemblerRedis(t *testing.T, profile *UserProfileData, summary string, covered int64, messages []models.Message) {
	t.Helper()
	ctx := context.Background()
	if err := repositories.SetAgentIdentity(ctx, 1, `{"name":"Tester","role":"test assistant"}`); err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	if profile != nil {
		raw, err := json.Marshal(profile)
		if err != nil {
			t.Fatalf("marshal profile: %v", err)
		}
		if err := repositories.SetUserProfile(ctx, "u1", string(raw)); err != nil {
			t.Fatalf("seed profile: %v", err)
		}
	}
	if summary != "" {
		if err := repositories.SetCompressedSummary(ctx, "s1", summary, covered); err != nil {
			t.Fatalf("seed summary: %v", err)
		}
	}
	if err := repositories.RebuildRecentMessages(ctx, "s1", messages); err != nil {
		t.Fatalf("seed recent: %v", err)
	}
}

func TestAssemblerUsesSummaryBoundary(t *testing.T) {
	withTestRedis(t)
	profile := &UserProfileData{Version: 1, Facts: []ProfileFact{
		{ID: "f1", Field: "goal", Key: "target_role", Value: "后端", Confidence: 0.9, Status: "active"},
	}}
	messages := []models.Message{
		{Sequence: 1, Role: "user", Content: "msg-1"},
		{Sequence: 2, Role: "assistant", Content: "msg-2"},
		{Sequence: 3, Role: "user", Content: "msg-3"},
		{Sequence: 4, Role: "assistant", Content: "msg-4"},
		{Sequence: 5, Role: "user", Content: "msg-5"},
	}
	seedAssemblerRedis(t, profile, "早前摘要", 2, messages)

	a := NewAssembler(Config{
		PromptBudgetTokens: 2000, IdentityMaxTokens: 300,
		ProfileMaxTokens: 300, SummaryMaxTokens: 300, RecentMaxTokens: 1000,
	})
	out, err := a.Build(context.Background(), BuildInput{
		UserID: "u1", SessionID: "s1", AgentID: 1, Query: "新问题",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	var all strings.Builder
	for _, m := range out.Messages {
		all.WriteString(m.Content)
		all.WriteString("\n")
	}
	content := all.String()
	if !strings.Contains(content, "早前摘要") {
		t.Fatal("summary should be included")
	}
	if strings.Contains(content, "msg-1") || strings.Contains(content, "msg-2") {
		t.Fatalf("messages covered by summary must not be included: %s", content)
	}
	for _, want := range []string{"msg-3", "msg-4", "msg-5"} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing short-term message %s", want)
		}
	}
	if out.CoveredThroughSeq != 2 || out.Report.CoveredThroughSeq != 2 {
		t.Fatalf("covered seq mismatch: out=%d report=%d", out.CoveredThroughSeq, out.Report.CoveredThroughSeq)
	}
	if out.Report.RecentMessages != 3 || out.Report.UsedProfile != true {
		t.Fatalf("unexpected report: %+v", out.Report)
	}
}

func TestAssemblerRendersOnlyActiveFacts(t *testing.T) {
	withTestRedis(t)
	profile := &UserProfileData{Version: 1, Facts: []ProfileFact{
		{ID: "active", Field: "goal", Key: "target_role", Value: "后端", Confidence: 0.9, Status: "active"},
		{ID: "old", Field: "goal", Key: "target_role", Value: "前端", Confidence: 0.9, Status: "superseded"},
	}}
	seedAssemblerRedis(t, profile, "摘要", 1, []models.Message{
		{Sequence: 2, Role: "user", Content: "hi"},
	})

	a := NewAssembler(Config{PromptBudgetTokens: 2000, ProfileMaxTokens: 300, RecentMaxTokens: 500})
	out, err := a.Build(context.Background(), BuildInput{
		UserID: "u1", SessionID: "s1", AgentID: 1, Query: "新问题",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var profileText string
	for _, m := range out.Messages {
		if strings.Contains(m.Content, "用户画像") {
			profileText = m.Content
		}
	}
	if !strings.Contains(profileText, "后端") {
		t.Fatalf("active fact missing: %q", profileText)
	}
	if strings.Contains(profileText, "前端") {
		t.Fatalf("superseded fact must not be rendered: %q", profileText)
	}
	if out.Report.ProfileFacts != 1 {
		t.Fatalf("want 1 active fact, got %d", out.Report.ProfileFacts)
	}
}

func TestAssemblerDropsRecentWhenOverBudget(t *testing.T) {
	withTestRedis(t)
	seedAssemblerRedis(t, &UserProfileData{Version: 1}, "摘要", 1, []models.Message{
		{Sequence: 2, Role: "user", Content: strings.Repeat("旧", 500)},
		{Sequence: 3, Role: "assistant", Content: strings.Repeat("新", 500)},
	})

	a := NewAssembler(Config{
		PromptBudgetTokens: 120, IdentityMaxTokens: 60,
		ProfileMaxTokens: 60, SummaryMaxTokens: 60, RecentMaxTokens: 60,
	})
	out, err := a.Build(context.Background(), BuildInput{
		UserID: "u1", SessionID: "s1", AgentID: 1, Query: "新问题",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if out.DroppedMessages == 0 {
		t.Fatalf("expected dropped messages, got report=%+v", out.Report)
	}
}
