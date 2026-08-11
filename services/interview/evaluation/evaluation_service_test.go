package evaluation

import (
	"context"
	"strings"
	"testing"
)

// ============================================================
// 评分/出题/追问服务的纯逻辑测试(不触 AI 调用)
// ============================================================

func TestEvaluationService_NormalizeResult(t *testing.T) {
	svc := &EvaluationService{}
	r := svc.normalizeResult(map[string]interface{}{
		"score":              88,
		"feedback":           "回答完整",
		"follow_up_needed":   true,
		"follow_up_question": "请补充边界情况",
		"missing_points":     []interface{}{"缺A", "缺B"},
		"logic_ok":           true,
	})

	if r.Score != 88 {
		t.Errorf("Score = %d, want 88", r.Score)
	}
	if r.Feedback != "回答完整" {
		t.Errorf("Feedback = %q", r.Feedback)
	}
	if !r.FollowUpNeeded {
		t.Error("FollowUpNeeded = false, want true")
	}
	if r.FollowUpQuestion != "请补充边界情况" {
		t.Errorf("FollowUpQuestion = %q", r.FollowUpQuestion)
	}
	if len(r.MissingPoints) != 2 || r.MissingPoints[0] != "缺A" {
		t.Errorf("MissingPoints = %v", r.MissingPoints)
	}
	if !r.LogicOK {
		t.Error("LogicOK = false, want true")
	}
}

func TestEvaluationService_NormalizeResult_Aliases(t *testing.T) {
	svc := &EvaluationService{}
	r := svc.normalizeResult(map[string]interface{}{
		"total_score":    90,
		"comment":        "不错",
		"ask_to_user":    "展开讲讲",
		"lack_points":    []interface{}{"x"},
		"logicOk":        false,
		"followUpNeeded": false,
	})

	if r.Score != 90 || r.Feedback != "不错" || r.FollowUpQuestion != "展开讲讲" {
		t.Errorf("别名归一化失败: %+v", r)
	}
	if r.FollowUpNeeded {
		t.Error("followUpNeeded=false 应直接采用")
	}
	if r.LogicOK {
		t.Error("logicOk=false 未生效")
	}
}

func TestEvaluationService_NormalizeResult_MissingPointsNil(t *testing.T) {
	svc := &EvaluationService{}
	r := svc.normalizeResult(map[string]interface{}{"score": 60})
	if r.MissingPoints == nil {
		t.Error("MissingPoints 应为空切片而非 nil")
	}
	if len(r.MissingPoints) != 0 {
		t.Errorf("MissingPoints = %v", r.MissingPoints)
	}
}

func TestInferFollowUpNeeded(t *testing.T) {
	svc := &EvaluationService{}
	tests := []struct {
		name string
		r    *EvaluationResult
		want bool
	}{
		{"逻辑不正确", &EvaluationResult{LogicOK: false}, true},
		{"有缺失点", &EvaluationResult{LogicOK: true, MissingPoints: []string{"a"}}, true},
		{"有追问题", &EvaluationResult{LogicOK: true, FollowUpQuestion: "q"}, true},
		{"全部正常", &EvaluationResult{LogicOK: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.inferFollowUpNeeded(tt.r); got != tt.want {
				t.Errorf("inferFollowUpNeeded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluationService_NormalizeResult_InferFollowUp(t *testing.T) {
	// follow_up_needed 缺失时按推断规则补
	svc := &EvaluationService{}
	r := svc.normalizeResult(map[string]interface{}{"logic_ok": false})
	if !r.FollowUpNeeded {
		t.Error("logic_ok=false 应推断为需要追问")
	}
	r2 := svc.normalizeResult(map[string]interface{}{"logic_ok": true, "score": 90})
	if r2.FollowUpNeeded {
		t.Error("全部正常应推断为不需要追问")
	}
}

func TestHashContent(t *testing.T) {
	short := hashContent("a", "b")
	if short != "[a b]" {
		t.Errorf("短内容 = %q", short)
	}
	// 相同输入 hash 一致
	if hashContent("x", "y") != hashContent("x", "y") {
		t.Error("相同输入应产生相同 hash")
	}
	// 长内容截断到 64
	long := hashContent(strings.Repeat("v", 100))
	if len(long) != 64 {
		t.Errorf("长内容应截断到 64, got %d", len(long))
	}
}

func TestExtractionService_NormalizeResult(t *testing.T) {
	svc := &ExtractionService{}
	r := svc.normalizeResult(map[string]interface{}{
		"questions":    []interface{}{"q1", "q2"},
		"sugest":       []interface{}{"s1"},
		"type":         "后端",
		"resumeScore":  85,
		"custom_field": "保留字段",
	})

	if len(r.Questions) != 2 || r.Questions[0] != "q1" {
		t.Errorf("Questions = %v", r.Questions)
	}
	if len(r.Suggestions) != 1 || r.Suggestions[0] != "s1" {
		t.Errorf("Suggestions(别名 sugest) = %v", r.Suggestions)
	}
	if r.Type != "后端" || r.ResumeScore != 85 {
		t.Errorf("Type=%q ResumeScore=%d", r.Type, r.ResumeScore)
	}
	// 非结构化字段应保留在 ResumeContext
	if v, ok := r.ResumeContext["custom_field"]; !ok || v != "保留字段" {
		t.Errorf("ResumeContext 应保留 custom_field, got %v", r.ResumeContext)
	}
	// 结构化字段应被剔除
	for _, k := range []string{"questions", "suggestions", "sugest", "resumeScore", "type"} {
		if _, ok := r.ResumeContext[k]; ok {
			t.Errorf("ResumeContext 不应包含 %q", k)
		}
	}
}

func TestExtractionService_NormalizeResult_Empty(t *testing.T) {
	svc := &ExtractionService{}
	r := svc.normalizeResult(map[string]interface{}{})
	if r.Questions == nil || len(r.Questions) != 0 {
		t.Error("Questions 应为空切片而非 nil")
	}
	if r.Suggestions == nil || len(r.Suggestions) != 0 {
		t.Error("Suggestions 应为空切片而非 nil")
	}
	if r.ResumeContext == nil {
		t.Error("ResumeContext 应为非 nil map")
	}
}

func TestGenerateFollowUp_LimitReached(t *testing.T) {
	// followUpCount >= maxFollowUp → 直接结束, 不调 AI
	svc := &FollowUpService{}
	r, err := svc.GenerateFollowUp(context.Background(), "题", "答", nil, 2, 2)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !r.EndInterview {
		t.Error("达到追问上限应 EndInterview")
	}
}

func TestGenerateFollowUp_DefaultMaxFollowUp(t *testing.T) {
	// maxFollowUp <= 0 → 兜底 2; followUpCount=3 ≥ 2 → 结束
	svc := &FollowUpService{}
	r, err := svc.GenerateFollowUp(context.Background(), "题", "答", nil, 3, 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !r.EndInterview {
		t.Error("达到兜底上限应 EndInterview")
	}
}

func TestSanitizeFollowUpQuestion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"空串", "", ""},
		{"纯空白", "   ", ""},
		{"none", "none", ""},
		{"None 大小写", "None", ""},
		{"null", "null", ""},
		{"n/a", "n/a", ""},
		{"短横线", "-", ""},
		{"结束标记", "__finish__", ""},
		{"普通文本补问号", "请展开讲讲", "请展开讲讲?"},
		{"英文问号不补", "why?", "why?"},
		{"中文问号不补", "为什么？", "为什么？"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeFollowUpQuestion(tt.in); got != tt.want {
				t.Errorf("sanitizeFollowUpQuestion(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeFollowUpQuestion_Clip(t *testing.T) {
	long := strings.Repeat("问", 50) // 150 字节 > 100
	got := sanitizeFollowUpQuestion(long)
	if len(got) != 100 {
		t.Errorf("应 clip 到 100 字节, got %d", len(got))
	}
}
