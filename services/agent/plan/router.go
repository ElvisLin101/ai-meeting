package plan

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// RouteDecision 是进入 Plan 模式前的轻量判断结果。
type RouteDecision struct {
	UsePlan bool
	Mode    string // plan / direct
	Score   int
	Reason  string
}

// Router 用规则给请求打分，决定是否进入 Plan 模式。
// 这里不调用 LLM：简单请求不应该为了“判断要不要规划”再付一次模型调用。
type Router struct {
	cfg Config
}

// NewRouter 创建前置路由器。
func NewRouter(cfg Config) *Router {
	return &Router{cfg: cfg.withDefaults()}
}

// Decide 返回是否进入 Plan 模式。规则分由以下信号组成：
//   - 强多步信号（先...再、分别、对比、并生成、改简历等）加分；
//   - 中信号（分析/总结/报告/简历/面试等）去重计数，最多 3 分；
//   - 长查询、多问号、多行/编号列表加分；
//   - 纯问候/明确要求直接回答，以及“短且无强信号”的查询减分。
func (r *Router) Decide(query string) RouteDecision {
	q := strings.TrimSpace(query)
	if r == nil || !r.cfg.Enabled || q == "" {
		return RouteDecision{Mode: "direct", Reason: "plan disabled or empty query"}
	}

	switch strings.ToLower(strings.TrimSpace(r.cfg.RouteMode)) {
	case "always":
		return RouteDecision{UsePlan: true, Mode: "plan", Reason: "route_mode=always"}
	case "never":
		return RouteDecision{UsePlan: false, Mode: "direct", Reason: "route_mode=never"}
	}

	score := 0
	var reasons []string

	// 纯问候只对“整句就是问候”生效，避免“你好，帮我改简历”被误判。
	simple := []string{"你好", "您好", "谢谢", "在吗", "你是谁", "介绍一下你自己", "好的", "嗯"}
	for _, s := range simple {
		if q == s || q == s+"。" || q == s+"！" || q == s+"!" {
			score -= 3
			reasons = append(reasons, "simple:"+s)
			break
		}
	}
	for _, s := range []string{"直接回答", "简单说", "一句话", "不用展开", "只回答", "别展开"} {
		if strings.Contains(q, s) {
			score -= 3
			reasons = append(reasons, "direct:"+s)
			break
		}
	}

	// 强多步信号：命中多个最多计 4 分，避免关键词堆叠无限加分。
	strong := 0
	addStrong := func(reason string) {
		if strong >= 4 {
			return
		}
		strong += 2
		reasons = append(reasons, "strong:"+reason)
	}
	if strings.Contains(q, "先") &&
		(strings.Contains(q, "再") || strings.Contains(q, "然后") || strings.Contains(q, "最后")) {
		addStrong("先...再/然后/最后")
	}
	for _, s := range []string{
		"分别", "对比", "比较", "拆解", "分步骤", "第一步", "首先",
		"并生成", "并输出", "并给出", "多个", "三家公司", "两份", "几家公司", "三项",
	} {
		if strings.Contains(q, s) {
			addStrong(s)
		}
	}
	if (strings.Contains(q, "改") && strings.Contains(q, "简历")) ||
		(strings.Contains(q, "简历") && (strings.Contains(q, "优化") || strings.Contains(q, "润色"))) {
		addStrong("简历改写")
	}
	score += strong

	// 中信号去重计数。
	mediumWords := []string{"分析", "总结", "评估", "改进", "优化", "规划", "计划", "报告", "方案", "调研", "研究", "简历", "面试", "复盘"}
	medium := 0
	for _, s := range mediumWords {
		if !strings.Contains(q, s) {
			continue
		}
		medium++
		reasons = append(reasons, "medium:"+s)
	}
	if medium > 3 {
		medium = 3
	}
	score += medium

	runes := utf8.RuneCountInString(q)
	if runes >= r.cfg.LongQueryRunes {
		score++
		reasons = append(reasons, "long")
	}
	if runes >= r.cfg.LongQueryRunes*2 {
		score++
		reasons = append(reasons, "very-long")
	}

	questionMarks := strings.Count(q, "?") + strings.Count(q, "？")
	if questionMarks >= 2 {
		score++
		reasons = append(reasons, "multi-question")
	}
	if len(strings.Split(q, "\n")) >= 3 {
		score++
		reasons = append(reasons, "multi-line")
	}

	// 短查询且没有强信号：默认是简单问答，直接走 ReAct。
	if strong == 0 && runes <= r.cfg.FastPathRunes && questionMarks <= 1 {
		score--
		reasons = append(reasons, "short-simple")
	}

	usePlan := score >= r.cfg.RouteMinScore
	mode := "direct"
	if usePlan {
		mode = "plan"
	}
	reason := "no-signal"
	if len(reasons) > 0 {
		reason = strings.Join(reasons, ",")
	}
	return RouteDecision{
		UsePlan: usePlan,
		Mode:    mode,
		Score:   score,
		Reason:  fmt.Sprintf("%s (score=%d)", reason, score),
	}
}
