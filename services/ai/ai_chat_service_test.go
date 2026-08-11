package ai

import (
	"strings"
	"testing"
)

// ============================================================
// AI 对话 prompt 构建测试
// ============================================================

func TestBuildAiChatPromptMessages(t *testing.T) {
	// 无记忆上下文 → 仅 system + user 两条
	msgs := buildAiChatPromptMessages("", "你好")
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "长期记忆") {
		t.Errorf("system prompt 异常: %q", msgs[0].Content)
	}
	if msgs[1].Role != "user" || msgs[1].Content != "你好" {
		t.Errorf("user 消息应为最后一条: %+v", msgs[1])
	}
}

func TestBuildAiChatPromptMessages_WithMemory(t *testing.T) {
	msgs := buildAiChatPromptMessages("记忆摘要内容", "当前问题")
	if len(msgs) != 3 {
		t.Fatalf("有记忆时 len = %d, want 3", len(msgs))
	}
	// 中间是记忆背景(system)
	if msgs[1].Role != "system" || !strings.Contains(msgs[1].Content, "记忆摘要内容") {
		t.Errorf("记忆背景消息异常: %+v", msgs[1])
	}
	// 用户消息仍在最后
	if msgs[2].Role != "user" || msgs[2].Content != "当前问题" {
		t.Errorf("user 消息异常: %+v", msgs[2])
	}
}

func TestBuildAiChatPromptMessages_BlankMemory(t *testing.T) {
	// 纯空白记忆按"无"处理, 不插入多余消息
	msgs := buildAiChatPromptMessages("   ", "hi")
	if len(msgs) != 2 {
		t.Errorf("空白记忆 len = %d, want 2", len(msgs))
	}
}
