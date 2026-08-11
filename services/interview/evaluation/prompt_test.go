package evaluation

import (
	"strings"
	"testing"
)

// ============================================================
// 面试 prompt 构建测试: 评分/出题/追问 + clipText 边界
// ============================================================

func TestBuildScorePromptMessages(t *testing.T) {
	msgs := buildScorePromptMessages("什么是进程", "进程是程序的执行实例", "简历：精通操作系统")
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "评分官") {
		t.Errorf("system prompt 异常: %q", msgs[0].Content)
	}
	if msgs[1].Role != "user" {
		t.Errorf("user role 异常: %q", msgs[1].Role)
	}
	for _, want := range []string{"什么是进程", "进程是程序的执行实例", "简历：精通操作系统", `"score":0`} {
		if !strings.Contains(msgs[1].Content, want) {
			t.Errorf("user prompt 缺少 %q", want)
		}
	}
}

func TestBuildScorePromptMessages_ClipResume(t *testing.T) {
	// 简历上下文超长(12000 字节)应被 clipText 截断, 不能完整压入 prompt
	longResume := strings.Repeat("简历内容", 1000) // 1000 × 12B = 12000B
	msgs := buildScorePromptMessages("题面", "答案", longResume)
	if len(msgs[1].Content) >= 12000 {
		t.Errorf("超长简历未被截断: prompt len = %d, want < 12000", len(msgs[1].Content))
	}
}

func TestBuildExtractionPromptMessages(t *testing.T) {
	msgs := buildExtractionPromptMessages("简历：3 年 Go 后端经验")
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "出题官") {
		t.Errorf("system prompt 异常: %q", msgs[0].Content)
	}
	for _, want := range []string{"简历：3 年 Go 后端经验", `"questions":["题1","题2"]`, `"resumeScore":85`} {
		if !strings.Contains(msgs[1].Content, want) {
			t.Errorf("user prompt 缺少 %q", want)
		}
	}
}

func TestBuildFollowUpPromptMessages(t *testing.T) {
	// 有缺失点
	msgs := buildFollowUpPromptMessages("题面", "答案", []string{"缺A", "缺B"}, 1, 2)
	if !strings.Contains(msgs[1].Content, "缺A; 缺B") {
		t.Errorf("缺失点未 join: %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "第 2 轮追问") {
		t.Errorf("追问轮次应为 followUpCount+1: %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "最多追问 2 轮") {
		t.Errorf("追问上限未写入 prompt: %q", msgs[1].Content)
	}

	// 无缺失点 → 兜底"无"
	msgs2 := buildFollowUpPromptMessages("题面", "答案", nil, 0, 2)
	if !strings.Contains(msgs2[1].Content, "--- 答案缺失点 ---\n无\n--- 缺失点结束 ---") {
		t.Errorf("缺失点为空应兜底'无': %q", msgs2[1].Content)
	}
	if !strings.Contains(msgs2[1].Content, "第 1 轮追问") {
		t.Errorf("第一轮追问轮次应为 1: %q", msgs2[1].Content)
	}
}

func TestClipText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"空文本", "", 100, ""},
		{"纯空白", "   \n\t  ", 100, ""},
		{"压缩连续空白", "a  b\nc\td", 100, "a b c d"},
		{"长度未超限", "hello", 100, "hello"},
		{"正好等于上限", "hello", 5, "hello"},
		{"超限截断", "hello world", 5, "hello"},
		{"首尾空白裁剪", "  hi  ", 100, "hi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clipText(tt.input, tt.maxLen); got != tt.want {
				t.Errorf("clipText(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
