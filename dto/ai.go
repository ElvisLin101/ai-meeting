package dto

type AiSessionCreateReqDTO struct {
	AiId        uint   `json:"ai_id"`
	FirstMessage string `json:"first_message"`
}

type AiSessionCreateRespDTO struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
}

type AiConversationPageReqDTO struct {
	Page    int `form:"page" default:"1"`
	Size    int `form:"size" default:"10"`
}

type AiConversationRespDTO struct {
	SessionID   string `json:"session_id"`
	AiId        uint   `json:"ai_id"`
	Title       string `json:"title"`
	MessageCnt  int    `json:"message_cnt"`
	Status      int    `json:"status"`
	UpdatedTime string `json:"updated_time"`
}

type AiMessageReqDTO struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content" binding:"required"`
}

type AiMessageHistoryRespDTO struct {
	MessageID string `json:"message_id,omitempty"`
	SessionID string `json:"session_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Sequence  int    `json:"sequence"`
	CreatedAt string `json:"created_at"`
}

type AiChatRespDTO struct {
	SessionID          string `json:"session_id"`
	UserMessageID      string `json:"user_message_id,omitempty"`
	AssistantMessageID string `json:"assistant_message_id,omitempty"`
	Content            string `json:"content"`
}

type AiPropertiesCreateReqDTO struct {
	Name      string `json:"name" binding:"required"`
	ModelType string `json:"model_type"`
	ApiKey    string `json:"api_key"`
	ApiSecret string `json:"api_secret"`
	Endpoint  string `json:"endpoint"`
	Config    string `json:"config"`
}

// AiPropertiesCreateFromPresetReqDTO 按预设模板创建（用户只需填 apiKey）
type AiPropertiesCreateFromPresetReqDTO struct {
	Provider string `json:"provider" binding:"required"` // deepseek/doubao/glm/qwen/moonshot/openai/custom
	Name     string `json:"name" binding:"required"`     // 用户自定义名称
	ApiKey   string `json:"api_key" binding:"required"`  // 用户填入的 API Key
	ApiSecret string `json:"api_secret"`                 // 可选
	ModelType string `json:"model_type"`                 // 可选，不填用预设默认
	Endpoint  string `json:"endpoint"`                   // 可选，不填用预设默认
	Config    string `json:"config"`                     // 可选，配置 JSON
}

type AiPropertiesUpdateReqDTO struct {
	ID        uint   `json:"id" binding:"required"`
	Name      string `json:"name"`
	ModelType string `json:"model_type"`
	ApiKey    string `json:"api_key"`
	ApiSecret string `json:"api_secret"`
	Endpoint  string `json:"endpoint"`
	Config    string `json:"config"`
}

type AiPropertiesPageReqDTO struct {
	Page    int `form:"page" default:"1"`
	Size    int `form:"size" default:"10"`
}

type AiPropertiesRespDTO struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	ModelType string `json:"model_type"`
	Endpoint  string `json:"endpoint"`
	IsEnabled bool   `json:"is_enabled"`
	CreatedAt string `json:"created_at"`
}

type AiModelOptionRespDTO struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type MemoryThresholdReqDTO struct {
	Threshold int `json:"threshold" binding:"required"`
}

type MemoryThresholdRespDTO struct {
	Threshold     int `json:"threshold"`
	MinThreshold  int `json:"min_threshold"`
	MaxThreshold  int `json:"max_threshold"`
	TriggerOffset int `json:"trigger_offset"`
}
