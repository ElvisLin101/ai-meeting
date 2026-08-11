package evaluation

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExtractContent(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "openai choices message envelope",
			raw:  `{"choices":[{"message":{"content":"答案内容"}}]}`,
			want: "答案内容",
		},
		{
			name: "openai choices delta envelope",
			raw:  `{"choices":[{"delta":{"content":"流式内容"}}]}`,
			want: "流式内容",
		},
		{
			name: "top-level content",
			raw:  `{"content":"顶层内容"}`,
			want: "顶层内容",
		},
		{
			name: "plain non-json text",
			raw:  "这是一段普通文本",
			want: "这是一段普通文本",
		},
		{
			name: "blank input",
			raw:  "  ",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractContent(tt.raw); got != tt.want {
				t.Errorf("ExtractContent(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseObject(t *testing.T) {
	tests := []struct {
		name string
		text string
		want map[string]interface{}
	}{
		{
			name: "plain json",
			text: `{"score": 80, "feedback": "很好"}`,
			want: map[string]interface{}{"score": float64(80), "feedback": "很好"},
		},
		{
			name: "markdown code fence",
			text: "```json\n{\"score\": 90}\n```",
			want: map[string]interface{}{"score": float64(90)},
		},
		{
			name: "json wrapped in prose",
			text: "回答如下：{\"score\": 70, \"feedback\": \"ok\"} 以上。",
			want: map[string]interface{}{"score": float64(70), "feedback": "ok"},
		},
		{
			name: "json field unwrap",
			text: `{"json": {"score": 66}}`,
			want: map[string]interface{}{"score": float64(66)},
		},
		{
			name: "invalid text",
			text: "不是 JSON",
			want: nil,
		},
		{
			name: "empty input",
			text: "",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseObject(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseObject(%q) = %#v, want %#v", tt.text, got, tt.want)
			}
		})
	}
}

func TestExtractStructuredResult(t *testing.T) {
	// AI 返回 OpenAI 包络 + markdown 围栏 + 嵌套 result 对象
	content := "```json\n" + `{"result": {"score": 88, "missing_points": ["a", "b"]}}` + "\n```"
	env, err := json.Marshal(map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{"content": content},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal envelope failed: %v", err)
	}

	got := ExtractStructuredResult(string(env), "score")
	if got == nil {
		t.Fatal("ExtractStructuredResult returned nil")
	}
	if score, ok := got["score"].(float64); !ok || score != 88 {
		t.Errorf("score = %v, want 88", got["score"])
	}
}

func TestParseScore(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want int
	}{
		{"float", float64(88.4), 88},
		{"float round up", float64(88.6), 89},
		{"int", int(75), 75},
		{"numeric string", "92", 92},
		{"clamp high", float64(150), 100},
		{"clamp low", float64(-10), 0},
		{"nil", nil, 0},
		{"non numeric", "abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseScore(tt.val); got != tt.want {
				t.Errorf("ParseScore(%v) = %d, want %d", tt.val, got, tt.want)
			}
		})
	}
}

func TestAsStringList(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want []string
	}{
		{"interface array", []interface{}{"a", " b "}, []string{"a", "b"}},
		{"string array", []string{"a", "b"}, []string{"a", "b"}},
		{"json array string", `["a","b"]`, []string{"a", "b"}},
		{"comma separated", "a,b，c;d\ne", []string{"a", "b", "c", "d", "e"}},
		{"single value", "a", []string{"a"}},
		{"nil", nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AsStringList(tt.val); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsStringList(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestAsBoolean(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want bool
	}{
		{"bool true", true, true},
		{"string true", "true", true},
		{"string 1", "1", true},
		{"string yes", "YES", true},
		{"string false", "false", false},
		{"float nonzero", float64(1), true},
		{"float zero", float64(0), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AsBoolean(tt.val); got != tt.want {
				t.Errorf("AsBoolean(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}
