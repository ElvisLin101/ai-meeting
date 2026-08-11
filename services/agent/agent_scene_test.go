package agent

import "testing"

// ============================================================
// 业务智能体场景枚举测试
// ============================================================

func TestBusinessAgentScene_GetCode(t *testing.T) {
	tests := []struct {
		scene BusinessAgentScene
		code  string
	}{
		{SceneGeneralAgentChat, "general-agent-chat"},
		{SceneInterviewQuestionExtraction, "interview-question-extraction"},
		{SceneInterviewAnswerEvaluation, "interview-answer-evaluation"},
		{SceneInterviewQuestionAsking, "interview-question-asking"},
	}
	for _, tt := range tests {
		if got := tt.scene.GetCode(); got != tt.code {
			t.Errorf("scene %d GetCode() = %q, want %q", tt.scene, got, tt.code)
		}
	}
}

func TestBusinessAgentScene_GetCandidateAgentNames(t *testing.T) {
	tests := []struct {
		scene    BusinessAgentScene
		contains string
	}{
		{SceneGeneralAgentChat, "通用智能体"},
		{SceneInterviewQuestionExtraction, "面试出题官"},
		{SceneInterviewAnswerEvaluation, "用户答案评分官"},
		{SceneInterviewQuestionAsking, "面试提问官"},
	}
	for _, tt := range tests {
		names := tt.scene.GetCandidateAgentNames()
		if names == nil {
			t.Fatalf("scene %d GetCandidateAgentNames() = nil", tt.scene)
		}
		found := false
		for _, n := range names {
			if n == tt.contains {
				found = true
			}
		}
		if !found {
			t.Errorf("scene %d 候选名 %v 缺少 %q", tt.scene, names, tt.contains)
		}
		// 默认名应在候选名最前
		if names[0] != tt.contains {
			t.Errorf("scene %d 默认名应为 %q, got %q", tt.scene, tt.contains, names[0])
		}
	}
}

func TestBusinessAgentScene_Invalid(t *testing.T) {
	// 未注册的场景值返回空, 不 panic
	invalid := BusinessAgentScene(99)
	if got := invalid.GetCode(); got != "" {
		t.Errorf("非法场景 GetCode() = %q, want \"\"", got)
	}
	if got := invalid.GetCandidateAgentNames(); got != nil {
		t.Errorf("非法场景 GetCandidateAgentNames() = %v, want nil", got)
	}
}
