package flow

// ============================================================
// 追问规则链（纯 Go 函数实现，对应 Java 的 LiteFlow 链）
//
// 执行顺序（短路，先到先得）:
//   1. 完成态守卫: 面试已结束 → 不追问
//   2. 追问上限守卫: followUpCount >= maxFollowUp → 不追问
//   3. AI 建议追问: followUpNeededFromAI → 追问
//   4. 低分追问: score < lowScoreThreshold → 追问
//   5. 缺失点追问: 有 missingPoints 或 followUpQuestionHint → 追问
//   6. 默认: 不追问
// ============================================================

const defaultLowScoreThreshold = 60

// FollowUpRuleContext 追问规则上下文
type FollowUpRuleContext struct {
	InterviewCompleted   bool     // 面试是否已结束
	FollowUpCount        int      // 当前主问题已追问轮次
	MaxFollowUp          int      // 追问上限(默认2)
	FollowUpNeededFromAI bool     // AI 建议是否追问
	Score                int      // 本轮得分[0,100]
	LowScoreThreshold    int      // 低分阈值(默认60)
	MissingPoints        []string // 缺失点
	FollowUpQuestionHint string   // 追问提示
}

// FollowUpRuleDecision 追问决策结果
type FollowUpRuleDecision struct {
	NeedFollowUp bool   // 是否需要追问
	ReasonCode   string // 决策原因码
}

// NeedFollowUp 构造"需要追问"决策
func NeedFollowUp(reason string) FollowUpRuleDecision {
	return FollowUpRuleDecision{NeedFollowUp: true, ReasonCode: reason}
}

// NoFollowUp 构造"不需要追问"决策
func NoFollowUp(reason string) FollowUpRuleDecision {
	return FollowUpRuleDecision{NeedFollowUp: false, ReasonCode: reason}
}

// DecideFollowUp 执行追问规则链
func DecideFollowUp(ctx *FollowUpRuleContext) FollowUpRuleDecision {
	// 规则配置兜底
	maxFollowUp := ctx.MaxFollowUp
	if maxFollowUp < 1 {
		maxFollowUp = defaultMaxFollowUp
	}
	lowScoreThreshold := ctx.LowScoreThreshold
	if lowScoreThreshold <= 0 {
		lowScoreThreshold = defaultLowScoreThreshold
	}

	// 1. 完成态守卫
	if ctx.InterviewCompleted {
		return NoFollowUp("INTERVIEW_COMPLETED")
	}

	// 2. 追问上限守卫
	if ctx.FollowUpCount >= maxFollowUp {
		return NoFollowUp("FOLLOW_UP_LIMIT_REACHED")
	}

	// 3. AI 建议追问
	if ctx.FollowUpNeededFromAI {
		return NeedFollowUp("AI_SUGGESTED")
	}

	// 4. 低分追问
	if ctx.Score < lowScoreThreshold {
		return NeedFollowUp("LOW_SCORE")
	}

	// 5. 缺失点追问
	if len(ctx.MissingPoints) > 0 || ctx.FollowUpQuestionHint != "" {
		return NeedFollowUp("MISSING_POINTS")
	}

	// 6. 默认不追问
	return NoFollowUp("DEFAULT")
}

// IsFollowUpQuestion 判断题号是否为追问题（格式: 数字-F数字，如 "1-F1"）
func IsFollowUpQuestion(questionNumber string) bool {
	if len(questionNumber) < 4 {
		return false
	}
	// 简单判定: 包含 "-F" 且前后有数字
	// 格式: {主号}-F{序号}
	dashF := false
	for i := 0; i < len(questionNumber)-1; i++ {
		if questionNumber[i] == '-' && (questionNumber[i+1] == 'F' || questionNumber[i+1] == 'f') {
			dashF = true
			break
		}
	}
	if !dashF {
		return false
	}
	// 前后都应有数字
	parts := splitFollowUpNumber(questionNumber)
	return parts[0] != "" && parts[1] != ""
}

// BuildFollowUpQuestionNumber 构造追问题号: "{main}-F{count}"
func BuildFollowUpQuestionNumber(mainQuestionNumber string, followUpCount int) string {
	return mainQuestionNumber + "-F" + itoa(followUpCount)
}

// ResolveMainQuestionNumber 从追问题号提取主问题号（"1-F1" → "1"）
func ResolveMainQuestionNumber(questionNumber string) string {
	for i := 0; i < len(questionNumber); i++ {
		if questionNumber[i] == '-' {
			return questionNumber[:i]
		}
	}
	return questionNumber
}

// splitFollowUpNumber 拆分 "1-F1" → ["1", "1"]
func splitFollowUpNumber(qn string) [2]string {
	var parts [2]string
	for i := 0; i < len(qn); i++ {
		if qn[i] == '-' && i+1 < len(qn) && (qn[i+1] == 'F' || qn[i+1] == 'f') {
			parts[0] = qn[:i]
			parts[1] = qn[i+2:]
			return parts
		}
	}
	return parts
}

// itoa 简单整数转字符串
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
