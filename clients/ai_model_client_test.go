package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-meeting/config"
	"ai-meeting/models"
)

// ============================================================
// AI 模型客户端测试: 响应解析 / 请求构造 / DeepSeek 配置加载 / httptest 全链路
// ============================================================

func TestParseAIChatResponse(t *testing.T) {
	// 标准 message.content
	content, err := parseAIChatResponse([]byte(`{"choices":[{"message":{"content":"你好"}}]}`))
	if err != nil || content != "你好" {
		t.Errorf("content 分支 = %q, %v", content, err)
	}

	// 兼容 text 字段(部分 provider)
	content, err = parseAIChatResponse([]byte(`{"choices":[{"text":"hi"}]}`))
	if err != nil || content != "hi" {
		t.Errorf("text 分支 = %q, %v", content, err)
	}

	// choices 为空
	_, err = parseAIChatResponse([]byte(`{"choices":[]}`))
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Errorf("空 choices 应报错, got %v", err)
	}

	// content 和 text 均空
	_, err = parseAIChatResponse([]byte(`{"choices":[{"message":{"content":"  "}}]}`))
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("空内容应报错, got %v", err)
	}

	// 非法 JSON
	if _, err = parseAIChatResponse([]byte(`not-json`)); err == nil {
		t.Error("非法 JSON 应报错")
	}
}

func TestParseAIChatStreamChunk(t *testing.T) {
	// delta.content
	chunk, err := parseAIChatStreamChunk([]byte(`{"choices":[{"delta":{"content":"中"}}]}`))
	if err != nil || chunk.Content != "中" {
		t.Errorf("delta.content 分支 = %+v, %v", chunk, err)
	}

	// reasoning_content 透传
	chunk, err = parseAIChatStreamChunk([]byte(`{"choices":[{"delta":{"reasoning_content":"思考中"}}]}`))
	if err != nil || chunk.ReasoningContent != "思考中" {
		t.Errorf("reasoning 分支 = %+v, %v", chunk, err)
	}

	// text 兜底
	chunk, err = parseAIChatStreamChunk([]byte(`{"choices":[{"text":"t"}]}`))
	if err != nil || chunk.Content != "t" {
		t.Errorf("text 兜底 = %+v, %v", chunk, err)
	}

	// choices 为空 → 空 chunk 不报错
	chunk, err = parseAIChatStreamChunk([]byte(`{"choices":[]}`))
	if err != nil || chunk.Content != "" {
		t.Errorf("空 choices = %+v, %v", chunk, err)
	}

	// 非法 JSON
	if _, err = parseAIChatStreamChunk([]byte(`bad`)); err == nil {
		t.Error("非法 JSON 应报错")
	}
}

func TestResolveAiModelName(t *testing.T) {
	tests := []struct {
		name string
		prop *models.AiProperties
		want string
	}{
		{"ModelType 优先", &models.AiProperties{Name: "deepseek", ModelType: "deepseek-chat"}, "deepseek-chat"},
		{"Config.model 覆盖", &models.AiProperties{Name: "deepseek", ModelType: "deepseek-chat", Config: `{"model":"deepseek-reasoner"}`}, "deepseek-reasoner"},
		{"Config 非法 JSON 忽略", &models.AiProperties{Name: "deepseek", ModelType: "deepseek-chat", Config: `not-json`}, "deepseek-chat"},
		{"Config.model 空串回退", &models.AiProperties{Name: "deepseek", ModelType: "deepseek-chat", Config: `{"model":"  "}`}, "deepseek-chat"},
		{"ModelType 空用 Name", &models.AiProperties{Name: "deepseek", ModelType: ""}, "deepseek"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAiModelName(tt.prop); got != tt.want {
				t.Errorf("resolveAiModelName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewAIChatRequest(t *testing.T) {
	ctx := context.Background()
	prop := &models.AiProperties{
		Endpoint:  "https://example.com/chat",
		ApiKey:    "key-123",
		ApiSecret: "secret-456",
	}
	req, err := newAIChatRequest(ctx, prop, map[string]interface{}{"model": "m", "stream": false})
	if err != nil {
		t.Fatalf("newAIChatRequest failed: %v", err)
	}
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if req.URL.String() != "https://example.com/chat" {
		t.Errorf("url = %s", req.URL.String())
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer key-123" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("X-API-Secret"); got != "secret-456" {
		t.Errorf("X-API-Secret = %q", got)
	}

	// 无 ApiKey 时不应设置 Authorization
	req2, err := newAIChatRequest(ctx, &models.AiProperties{Endpoint: "https://e.com"}, map[string]interface{}{})
	if err != nil {
		t.Fatalf("newAIChatRequest failed: %v", err)
	}
	if got := req2.Header.Get("Authorization"); got != "" {
		t.Errorf("无 ApiKey 不应设置 Authorization, got %q", got)
	}
	// body 应为合法 JSON
	var body map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Errorf("body 非法 JSON: %v", err)
	}
	if body["model"] != "m" {
		t.Errorf("body.model = %v, want m", body["model"])
	}
}

// withTestDeepSeekConfig 设置 config fallback(aiID=0 走 DeepSeek 配置, 不碰 DB), 返回恢复函数
func withTestDeepSeekConfig(t *testing.T, endpoint, apiKey string) func() {
	t.Helper()
	orig := config.AppConfig.AI
	config.AppConfig.AI = config.AIConfig{
		Provider: "deepseek",
		DeepSeek: config.DeepSeekConfig{
			Enabled:  true,
			Endpoint: endpoint,
			Model:    "deepseek-chat",
			APIKey:   apiKey,
		},
	}
	return func() { config.AppConfig.AI = orig }
}

func TestLoadConfiguredDeepSeekProperty(t *testing.T) {
	t.Run("provider 非 deepseek", func(t *testing.T) {
		orig := config.AppConfig.AI
		config.AppConfig.AI.Provider = "glm"
		defer func() { config.AppConfig.AI = orig }()
		if prop, ok := loadConfiguredDeepSeekProperty(); ok || prop != nil {
			t.Errorf("provider 非 deepseek 应返回 (nil,false), got %+v", prop)
		}
	})

	t.Run("未启用", func(t *testing.T) {
		orig := config.AppConfig.AI
		config.AppConfig.AI.Provider = "deepseek"
		config.AppConfig.AI.DeepSeek.Enabled = false
		config.AppConfig.AI.DeepSeek.APIKey = "key"
		defer func() { config.AppConfig.AI = orig }()
		if _, ok := loadConfiguredDeepSeekProperty(); ok {
			t.Error("未启用应返回 false")
		}
	})

	t.Run("无 APIKey", func(t *testing.T) {
		orig := config.AppConfig.AI
		config.AppConfig.AI.Provider = "deepseek"
		config.AppConfig.AI.DeepSeek.Enabled = true
		config.AppConfig.AI.DeepSeek.APIKey = "  "
		defer func() { config.AppConfig.AI = orig }()
		if _, ok := loadConfiguredDeepSeekProperty(); ok {
			t.Error("无 APIKey 应返回 false")
		}
	})

	t.Run("默认 endpoint/model 兜底", func(t *testing.T) {
		restore := withTestDeepSeekConfig(t, "", "key-1")
		defer restore()
		prop, ok := loadConfiguredDeepSeekProperty()
		if !ok || prop == nil {
			t.Fatal("应加载成功")
		}
		if prop.Endpoint != "https://api.deepseek.com/chat/completions" {
			t.Errorf("endpoint = %q, want 默认值", prop.Endpoint)
		}
		if prop.ModelType != "deepseek-chat" {
			t.Errorf("model = %q, want 默认值", prop.ModelType)
		}
		if !prop.IsEnabled || prop.ApiKey != "key-1" {
			t.Errorf("prop = %+v", prop)
		}
	})

	t.Run("自定义 endpoint/model", func(t *testing.T) {
		restore := withTestDeepSeekConfig(t, "https://custom.example.com/v1", "key-2")
		defer restore()
		prop, ok := loadConfiguredDeepSeekProperty()
		if !ok || prop == nil {
			t.Fatal("应加载成功")
		}
		if prop.Endpoint != "https://custom.example.com/v1" {
			t.Errorf("endpoint = %q", prop.Endpoint)
		}
	})
}

func TestCallConfiguredAIChat_WithConfiguredDeepSeek(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"hi from server"}}]}`))
	}))
	defer srv.Close()

	restore := withTestDeepSeekConfig(t, srv.URL, "test-key")
	defer restore()

	out, err := CallConfiguredAIChat(context.Background(), 0, []PromptMessage{{Role: "user", Content: "hello"}}, 0.7)
	if err != nil {
		t.Fatalf("CallConfiguredAIChat failed: %v", err)
	}
	if out != "hi from server" {
		t.Errorf("out = %q, want hi from server", out)
	}
}

func TestCallConfiguredAIChat_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("upstream error"))
	}))
	defer srv.Close()

	restore := withTestDeepSeekConfig(t, srv.URL, "test-key")
	defer restore()

	_, err := CallConfiguredAIChat(context.Background(), 0, nil, 0.7)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("应返回状态码错误, got %v", err)
	}
}

func TestCallConfiguredAIChat_EmptyEndpoint(t *testing.T) {
	restore := withTestDeepSeekConfig(t, "  ", "test-key")
	defer restore()
	if _, err := CallConfiguredAIChat(context.Background(), 0, nil, 0.7); err == nil {
		t.Error("空 endpoint 应报错")
	}
}

func TestCallConfiguredAIChatStream_SSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	restore := withTestDeepSeekConfig(t, srv.URL, "test-key")
	defer restore()

	var got strings.Builder
	err := CallConfiguredAIChatStream(context.Background(), 0, []PromptMessage{{Role: "user", Content: "hi"}}, 0.7, func(chunk ChatStreamChunk) error {
		got.WriteString(chunk.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("CallConfiguredAIChatStream failed: %v", err)
	}
	if got.String() != "你好" {
		t.Errorf("stream 内容 = %q, want 你好", got.String())
	}
}

func TestCallConfiguredAIChatStream_NilCallback(t *testing.T) {
	err := CallConfiguredAIChatStream(context.Background(), 0, nil, 0.7, nil)
	if err == nil || !strings.Contains(err.Error(), "callback is nil") {
		t.Errorf("nil callback 应报错, got %v", err)
	}
}
