package runtime

import "testing"

// ============================================================
// 运行态 Redis key 命名测试: 统一前缀 interview: + 业务域 + session
// ============================================================

func TestCacheKeys(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) string
		want string
	}{
		{"flowKey", flowKey, "interview:flow:session:s1"},
		{"questionsKey", questionsKey, "interview:questions:session:s1"},
		{"suggestionsKey", suggestionsKey, "interview:suggestions:session:s1"},
		{"followUpQuestionsKey", followUpQuestionsKey, "interview:follow_up_questions:session:s1"},
		{"resumeScoreKey", resumeScoreKey, "interview:resume_score:session:s1"},
		{"directionKey", directionKey, "interview:direction:session:s1"},
		{"resumeContextKey", resumeContextKey, "interview:resume_context:session:s1"},
		{"scoreKey", scoreKey, "interview:score:session:s1"},
		{"scoreSumKey", scoreSumKey, "interview:score_sum:session:s1"},
		{"scoreCountKey", scoreCountKey, "interview:score_count:session:s1"},
		{"turnsKey", turnsKey, "interview:turns:session:s1"},
		{"answerRequestKey", answerRequestKey, "interview:answer:req:session:s1"},
		{"turnRequestKey", turnRequestKey, "interview:turn:req:session:s1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn("s1"); got != tt.want {
				t.Errorf("%s(s1) = %q, want %q", tt.name, got, tt.want)
			}
			// 不同 session 必须产生不同 key, 避免会话间串数据
			if tt.fn("s1") == tt.fn("s2") {
				t.Errorf("%s: 不同 session 产生相同 key", tt.name)
			}
		})
	}
}
