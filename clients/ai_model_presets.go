package clients

// AIModelPreset 预设模型模板
type AIModelPreset struct {
	Provider    string `json:"provider"`     // 提供商标识: deepseek/doubao/glm/custom
	DisplayName string `json:"display_name"` // 显示名称
	Endpoint    string `json:"endpoint"`     // 默认 API 地址
	ModelType   string `json:"model_type"`   // 默认模型名
	DocURL      string `json:"doc_url"`      // API Key 申请地址
	NeedsApiKey bool   `json:"needs_api_key"`
	NeedsSecret bool   `json:"needs_secret"` // 是否需要 api_secret
	ConfigHint  string `json:"config_hint"`  // 配置 JSON 示例提示
}

// PresetModels 预设模型模板列表
var PresetModels = []AIModelPreset{
	{
		Provider:    "deepseek",
		DisplayName: "DeepSeek",
		Endpoint:    "https://api.deepseek.com/chat/completions",
		ModelType:   "deepseek-chat",
		DocURL:      "https://platform.deepseek.com/api_keys",
		NeedsApiKey: true,
		NeedsSecret: false,
		ConfigHint:  `{"temperature":0.7,"max_tokens":4096}`,
	},
	{
		Provider:    "doubao",
		DisplayName: "豆包 (字节跳动)",
		Endpoint:    "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
		ModelType:   "doubao-pro-32k",
		DocURL:      "https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey",
		NeedsApiKey: true,
		NeedsSecret: false,
		ConfigHint:  `{"temperature":0.7,"max_tokens":4096}`,
	},
	{
		Provider:    "glm",
		DisplayName: "GLM (智谱AI)",
		Endpoint:    "https://open.bigmodel.cn/api/paas/v4/chat/completions",
		ModelType:   "glm-4",
		DocURL:      "https://open.bigmodel.cn/usercenter/apikeys",
		NeedsApiKey: true,
		NeedsSecret: false,
		ConfigHint:  `{"temperature":0.7,"max_tokens":4096}`,
	},
	{
		Provider:    "qwen",
		DisplayName: "通义千问 (阿里云)",
		Endpoint:    "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		ModelType:   "qwen-plus",
		DocURL:      "https://dashscope.console.aliyun.com/apiKey",
		NeedsApiKey: true,
		NeedsSecret: false,
		ConfigHint:  `{"temperature":0.7,"max_tokens":4096}`,
	},
	{
		Provider:    "moonshot",
		DisplayName: "Moonshot (Kimi)",
		Endpoint:    "https://api.moonshot.cn/v1/chat/completions",
		ModelType:   "moonshot-v1-8k",
		DocURL:      "https://platform.moonshot.cn/console/api-keys",
		NeedsApiKey: true,
		NeedsSecret: false,
		ConfigHint:  `{"temperature":0.7,"max_tokens":4096}`,
	},
	{
		Provider:    "openai",
		DisplayName: "OpenAI",
		Endpoint:    "https://api.openai.com/v1/chat/completions",
		ModelType:   "gpt-4o-mini",
		DocURL:      "https://platform.openai.com/api-keys",
		NeedsApiKey: true,
		NeedsSecret: false,
		ConfigHint:  `{"temperature":0.7,"max_tokens":4096}`,
	},
	{
		Provider:    "custom",
		DisplayName: "自定义模型",
		Endpoint:    "",
		ModelType:   "",
		DocURL:      "",
		NeedsApiKey: true,
		NeedsSecret: false,
		ConfigHint:  `{"temperature":0.7,"max_tokens":4096}`,
	},
}

// GetPresetByProvider 按 provider 标识查找预设模板
func GetPresetByProvider(provider string) *AIModelPreset {
	for i := range PresetModels {
		if PresetModels[i].Provider == provider {
			return &PresetModels[i]
		}
	}
	return nil
}
