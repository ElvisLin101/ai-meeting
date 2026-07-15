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
	ID        uint   `json:"id"`
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
	ModelType string `json:"model_type" binding:"required"`
	ApiKey    string `json:"api_key"`
	ApiSecret string `json:"api_secret"`
	Endpoint  string `json:"endpoint"`
	Config    string `json:"config"`
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
