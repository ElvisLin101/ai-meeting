package agentctx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"

	"github.com/sirupsen/logrus"
)

// ============================================================
// L1 画像候选：每轮只做规则筛选，不调用 LLM，不直接写正式画像。
//
// 候选只记录“这条消息可能包含长期记忆点”，真正的结构化抽取和合并
// 由 L2 consolidation 完成。这样每轮成本近似为零，且不会用单轮噪声
// 污染用户画像。
// ============================================================

// RuleCandidate 是规则筛出的一条候选。
type RuleCandidate struct {
	Field      string
	Rule       string
	Quote      string
	Confidence float64
}

// ExtractRuleCandidates 用关键词规则筛出可能包含长期记忆的用户消息。
// 返回空表示这条消息不需要进入候选队列。
func ExtractRuleCandidates(query string) []RuleCandidate {
	q := strings.TrimSpace(query)
	if utf8.RuneCountInString(q) < 4 {
		return nil
	}

	out := make([]RuleCandidate, 0, 3)
	seen := make(map[string]bool)
	add := func(field, rule string, confidence float64) {
		if seen[field] {
			return
		}
		seen[field] = true
		out = append(out, RuleCandidate{
			Field: field, Rule: rule, Quote: q, Confidence: confidence,
		})
	}

	if containsAny(q, []string{
		"我想找", "我想做", "目标岗位", "目标职位", "准备投", "想投", "投递",
		"求职方向", "想找一份", "目标是", "想去做",
	}) {
		add("goal", "goal_keyword", 0.7)
	}
	if containsAny(q, []string{
		"只考虑", "不接受", "不考虑", "工作地点", "想在", "base", "薪资",
		"远程", "实习", "全职", "到岗时间",
	}) {
		add("constraint", "constraint_keyword", 0.7)
	}
	if containsAny(q, []string{
		"我是", "我有", "我学", "我做过", "我擅长", "毕业于", "年经验",
		"专业是", "目前在做", "工作经历",
	}) {
		add("background", "background_keyword", 0.6)
	}
	if containsAny(q, []string{
		"回答简短", "简短点", "别用表格", "用中文", "详细一点", "直接说",
		"不要展开", "先说结论",
	}) {
		add("preference", "preference_keyword", 0.7)
	}
	if containsAny(q, []string{
		"我说错了", "更正", "搞错了", "重新说", "我改一下", "不是这个意思",
	}) {
		add("correction", "correction_keyword", 0.8)
	}
	if containsAny(q, []string{"记住", "以后都", "下次", "帮我记"}) {
		add("explicit_memory", "explicit_memory", 0.9)
	}
	if containsAny(q, []string{
		"我面过", "被问过", "挂了", "弱项", "面试官", "二面", "三面",
	}) {
		add("interview_state", "interview_keyword", 0.6)
	}
	return out
}

// CaptureCandidatesAsync 把规则候选异步写入 MySQL。
// 失败只告警，不影响主回答；value_hash 保证同一条消息同一字段只留一条。
func CaptureCandidatesAsync(userID, sessionID string, messageID uint, query string) {
	if userID == "" || messageID == 0 {
		return
	}
	candidates := ExtractRuleCandidates(query)
	if len(candidates) == 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("[agent-profile] capture candidates panic: %v", r)
			}
		}()
		for _, c := range candidates {
			rec := &models.ProfileCandidate{
				UserID:     userID,
				SessionID:  sessionID,
				MessageID:  messageID,
				Field:      c.Field,
				Value:      "", // L2 从 quote 中抽取结构化值
				ValueHash:  candidateHash(userID, messageID, c.Field, c.Quote),
				Confidence: c.Confidence,
				Quote:      c.Quote,
				Rule:       c.Rule,
				Status:     "pending",
			}
			if err := mysqlrepo.CreateProfileCandidate(rec); err != nil {
				logrus.Warnf("[agent-profile] create candidate failed user=%s field=%s err=%v",
					userID, c.Field, err)
			}
		}
	}()
}

func candidateHash(userID string, messageID uint, field, quote string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%s", userID, messageID, field, quote)))
	return hex.EncodeToString(sum[:16])
}

func containsAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
