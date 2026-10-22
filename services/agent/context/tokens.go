package agentctx

import (
	"math"
	"strings"
	"unicode/utf8"
)

// ============================================================
// Token 估算
//
// 这里不引入 tiktoken 依赖：DeepSeek/OpenAI 的 tokenizer 与本地库不完全一致，
// 先用保守启发式估算，再乘安全系数。接口独立，后续可替换。
//
// 经验值:
//   - CJK 字符约 1 token/字
//   - ASCII/其他约 4 字符/token
//   - 每条消息额外 4 token（role/分隔符）
// ============================================================

const tokenSafetyFactor = 1.15

// EstimateTokens 估算一段文本的 token 数。
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	var cjk, other int
	for _, r := range text {
		if isCJK(r) {
			cjk++
		} else {
			other++
		}
	}
	raw := float64(cjk) + float64(other)/4.0
	return int(math.Ceil(raw * tokenSafetyFactor))
}

// EstimateMessageTokens 估算一条消息的 token 数（含角色开销）。
func EstimateMessageTokens(role, content string) int {
	return EstimateTokens(role) + EstimateTokens(content) + 4
}

// EstimateMessagesTokens 估算一组消息的 token 数。
func EstimateMessagesTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += EstimateMessageTokens(m.Role, m.Content)
	}
	return total
}

// truncateToTokens 按估算 token 截断文本，保证不切坏 UTF-8。
func truncateToTokens(text string, maxTokens int) string {
	if maxTokens <= 0 || text == "" {
		return ""
	}
	if EstimateTokens(text) <= maxTokens {
		return text
	}
	// 按 rune 逐字累计，最后留一点余量给省略号。
	budget := float64(maxTokens) / tokenSafetyFactor
	var b strings.Builder
	var used float64
	for _, r := range text {
		cost := 1.0
		if !isCJK(r) {
			cost = 0.25
		}
		if used+cost > budget {
			break
		}
		b.WriteRune(r)
		used += cost
	}
	if b.Len() == 0 {
		// 极端情况：预算太小，至少返回一个 rune，避免空内容。
		r, _ := utf8.DecodeRuneInString(text)
		return string(r) + "…"
	}
	return b.String() + "…"
}

func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
		return true
	case r >= 0xF900 && r <= 0xFAFF: // CJK Compatibility Ideographs
		return true
	case r >= 0x3040 && r <= 0x30FF: // Hiragana/Katakana
		return true
	case r >= 0xAC00 && r <= 0xD7AF: // Hangul
		return true
	default:
		return false
	}
}
