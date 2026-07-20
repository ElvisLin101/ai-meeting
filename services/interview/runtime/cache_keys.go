package runtime

import "fmt"

// ============================================================
// 面试运行态 Redis key 命名规则
// 统一前缀 interview: + 业务域 + :session: + sessionId
// TTL 默认 24 小时(见 cacheTTL)
// ============================================================

const cacheTTLHours = 24

func flowKey(sessionID string) string {
	return fmt.Sprintf("interview:flow:session:%s", sessionID)
}

func questionsKey(sessionID string) string {
	return fmt.Sprintf("interview:questions:session:%s", sessionID)
}

func suggestionsKey(sessionID string) string {
	return fmt.Sprintf("interview:suggestions:session:%s", sessionID)
}

func followUpQuestionsKey(sessionID string) string {
	return fmt.Sprintf("interview:follow_up_questions:session:%s", sessionID)
}

func resumeScoreKey(sessionID string) string {
	return fmt.Sprintf("interview:resume_score:session:%s", sessionID)
}

func directionKey(sessionID string) string {
	return fmt.Sprintf("interview:direction:session:%s", sessionID)
}

func resumeContextKey(sessionID string) string {
	return fmt.Sprintf("interview:resume_context:session:%s", sessionID)
}

func scoreKey(sessionID string) string {
	return fmt.Sprintf("interview:score:session:%s", sessionID)
}

func scoreSumKey(sessionID string) string {
	return fmt.Sprintf("interview:score_sum:session:%s", sessionID)
}

func scoreCountKey(sessionID string) string {
	return fmt.Sprintf("interview:score_count:session:%s", sessionID)
}

func turnsKey(sessionID string) string {
	return fmt.Sprintf("interview:turns:session:%s", sessionID)
}

func answerRequestKey(sessionID string) string {
	return fmt.Sprintf("interview:answer:req:session:%s", sessionID)
}

func turnRequestKey(sessionID string) string {
	return fmt.Sprintf("interview:turn:req:session:%s", sessionID)
}
