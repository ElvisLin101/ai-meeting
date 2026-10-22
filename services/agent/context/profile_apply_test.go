package agentctx

import (
	"strings"
	"testing"

	"ai-meeting/models"
)

func TestNormalizeProfileMigratesLegacy(t *testing.T) {
	p := &UserProfileData{
		Goals:       []string{"找后端工作"},
		Constraints: []string{"只考虑上海"},
		Preferences: map[string]string{"语言": "中文"},
		Background:  map[string]interface{}{"years": 3},
	}
	NormalizeProfile(p)
	if len(p.Facts) != 4 {
		t.Fatalf("want 4 facts, got %d: %+v", len(p.Facts), p.Facts)
	}
	for _, f := range p.Facts {
		if f.ID == "" || f.Status != "active" || f.Confidence <= 0 {
			t.Fatalf("fact not normalized: %+v", f)
		}
	}
	if p.Version <= 0 {
		t.Fatal("version should be set")
	}
}

func TestApplyProfileDiffSupersedesOnHigherConfidence(t *testing.T) {
	p := &UserProfileData{Version: 1, Facts: []ProfileFact{
		{ID: "old", Field: "goal", Key: "target_role", Value: "后端", Confidence: 0.7, Status: "active"},
	}}
	next, questions, changed, err := ApplyProfileDiff(p, ProfileDiff{
		Reason: "role changed",
		Add: []ProfileFact{
			{Field: "goal", Key: "target_role", Value: "前端", Confidence: 0.95},
		},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !changed || len(questions) != 0 {
		t.Fatalf("expected change without questions: changed=%v q=%v", changed, questions)
	}
	active := next.ActiveFacts()
	if len(active) != 1 || active[0].Value != "前端" {
		t.Fatalf("new fact should be active: %+v", active)
	}
	foundSuperseded := false
	for _, f := range next.Facts {
		if f.ID == "old" && f.Status == "superseded" {
			foundSuperseded = true
		}
	}
	if !foundSuperseded {
		t.Fatal("old fact should be marked superseded")
	}
}

func TestApplyProfileDiffAsksOnConflict(t *testing.T) {
	p := &UserProfileData{Version: 1, Facts: []ProfileFact{
		{ID: "old", Field: "constraint", Key: "location", Value: "上海", Confidence: 0.9, Status: "active"},
	}}
	next, questions, _, err := ApplyProfileDiff(p, ProfileDiff{
		Reason: "possible conflict",
		Add: []ProfileFact{
			{Field: "constraint", Key: "location", Value: "北京", Confidence: 0.6},
		},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("want 1 question, got %+v", questions)
	}
	active := next.ActiveFacts()
	if len(active) != 1 || active[0].Value != "上海" {
		t.Fatalf("low-confidence fact must not overwrite: %+v", active)
	}
}

func TestApplyProfileDiffUpdateByID(t *testing.T) {
	p := &UserProfileData{Version: 1, Facts: []ProfileFact{
		{ID: "f1", Field: "background", Key: "years", Value: "3", Confidence: 0.6, Status: "active"},
	}}
	next, _, changed, err := ApplyProfileDiff(p, ProfileDiff{
		Update: []ProfileFact{{ID: "f1", Value: "5", Confidence: 0.9}},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !changed || len(next.ActiveFacts()) != 1 || next.ActiveFacts()[0].Value != "5" {
		t.Fatalf("update failed: %+v", next.ActiveFacts())
	}
}

func TestRenderProfileUsesFacts(t *testing.T) {
	p := &UserProfileData{Version: 1, Facts: []ProfileFact{
		{ID: "f1", Field: "goal", Key: "target_role", Value: "后端", Confidence: 0.9, Status: "active"},
	}}
	out := renderProfile(p)
	if !strings.Contains(out, "goal/target_role") || !strings.Contains(out, "后端") {
		t.Fatalf("unexpected profile render: %s", out)
	}
}

func TestNormalizeProfileDiff(t *testing.T) {
	diff := ProfileDiff{
		Add: []ProfileFact{
			{Field: "unknown", Value: "x"},
			{Field: "goal", Value: "后端", Confidence: 2},
		},
	}
	normalizeProfileDiff(&diff)
	if len(diff.Add) != 1 || diff.Add[0].Field != "goal" || diff.Add[0].Confidence != 1 {
		t.Fatalf("unexpected normalized diff: %+v", diff.Add)
	}
}

func TestSelectDialogueForConsolidationFiltersToolMessages(t *testing.T) {
	messages := []models.Message{
		{Role: "user", Content: "我想找后端"},
		{Role: "tool", Content: "tool output should be ignored"},
		{Role: "assistant", Content: "好的"},
	}
	out := selectDialogueForConsolidation(messages)
	if strings.Contains(out, "tool output") {
		t.Fatalf("tool messages should be excluded: %s", out)
	}
	if !strings.Contains(out, "user: 我想找后端") || !strings.Contains(out, "assistant: 好的") {
		t.Fatalf("dialogue missing user/assistant: %s", out)
	}
}

func TestDiffEmptyIgnoresCandidateMarks(t *testing.T) {
	if !diffEmpty(ProfileDiff{Used: []uint{1}, Reject: []uint{2}}) {
		t.Fatal("used/reject only diff should be considered empty for profile changes")
	}
	if diffEmpty(ProfileDiff{Add: []ProfileFact{{Field: "goal", Value: "x"}}}) {
		t.Fatal("add should not be empty")
	}
}
