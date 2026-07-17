package dto

type AgentSessionCreateReqDTO struct {
	FirstMessage string `json:"first_message"`
}

type AgentSessionCreateRespDTO struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
}

type AgentConversationPageReqDTO struct {
	Page int `form:"page" default:"1"`
	Size int `form:"size" default:"10"`
}

type AgentConversationRespDTO struct {
	SessionID   string `json:"session_id"`
	Title       string `json:"title"`
	MessageCnt  int    `json:"message_cnt"`
	Status      int    `json:"status"`
	UpdatedTime string `json:"updated_time"`
}

type AgentMessageHistoryRespDTO struct {
	ID        uint   `json:"id"`
	MessageID string `json:"message_id,omitempty"`
	SessionID string `json:"session_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Sequence  int    `json:"sequence"`
	CreatedAt string `json:"created_at"`
}

type UserMessageReqDTO struct {
	SessionID string `json:"session_id"`
	UserName  string `json:"user_name"`
	Content   string `json:"content" binding:"required"`
}

type AgentPropertiesReqDTO struct {
	ID          uint   `json:"id"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Config      string `json:"config"`
}

type AgentPropertiesRespDTO struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Config      string `json:"config"`
	IsEnabled   bool   `json:"is_enabled"`
	CreatedAt   string `json:"created_at"`
}

type AgentFileUploadRespDTO struct {
	FileID   string `json:"file_id"`
	Filename string `json:"filename"`
	Path     string `json:"path"`
}
