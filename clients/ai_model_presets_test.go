package clients

import "testing"

// ============================================================
// AI 模型预设模板测试
// ============================================================

func TestGetPresetByProvider(t *testing.T) {
	tests := []struct {
		provider    string
		modelType   string
		needsApiKey bool
	}{
		{"deepseek", "deepseek-chat", true},
		{"doubao", "doubao-pro-32k", true},
		{"glm", "glm-4", true},
		{"qwen", "qwen-plus", true},
		{"moonshot", "moonshot-v1-8k", true},
		{"openai", "gpt-4o-mini", true},
		{"custom", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			p := GetPresetByProvider(tt.provider)
			if p == nil {
				t.Fatalf("GetPresetByProvider(%q) = nil", tt.provider)
			}
			if p.Provider != tt.provider {
				t.Errorf("Provider = %q, want %q", p.Provider, tt.provider)
			}
			if p.ModelType != tt.modelType {
				t.Errorf("ModelType = %q, want %q", p.ModelType, tt.modelType)
			}
			if p.NeedsApiKey != tt.needsApiKey {
				t.Errorf("NeedsApiKey = %v, want %v", p.NeedsApiKey, tt.needsApiKey)
			}
			if p.Endpoint == "" && p.Provider != "custom" {
				t.Errorf("非 custom 预设缺少 Endpoint")
			}
		})
	}
}

func TestGetPresetByProvider_Unknown(t *testing.T) {
	if p := GetPresetByProvider("not-exist-provider"); p != nil {
		t.Errorf("未知 provider 应返回 nil, got %+v", p)
	}
}

func TestPresetModels_AllResolvable(t *testing.T) {
	// 每个预设都能通过 GetPresetByProvider 解析回来, 保证 Provider 字段唯一一致
	seen := make(map[string]bool)
	for _, p := range PresetModels {
		if seen[p.Provider] {
			t.Errorf("重复的 provider: %q", p.Provider)
		}
		seen[p.Provider] = true
		if got := GetPresetByProvider(p.Provider); got == nil || got.Provider != p.Provider {
			t.Errorf("GetPresetByProvider(%q) 解析不一致", p.Provider)
		}
	}
}
