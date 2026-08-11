package agent

import (
	"strings"
	"testing"

	"ai-meeting/models"
)

// ============================================================
// Agent 对话 prompt 构建测试
// ============================================================

func TestBuildAgentPromptMessages(t *testing.T) {
	props := &models.AgentProperties{Description: "后端面试官"}
	// history 最后一条是当前用户消息(调用方传入完整历史, 函数内部排除)
	history := []models.AgentMessage{
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "请讲"},
		{Role: "user", Content: "什么是进程"},
	}

	msgs := buildAgentPromptMessages(props, history, "什么是进程")
	// system 1 + 历史 2(排除最后一条)+ 当前 user 1 = 4
	if len(msgs) != 4 {
		t.Fatalf("len = %d, want 4(最后一条历史应被排除)", len(msgs))
	}

	// system 含智能体描述
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "后端面试官") {
		t.Errorf("system prompt 异常: %q", msgs[0].Content)
	}
	// 历史按顺序保留
	if msgs[1].Role != "user" || msgs[1].Content != "你好" {
		t.Errorf("历史第1条异常: %+v", msgs[1])
	}
	if msgs[2].Role != "assistant" || msgs[2].Content != "请讲" {
		t.Errorf("历史第2条异常: %+v", msgs[2])
	}
	// 当前用户消息在最后
	if msgs[3].Role != "user" || msgs[3].Content != "什么是进程" {
		t.Errorf("当前消息异常: %+v", msgs[3])
	}
}

func TestBuildAgentPromptMessages_EmptyHistory(t *testing.T) {
	props := &models.AgentProperties{Description: "助手"}
	msgs := buildAgentPromptMessages(props, nil, "第一次提问")
	if len(msgs) != 2 {
		t.Fatalf("无历史时 len = %d, want 2", len(msgs))
	}
	if msgs[1].Role != "user" || msgs[1].Content != "第一次提问" {
		t.Errorf("当前消息异常: %+v", msgs[1])
	}
}
