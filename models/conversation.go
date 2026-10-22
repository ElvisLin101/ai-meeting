package models

import "time"

// ============================================================
// 统一会话与消息模型（MySQL 为真相源）
//
// 原 AI/Agent/面试会话分散在 Mongo 多个集合, 改造后统一落 MySQL:
//   conversations 存会话元数据, messages 存全量消息。
// Redis 只做热数据缓存, 丢失后可从这两张表重建。
// ============================================================

// 会话场景
const (
	ConversationSceneAI        = "ai"
	ConversationSceneAgent     = "agent"
	ConversationSceneInterview = "interview"
)

// 会话状态
const (
	ConversationStatusActive = 1
	ConversationStatusEnded  = 0
)

// Conversation 会话元数据
type Conversation struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	SessionID    string    `gorm:"size:64;uniqueIndex;not null" json:"session_id"`
	UserID       string    `gorm:"size:128;index;not null" json:"user_id"`
	Scene        string    `gorm:"size:32;index;not null" json:"scene"` // ai/agent/interview
	AgentID      uint      `gorm:"index;default:0" json:"agent_id"`
	Title        string    `gorm:"size:255" json:"title"`
	Status       int       `gorm:"index;default:1" json:"status"`
	MessageCount int       `gorm:"default:0" json:"message_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (Conversation) TableName() string { return "conversations" }

// Message 全量消息（MySQL 真相源）
// Sequence 为会话内单调递增序号, 由 session_sequences 原子分配。
// Metadata 存结构化扩展(如 tool_calls/tool_call_id), 避免塞进 Content 后无法查询。
type Message struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SessionID string    `gorm:"size:64;not null;uniqueIndex:uk_session_seq,priority:1;index:idx_session_created,priority:1" json:"session_id"`
	UserID    string    `gorm:"size:128;index;not null" json:"user_id"`
	Role      string    `gorm:"size:32;not null" json:"role"` // user/assistant/system/tool
	Content   string    `gorm:"type:longtext" json:"content"`
	Sequence  int64     `gorm:"not null;uniqueIndex:uk_session_seq,priority:2" json:"sequence"`
	Metadata  string    `gorm:"type:longtext" json:"metadata"` // JSON: {"tool_calls":[...],"tool_call_id":"..."}
	CreatedAt time.Time `gorm:"index:idx_session_created,priority:2" json:"created_at"`
}

// TableName 指定表名
func (Message) TableName() string { return "messages" }

// SessionSequence 会话内序列原子分配表
// 用 INSERT ... ON DUPLICATE KEY UPDATE seq = seq + 1 保证并发安全且持久。
type SessionSequence struct {
	SessionID string    `gorm:"primaryKey;size:64" json:"session_id"`
	Seq       int64     `gorm:"not null;default:0" json:"seq"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (SessionSequence) TableName() string { return "session_sequences" }
