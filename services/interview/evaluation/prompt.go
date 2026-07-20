package evaluation

import (
	"ai-meeting/clients"
	"fmt"
	"strings"
)

// ============================================================
// 面试评分/出题/追问的 prompt 定义
// 全部走 DeepSeek (CallConfiguredAIChat, aiID=0 config fallback)
// prompt 在代码里维护, 不依赖外部平台
// ============================================================

// 评分温度: 偏低求稳定(和 AI 压缩一致)
const scoreTemperature = 0.2
// 出题温度: 略高求多样性
const extractionTemperature = 0.3
// 追问温度: 再略高
const followUpTemperature = 0.4

// buildScorePromptMessages 构建评分 prompt
// 输入: 题面、候选人答案、简历上下文(clip 2000)
func buildScorePromptMessages(questionContent, answerContent, resumeContext string) []clients.PromptMessage {
	system := "你是技术面试评分官。根据题面、候选人答案和简历上下文评分，只输出 JSON，不要输出任何解释或多余文本。"

	user := fmt.Sprintf(`请对候选人的面试回答进行评分。

--- 题面 ---
%s
--- 题面结束 ---

--- 候选人答案 ---
%s
--- 答案结束 ---

--- 简历上下文 ---
%s
--- 简历上下文结束 ---

评分要求：
1. score 必须是 [0,100] 的整数。
2. feedback 简洁实用，指出优缺点。
3. missing_points 是字符串数组，列出答案中缺失或不足的关键点。
4. follow_up_needed 为 true 或 false，表示是否需要追问。
5. 如果 follow_up_needed 为 true，follow_up_question 必须非空。
6. logic_ok 表示答案逻辑是否正确。

输出格式（严格 JSON）：
{"score":0,"feedback":"","follow_up_needed":true,"follow_up_question":"","missing_points":["..."],"logic_ok":true}`,
		questionContent, answerContent, clipText(resumeContext, 2000))

	return []clients.PromptMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// buildExtractionPromptMessages 构建出题 prompt
// 输入: 简历内容(文本)
func buildExtractionPromptMessages(resumeContent string) []clients.PromptMessage {
	system := "你是面试出题官。根据简历内容提取技术面试问题，只输出 JSON，不要输出寒暄、解释或多余文本。"

	user := fmt.Sprintf(`请根据以下简历内容提取面试题。

--- 简历内容 ---
%s
--- 简历内容结束 ---

要求：
1. questions 是字符串数组，包含 3-8 个面试题，按由浅入深排列。
2. suggestions 是字符串数组，包含出题建议。
3. type 是面试方向（如"后端"、"前端"、"算法"等）。
4. resumeScore 是 [0,100] 的整数，表示简历匹配度评分。

输出格式（严格 JSON）：
{"questions":["题1","题2"],"suggestions":["建议1"],"type":"后端","resumeScore":85}`,
		clipText(resumeContent, 4000))

	return []clients.PromptMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// buildFollowUpPromptMessages 构建追问 prompt
// 输入: 题面、答案、缺失点、追问轮次/上限
func buildFollowUpPromptMessages(questionContent, answerContent string, missingPoints []string, followUpCount, maxFollowUp int) []clients.PromptMessage {
	system := "你是面试追问官。根据当前题面、候选人答案和缺失点，生成一个针对性的追问问题。只输出 JSON，不要输出多余文本。"

	missingStr := "无"
	if len(missingPoints) > 0 {
		missingStr = strings.Join(missingPoints, "; ")
	}

	user := fmt.Sprintf(`请根据以下信息生成一个追问问题。

--- 当前题面 ---
%s
--- 题面结束 ---

--- 候选人答案 ---
%s
--- 答案结束 ---

--- 答案缺失点 ---
%s
--- 缺失点结束 ---

当前是第 %d 轮追问，最多追问 %d 轮。

要求：
1. ask_to_user 是一个追问问题，针对答案中的缺失点或不足。
2. end_interview 为 true 时表示应结束面试不再追问。
3. 如果候选人回答已经足够完整，或已达到追问上限，设 end_interview 为 true。

输出格式（严格 JSON）：
{"ask_to_user":"追问问题？","end_interview":false}`,
		questionContent, answerContent, missingStr, followUpCount+1, maxFollowUp)

	return []clients.PromptMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// clipText 截断文本到指定长度，压缩连续空白
func clipText(text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	// 压缩连续空白为单空格
	var b strings.Builder
	prevSpace := false
	for _, r := range text {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	result := b.String()
	if len(result) > maxLen {
		result = result[:maxLen]
	}
	return result
}
