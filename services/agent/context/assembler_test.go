package agentctx

import (
	"strings"
	"testing"
)

func TestMergeProfile(t *testing.T) {
	old := &UserProfileData{
		Preferences: map[string]string{"语言": "中文"},
		Goals:       []string{"后端"},
		Background:  map[string]interface{}{"target_role": "Go 后端"},
		Version:     1,
	}
	fresh := &UserProfileData{
		Preferences: map[string]string{"沟通风格": "直接"},
		Goals:       []string{"基础设施"},
		Version:     1,
	}
	merged := mergeProfile(old, fresh)

	if merged.Preferences["语言"] != "中文" || merged.Preferences["沟通风格"] != "直接" {
		t.Fatalf("preferences not merged: %+v", merged.Preferences)
	}
	if len(merged.Goals) != 1 || merged.Goals[0] != "基础设施" {
		t.Fatalf("goals should be overridden by fresh: %+v", merged.Goals)
	}
	if merged.Background["target_role"] != "Go 后端" {
		t.Fatalf("background should be preserved: %+v", merged.Background)
	}
	if merged.Version != 2 {
		t.Fatalf("version should increment, got %d", merged.Version)
	}
}

func TestBuildIdentityPrompt(t *testing.T) {
	prompt := buildIdentityPrompt(DefaultIdentity())
	for _, want := range []string{"CareerAgent", "能力", "边界", "工具使用规则", "不编造"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("identity prompt missing %q", want)
		}
	}
}

func TestExtractJSONObject(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"prefix {\"a\":1} suffix": `{"a":1}`,
		`{"a":1}`:                 `{"a":1}`,
	}
	for in, want := range cases {
		if got := extractJSONObject(in); got != want {
			t.Errorf("extractJSONObject(%q)=%q, want %q", in, got, want)
		}
	}
}
