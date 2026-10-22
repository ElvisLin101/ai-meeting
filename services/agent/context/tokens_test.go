package agentctx

import (
	"strings"
	"testing"
	"unicode/utf8"

	"ai-meeting/models"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("empty text should be 0, got %d", got)
	}
	cjk := EstimateTokens("你好世界")
	ascii := EstimateTokens("hello world")
	if cjk <= 0 || ascii <= 0 {
		t.Fatalf("tokens should be positive: cjk=%d ascii=%d", cjk, ascii)
	}
	if cjk <= ascii {
		t.Fatalf("CJK should cost more tokens than same-length ASCII: cjk=%d ascii=%d", cjk, ascii)
	}
}

func TestTruncateToTokensKeepsValidUTF8(t *testing.T) {
	text := strings.Repeat("上下文管理", 100)
	out := truncateToTokens(text, 50)
	if out == "" {
		t.Fatal("truncated text should not be empty")
	}
	if !utf8.ValidString(out) {
		t.Fatal("truncated text must be valid UTF-8")
	}
	if EstimateTokens(out) > 50+10 { // 允许省略号和估算误差
		t.Fatalf("truncated text too large: %d tokens", EstimateTokens(out))
	}
}

func TestSelectRecentByTokensPicksNewestFirst(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: strings.Repeat("旧", 200)},
		{Role: "assistant", Content: strings.Repeat("中", 200)},
		{Role: "user", Content: strings.Repeat("新", 50)},
	}
	picked, dropped := selectRecentByTokens(messages, 200)
	if len(picked) != 1 {
		t.Fatalf("want only newest message, got %d (dropped=%d)", len(picked), dropped)
	}
	if picked[0].Content != messages[2].Content {
		t.Fatal("newest message should be selected")
	}
	if dropped != 2 {
		t.Fatalf("want 2 dropped, got %d", dropped)
	}
}

func TestSelectRecentByTokensTruncatesOversizedNewest(t *testing.T) {
	messages := []Message{{Role: "user", Content: strings.Repeat("长", 1000)}}
	picked, dropped := selectRecentByTokens(messages, 100)
	if len(picked) != 1 || dropped != 0 {
		t.Fatalf("oversized newest should be truncated and kept, got picked=%d dropped=%d", len(picked), dropped)
	}
	if EstimateTokens(picked[0].Content) > 110 {
		t.Fatalf("truncated newest still too large: %d", EstimateTokens(picked[0].Content))
	}
}

func TestSplitConversationByTokens(t *testing.T) {
	messages := []models.Message{ // 倒序：最新在前
		{Sequence: 5, Role: "assistant", Content: strings.Repeat("新", 100)},
		{Sequence: 4, Role: "user", Content: strings.Repeat("中", 100)},
		{Sequence: 3, Role: "assistant", Content: strings.Repeat("旧", 100)},
	}
	old, tail := splitConversationByTokens(messages, 200)
	if len(tail) != 1 || tail[0].Sequence != 5 {
		t.Fatalf("tail should contain newest message only, got %+v", tail)
	}
	if len(old) != 2 || old[0].Sequence != 4 {
		t.Fatalf("old should contain the rest, got %+v", old)
	}

	old, tail = splitConversationByTokens(messages, 0)
	if len(tail) != 0 || len(old) != 3 {
		t.Fatalf("keep=0 should compress all, old=%d tail=%d", len(old), len(tail))
	}

	old, tail = splitConversationByTokens(messages, 100000)
	if len(old) != 0 || len(tail) != 3 {
		t.Fatalf("large keep should retain all, old=%d tail=%d", len(old), len(tail))
	}
}
