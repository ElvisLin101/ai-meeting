package flow

import "testing"

// ============================================================
// 追问规则链测试: DecideFollowUp 六分支短路 + 阈值兜底 + 题号工具
// ============================================================

func TestDecideFollowUp(t *testing.T) {
	tests := []struct {
		name string
		ctx  *FollowUpRuleContext
		want bool
		code string
	}{
		{
			name: "面试已完成不追问",
			ctx:  &FollowUpRuleContext{InterviewCompleted: true, Score: 10},
			want: false, code: "INTERVIEW_COMPLETED",
		},
		{
			name: "达到追问上限不追问",
			ctx:  &FollowUpRuleContext{FollowUpCount: 2, MaxFollowUp: 2, Score: 10},
			want: false, code: "FOLLOW_UP_LIMIT_REACHED",
		},
		{
			name: "AI 建议追问",
			ctx:  &FollowUpRuleContext{FollowUpNeededFromAI: true, Score: 90},
			want: true, code: "AI_SUGGESTED",
		},
		{
			name: "低分触发追问",
			ctx:  &FollowUpRuleContext{Score: 40, LowScoreThreshold: 60},
			want: true, code: "LOW_SCORE",
		},
		{
			name: "缺失点触发追问",
			ctx:  &FollowUpRuleContext{Score: 90, MissingPoints: []string{"缺少边界处理"}},
			want: true, code: "MISSING_POINTS",
		},
		{
			name: "追问提示触发追问",
			ctx:  &FollowUpRuleContext{Score: 90, FollowUpQuestionHint: "深入追问"},
			want: true, code: "MISSING_POINTS",
		},
		{
			name: "默认不追问",
			ctx:  &FollowUpRuleContext{Score: 90},
			want: false, code: "DEFAULT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideFollowUp(tt.ctx)
			if got.NeedFollowUp != tt.want {
				t.Errorf("NeedFollowUp = %v, want %v (decision=%+v)", got.NeedFollowUp, tt.want, got)
			}
			if got.ReasonCode != tt.code {
				t.Errorf("ReasonCode = %q, want %q", got.ReasonCode, tt.code)
			}
		})
	}
}

func TestDecideFollowUp_DefaultThresholds(t *testing.T) {
	// MaxFollowUp=0 → 兜底默认 2; FollowUpCount=1 未超上限不应触发上限守卫
	if d := DecideFollowUp(&FollowUpRuleContext{FollowUpCount: 1, MaxFollowUp: 0, Score: 100}); d.ReasonCode != "DEFAULT" {
		t.Errorf("MaxFollowUp=0 应兜底默认值, got %+v", d)
	}
	// FollowUpCount=2 ≥ 默认上限 2 → 上限守卫
	if d := DecideFollowUp(&FollowUpRuleContext{FollowUpCount: 2, MaxFollowUp: 0, Score: 100}); d.ReasonCode != "FOLLOW_UP_LIMIT_REACHED" {
		t.Errorf("FollowUpCount=2 应触发默认上限, got %+v", d)
	}
	// LowScoreThreshold=0 → 兜底默认 60; score=40 < 60 → 低分追问
	if d := DecideFollowUp(&FollowUpRuleContext{Score: 40, LowScoreThreshold: 0}); d.ReasonCode != "LOW_SCORE" {
		t.Errorf("LowScoreThreshold=0 应兜底默认值, got %+v", d)
	}
	// 优先级: 完成态 > 上限 > AI 建议(即使 AI 建议为 true 也已完成则直接不追问)
	if d := DecideFollowUp(&FollowUpRuleContext{InterviewCompleted: true, FollowUpNeededFromAI: true}); d.NeedFollowUp {
		t.Errorf("完成态守卫应优先于 AI 建议, got %+v", d)
	}
}

func TestIsFollowUpQuestion(t *testing.T) {
	tests := []struct {
		qn   string
		want bool
	}{
		{"1-F1", true},
		{"1-F2", true},
		{"1-f1", true},    // 小写 f 兼容
		{"10-F3", true},   // 多位数主号
		{"1-F", false},    // 缺追问序号
		{"F1", false},     // 缺主号
		{"1", false},      // 普通题号
		{"abc", false},    // 非法格式
		{"", false},       // 空串
		{"1-F1-F2", true}, // 嵌套追问仍按拆分判定
	}
	for _, tt := range tests {
		if got := IsFollowUpQuestion(tt.qn); got != tt.want {
			t.Errorf("IsFollowUpQuestion(%q) = %v, want %v", tt.qn, got, tt.want)
		}
	}
}

func TestBuildFollowUpQuestionNumber(t *testing.T) {
	if got := BuildFollowUpQuestionNumber("1", 1); got != "1-F1" {
		t.Errorf("BuildFollowUpQuestionNumber(1,1) = %q, want %q", got, "1-F1")
	}
	if got := BuildFollowUpQuestionNumber("10", 3); got != "10-F3" {
		t.Errorf("BuildFollowUpQuestionNumber(10,3) = %q, want %q", got, "10-F3")
	}
}

func TestResolveMainQuestionNumber(t *testing.T) {
	tests := []struct {
		qn   string
		want string
	}{
		{"1-F1", "1"},
		{"10-F3", "10"},
		{"1", "1"}, // 无连字符原样返回
		{"", ""},
	}
	for _, tt := range tests {
		if got := ResolveMainQuestionNumber(tt.qn); got != tt.want {
			t.Errorf("ResolveMainQuestionNumber(%q) = %q, want %q", tt.qn, got, tt.want)
		}
	}
}

func TestSplitFollowUpNumber(t *testing.T) {
	got := splitFollowUpNumber("1-F1")
	if got[0] != "1" || got[1] != "1" {
		t.Errorf("splitFollowUpNumber(1-F1) = %v, want [1 1]", got)
	}
	got = splitFollowUpNumber("无连字符")
	if got[0] != "" || got[1] != "" {
		t.Errorf("splitFollowUpNumber(无连字符) = %v, want empty", got)
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{5, "5"},
		{123, "123"},
		{-7, "-7"},
		{-0, "0"},
	}
	for _, tt := range tests {
		if got := itoa(tt.n); got != tt.want {
			t.Errorf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
