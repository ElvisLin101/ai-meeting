package ai

import (
	"strings"
	"testing"

	"ai-meeting/models"
)

// ============================================================
// AI 记忆压缩纯逻辑测试: 阈值 / 预算窗口 / 兜底摘要 / 格式化 / key 生成
// 直接构造 &AiMemoryService{threshold: N}, 不触发全局 Redis/Mongo 单例
// ============================================================

func TestNormalizeThreshold(t *testing.T) {
	s := &AiMemoryService{threshold: COMPRESSION_THRESHOLD}

	tests := []struct {
		name      string
		threshold int
		want      int
	}{
		{"零值用当前阈值", 0, COMPRESSION_THRESHOLD},
		{"低于下限收敛", 100, MIN_COMPRESSION_THRESHOLD},
		{"高于上限收敛", 999999, MAX_COMPRESSION_THRESHOLD},
		{"等于下限", MIN_COMPRESSION_THRESHOLD, MIN_COMPRESSION_THRESHOLD},
		{"等于上限", MAX_COMPRESSION_THRESHOLD, MAX_COMPRESSION_THRESHOLD},
		{"合法中间值", 8192, 8192},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.normalizeThreshold(tt.threshold); got != tt.want {
				t.Errorf("normalizeThreshold(%d) = %d, want %d", tt.threshold, got, tt.want)
			}
		})
	}
}

func TestSetGetCompressionThreshold(t *testing.T) {
	s := &AiMemoryService{threshold: COMPRESSION_THRESHOLD}

	if got := s.GetCompressionThreshold(); got != COMPRESSION_THRESHOLD {
		t.Errorf("默认阈值 = %d, want %d", got, COMPRESSION_THRESHOLD)
	}

	if err := s.SetCompressionThreshold(2048); err != nil {
		t.Fatalf("SetCompressionThreshold(2048) failed: %v", err)
	}
	if got := s.GetCompressionThreshold(); got != 2048 {
		t.Errorf("阈值 = %d, want 2048", got)
	}

	// 范围校验
	if err := s.SetCompressionThreshold(MIN_COMPRESSION_THRESHOLD - 1); err == nil {
		t.Error("低于下限应报错")
	}
	if err := s.SetCompressionThreshold(MAX_COMPRESSION_THRESHOLD + 1); err == nil {
		t.Error("高于上限应报错")
	}
	// 设置失败后阈值不变
	if got := s.GetCompressionThreshold(); got != 2048 {
		t.Errorf("失败后阈值被污染: %d", got)
	}
}

func TestGetCompressionThresholdConfig(t *testing.T) {
	s := &AiMemoryService{threshold: 2048}
	cur, min, max, offset := s.GetCompressionThresholdConfig()
	if cur != 2048 || min != MIN_COMPRESSION_THRESHOLD || max != MAX_COMPRESSION_THRESHOLD || offset != COMPRESSION_TRIGGER_OFFSET {
		t.Errorf("config = (%d,%d,%d,%d)", cur, min, max, offset)
	}
}

func TestFallbackAiCompressedSummary(t *testing.T) {
	// 短文本(<=900)直接完整返回
	short := "这是一个简短的会话"
	if got := fallbackAiCompressedSummary(short); got != "【AI长期记忆】"+short {
		t.Errorf("短文本 = %q", got)
	}

	// 长文本: 前缀 + 前450字节 + 分隔 + 后450字节
	long := strings.Repeat("a", 1200)
	got := fallbackAiCompressedSummary(long)
	if !strings.HasPrefix(got, "【AI长期记忆】") {
		t.Errorf("缺少前缀: %q", got[:20])
	}
	if !strings.Contains(got, "\n...\n") {
		t.Errorf("缺少分隔符: %q", got[:30])
	}
	if got != "【AI长期记忆】"+strings.Repeat("a", 450)+"\n...\n"+strings.Repeat("a", 450) {
		t.Errorf("长文本截断结果异常, len=%d", len(got))
	}

	// 空文本
	if got := fallbackAiCompressedSummary("   "); got != "【AI长期记忆】" {
		t.Errorf("空文本 = %q", got)
	}
}

func TestFormatAiMessageLine(t *testing.T) {
	if got := formatAiMessageLine(models.AiMessage{Role: "user", Content: "hi"}); got != "user: hi\n" {
		t.Errorf("user 行 = %q", got)
	}
	if got := formatAiMessageLine(models.AiMessage{Role: "assistant", Content: "ok"}); got != "assistant: ok\n" {
		t.Errorf("assistant 行 = %q", got)
	}
	// 未知角色按 assistant 处理
	if got := formatAiMessageLine(models.AiMessage{Role: "system", Content: "x"}); got != "assistant: x\n" {
		t.Errorf("未知角色 = %q", got)
	}
}

func TestAiMessagesLength(t *testing.T) {
	msgs := []models.AiMessage{
		{Role: "user", Content: "hi"},      // "user: hi\n" = 9 字节
		{Role: "assistant", Content: "ok"}, // "assistant: ok\n" = 14 字节
	}
	want := 9 + 14
	if got := aiMessagesLength(msgs); got != want {
		t.Errorf("aiMessagesLength = %d, want %d", got, want)
	}
}

func TestBuildAiChronologicalContext(t *testing.T) {
	// 入参按 Mongo 查询为倒序(新→旧): 第二条在前
	msgs := []models.AiMessage{
		{Role: "assistant", Content: "第二条"},
		{Role: "user", Content: "第一条"},
	}
	got := buildAiChronologicalContext(msgs)
	// 应翻转回时间正序(旧→新): 第一条在前
	if strings.Index(got, "第一条") > strings.Index(got, "第二条") {
		t.Errorf("应按时间正序输出: %q", got)
	}
	if !strings.HasPrefix(got, "user: 第一条\n") || !strings.Contains(got, "assistant: 第二条\n") {
		t.Errorf("内容异常: %q", got)
	}
}

func TestBuildAiCompressionPrompt(t *testing.T) {
	prompt := buildAiCompressionPrompt("要压缩的聊天记录")
	for _, want := range []string{"要压缩的聊天记录", "会话长期记忆摘要", "【AI长期记忆】"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt 缺少 %q", want)
		}
	}
}

func TestBuildContextWithWindow(t *testing.T) {
	s := &AiMemoryService{threshold: COMPRESSION_THRESHOLD}

	// 无摘要无消息
	if got := s.buildContextWithWindow("", nil, 4096); got != "" {
		t.Errorf("空输入 = %q", got)
	}

	// 有摘要 + 消息在预算内 → 摘要头 + 消息(时间正序)
	msgs := []models.AiMessage{
		{Role: "user", Content: "最近的问题"},
		{Role: "assistant", Content: "最近的回答"},
	}
	got := s.buildContextWithWindow("摘要", msgs, 4096)
	for _, want := range []string{"【AI长期记忆摘要】", "摘要", "最近的问题", "最近的回答"} {
		if !strings.Contains(got, want) {
			t.Errorf("结果缺少 %q: %q", want, got)
		}
	}

	// 消息超预算 → 截断, 只保留能装下的
	bigMsgs := []models.AiMessage{
		{Role: "user", Content: strings.Repeat("a", 3000)},
		{Role: "user", Content: strings.Repeat("b", 3000)},
	}
	got = s.buildContextWithWindow("", bigMsgs, 4096)
	// 预算 = 4096 - 500 = 3596, 第一条 3007 可装下, 第二条超预算被丢弃
	if !strings.Contains(got, strings.Repeat("a", 3000)) {
		t.Error("第一条消息应保留")
	}
	if strings.Contains(got, strings.Repeat("b", 3000)) {
		t.Error("第二条消息应被预算截断")
	}

	// 摘要过长导致 windowBudget=0 → 无近期消息
	got = s.buildContextWithWindow(strings.Repeat("s", 5000), msgs, 4096)
	if strings.Contains(got, "最近的问题") {
		t.Error("预算耗尽后不应再包含近期消息")
	}
}

func TestMemoryKeyGeneration(t *testing.T) {
	if got := aiCompressedContextDocumentID("s1"); got != "ai:s1" {
		t.Errorf("documentID = %q", got)
	}
	if got := aiCompressedContextSummaryKey("s1"); got != "memory:ai:s1:summary" {
		t.Errorf("summaryKey = %q", got)
	}
	if got := aiCompressedContextIndexKey("s1"); got != "memory:ai:s1:index" {
		t.Errorf("indexKey = %q", got)
	}
}
