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
	SessionID  string    `bson:"_id" json:"session_id"`
	UserID     string    `bson:"user_id" json:"user_id"`
	AgentID    uint      `bson:"agent_id" json:"agent_id"`
	Title      string    `bson:"title" json:"title"`
	Status     int       `bson:"status" json:"status"`
	MessageCnt int       `bson:"message_cnt" json:"message_cnt"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
}

type AgentMessage struct {
	MongoID      primitive.ObjectID `json:"-" bson:"_id,omitempty"`
	SessionID    string             `json:"session_id" bson:"session_id"`
	UserID       string             `json:"user_id" bson:"user_id"`
	Role         string             `json:"role" bson:"role"`
	Content      string             `json:"content" bson:"content"`
	Sequence     int                `json:"sequence" bson:"sequence"`
	ResponseTime int64              `json:"response_time,omitempty" bson:"response_time,omitempty"`
	ErrorMessage string             `json:"error_message,omitempty" bson:"error_message,omitempty"`
	CreatedAt    time.Time          `json:"created_at" bson:"created_at"`
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
