package evaluation

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ============================================================
// AI 返回 JSON 的解析容错（复刻 Java InterviewResponseParser）
//
// 解析流水线:
//   rawResponse
//     → ExtractContent (剥 choices[0].delta/message.content 或顶层 content)
//     → ParseObject
//         → stripMarkdownCodeFence (剥 ```json 围栏)
//         → 直接 json.Unmarshal
//         → 失败则 extractFirstJsonObject (首个{ 到末个})
//         → unwrapJsonField (解 {"json":{...}} 包裹)
//     → ExtractStructuredResult (递归 DFS 找含目标 key 的对象)
// ============================================================

// openAIResponse 用于剥 choices 包络
type openAIResponse struct {
	Choices []struct {
		Delta   struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Content string `json:"content"`
}

// ExtractContent 从 AI 响应中提取内容文本
// 先尝试剥 OpenAI choices 包络，否则取顶层 content，都没有返回原串
func ExtractContent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var resp openAIResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return raw // 不是 JSON 就原样返回
	}

	// choices[0].delta.content 或 choices[0].message.content
	if len(resp.Choices) > 0 {
		if resp.Choices[0].Delta.Content != "" {
			return resp.Choices[0].Delta.Content
		}
		if resp.Choices[0].Message.Content != "" {
			return resp.Choices[0].Message.Content
		}
	}
	// 顶层 content
	if resp.Content != "" {
		return resp.Content
	}
	return raw
}

// markdownFencePattern 匹配 ```json 或 ``` 开头
var markdownFencePattern = regexp.MustCompile("(?s)^```[a-zA-Z]*\\s*")

// ParseObject 从文本中解析出 JSON 对象 map
// 剥 markdown 围栏 → 直接解析 → 失败取首个 JSON 对象 → unwrap json 字段
func ParseObject(text string) map[string]interface{} {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	normalized := stripMarkdownCodeFence(text)

	// 直接解析
	if m := tryParseJsonObject(normalized); m != nil {
		return unwrapJsonField(m)
	}

	// 取首个 { 到末个 } 截断再试
	jsonBody := extractFirstJsonObject(normalized)
	if jsonBody != "" {
		if m := tryParseJsonObject(jsonBody); m != nil {
			return unwrapJsonField(m)
		}
	}

	return nil
}

// stripMarkdownCodeFence 剥 markdown 代码围栏
func stripMarkdownCodeFence(text string) string {
	cleaned := strings.TrimSpace(text)
	if !strings.HasPrefix(cleaned, "```") {
		return cleaned
	}
	// 去开头 ```json / ```
	cleaned = markdownFencePattern.ReplaceAllString(cleaned, "")
	// 去结尾 ```
	cleaned = strings.TrimSuffix(strings.TrimSpace(cleaned), "```")
	return strings.TrimSpace(cleaned)
}

// tryParseJsonObject 尝试 json.Unmarshal 成 map
func tryParseJsonObject(text string) map[string]interface{} {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasPrefix(text, "{") {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return nil
	}
	return m
}

// extractFirstJsonObject 取首个 { 到末个 } 的子串
func extractFirstJsonObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

// unwrapJsonField 如果 map 有 "json" 字段且是对象/字符串，解一层
func unwrapJsonField(m map[string]interface{}) map[string]interface{} {
	if m == nil || len(m) == 0 {
		return nil
	}
	jsonField, ok := m["json"]
	if !ok {
		return m
	}
	switch v := jsonField.(type) {
	case map[string]interface{}:
		return v
	case string:
		if inner := tryParseJsonObject(v); inner != nil {
			return inner
		}
	}
	return m
}

// ExtractStructuredResult 递归 DFS 找含任一 targetKey 的对象
func ExtractStructuredResult(raw string, targetKeys ...string) map[string]interface{} {
	parsed := ParseObject(ExtractContent(raw))
	if matched := findFirstObjectContainingKeys(parsed, targetKeys); matched != nil {
		return matched
	}
	// 对原始串再试一次
	root := ParseObject(raw)
	if matched := findFirstObjectContainingKeys(root, targetKeys); matched != nil {
		return matched
	}
	return parsed
}

// findFirstObjectContainingKeys 递归 DFS：当前 map 含任一 targetKey 就返回
func findFirstObjectContainingKeys(m map[string]interface{}, targetKeys []string) map[string]interface{} {
	if m == nil {
		return nil
	}
	for _, key := range targetKeys {
		if _, ok := m[key]; ok {
			return m
		}
	}
	for _, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			if found := findFirstObjectContainingKeys(val, targetKeys); found != nil {
				return found
			}
		case []interface{}:
			for _, item := range val {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if found := findFirstObjectContainingKeys(itemMap, targetKeys); found != nil {
						return found
					}
				}
			}
		case string:
			if inner := ParseObject(val); inner != nil {
				if found := findFirstObjectContainingKeys(inner, targetKeys); found != nil {
					return found
				}
			}
		}
	}
	return nil
}

// ============================================================
// 容错辅助函数
// ============================================================

// ParseScore 从 interface{} 解析分数，Number/String 都收，round，clamp [0,100]
func ParseScore(val interface{}) int {
	if val == nil {
		return 0
	}
	var score int
	switch v := val.(type) {
	case float64:
		score = int(math.Round(v))
	case int:
		score = v
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		score = int(math.Round(f))
	default:
		return 0
	}
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

// AsStringList 从 interface{} 解析字符串数组
// 支持: List / "[...]"字符串 / 分隔符切分(中英文逗号分号换行) / 单值
func AsStringList(val interface{}) []string {
	if val == nil {
		return []string{}
	}
	switch v := val.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" {
				result = append(result, s)
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(v))
		for _, s := range v {
			s = strings.TrimSpace(s)
			if s != "" {
				result = append(result, s)
			}
		}
		return result
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return []string{}
		}
		// 形如 "[...]" → 当 JSON 数组解析
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			var arr []interface{}
			if err := json.Unmarshal([]byte(s), &arr); err == nil {
				return AsStringList(arr)
			}
		}
		// 含分隔符 → 切分
		if strings.ContainsAny(s, ",;\n，；") {
			parts := strings.FieldsFunc(s, func(r rune) bool {
				return r == ',' || r == ';' || r == '\n' || r == '，' || r == '；'
			})
			result := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					result = append(result, p)
				}
			}
			return result
		}
		return []string{s}
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return []string{}
		}
		return []string{s}
	}
}

// AsBoolean 从 interface{} 解析布尔值
// 支持: true/1/yes 字符串
func AsBoolean(val interface{}) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "true" || s == "1" || s == "yes"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}

// AsString 从 interface{} 解析字符串，nil → ""
func AsString(val interface{}) string {
	if val == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(val))
}

// extractByAliases 按别名优先级从 map 中提取值
func extractByAliases(m map[string]interface{}, aliases ...string) interface{} {
	for _, alias := range aliases {
		if val, ok := m[alias]; ok {
			return val
		}
	}
	return nil
}
