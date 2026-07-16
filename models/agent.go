package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AgentProperties struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"unique;size:100;not null" json:"name"`
	Description string    `gorm:"size:500" json:"description"`
	Config      string    `gorm:"type:text" json:"config"`
	IsEnabled   bool      `gorm:"default:true" json:"is_enabled"`
	ApiKey      string    `gorm:"size:255" json:"api_key"`
	ApiSecret   string    `gorm:"size:255" json:"api_secret"`
	ApiFlowId   string    `gorm:"size:255" json:"api_flow_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AgentProperties) TableName() string {
	return "agent_properties"
}

type AgentConversation struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SessionID  string    `gorm:"unique;size:64;not null" json:"session_id"`
	UserID     string    `gorm:"size:64;not null" json:"user_id"`
	AgentID    uint      `json:"agent_id"`
	Title      string    `gorm:"size:200" json:"title"`
	Status     int       `gorm:"default:1" json:"status"`
	MessageCnt int       `gorm:"default:0" json:"message_cnt"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (AgentConversation) TableName() string {
	return "agent_conversations"
}

type AgentMessage struct {
	ID           uint               `gorm:"primaryKey" json:"id" bson:"-"`
	MongoID      primitive.ObjectID `gorm:"-" json:"-" bson:"_id,omitempty"`
	SessionID    string             `gorm:"size:64;index;not null" json:"session_id" bson:"session_id"`
	UserID       string             `gorm:"size:64;not null" json:"user_id" bson:"user_id"`
	Role         string             `gorm:"size:20;not null" json:"role" bson:"role"`
	Content      string             `gorm:"type:text;not null" json:"content" bson:"content"`
	Sequence     int                `json:"sequence" bson:"sequence"`
	ResponseTime int64              `json:"response_time,omitempty" bson:"response_time,omitempty"`
	ErrorMessage string             `gorm:"type:text" json:"error_message,omitempty" bson:"error_message,omitempty"`
	CreatedAt    time.Time          `json:"created_at" bson:"created_at"`
}

func (AgentMessage) TableName() string {
	return "agent_messages"
}

type AgentFileAsset struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SessionID string    `gorm:"size:64" json:"session_id"`
	UserID    string    `gorm:"size:64;not null" json:"user_id"`
	Filename  string    `gorm:"size:255;not null" json:"filename"`
	Path      string    `gorm:"size:500;not null" json:"path"`
	BizType   string    `gorm:"size:50" json:"biz_type"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func (AgentFileAsset) TableName() string {
	return "agent_file_assets"
}
