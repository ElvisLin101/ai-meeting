package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AiProperties struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"unique;size:100;not null" json:"name"`
	ModelType string    `gorm:"size:50;not null" json:"model_type"`
	ApiKey    string    `gorm:"size:255" json:"api_key"`
	ApiSecret string    `gorm:"size:255" json:"api_secret"`
	Endpoint  string    `gorm:"size:500" json:"endpoint"`
	Config    string    `gorm:"type:text" json:"config"`
	IsEnabled bool      `gorm:"default:true" json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AiProperties) TableName() string {
	return "ai_properties"
}

type AiConversation struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SessionID  string    `gorm:"unique;size:64;not null" json:"session_id"`
	UserID     string    `gorm:"size:64;not null" json:"user_id"`
	AiID       uint      `json:"ai_id"`
	Title      string    `gorm:"size:200" json:"title"`
	Status     int       `gorm:"default:1" json:"status"`
	MessageCnt int       `gorm:"default:0" json:"message_cnt"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (AiConversation) TableName() string {
	return "ai_conversations"
}

type AiMessage struct {
	ID        uint               `gorm:"primaryKey" json:"id" bson:"-"`
	MongoID   primitive.ObjectID `gorm:"-" json:"-" bson:"_id,omitempty"`
	SessionID string             `gorm:"size:64;index;not null" json:"session_id" bson:"session_id"`
	UserID    string             `gorm:"size:64;not null" json:"user_id" bson:"user_id"`
	Role      string             `gorm:"size:20;not null" json:"role" bson:"role"`
	Content   string             `gorm:"type:text;not null" json:"content" bson:"content"`
	Sequence  int                `json:"sequence" bson:"sequence"`
	CreatedAt time.Time          `json:"created_at" bson:"created_at"`
}

func (AiMessage) TableName() string {
	return "ai_messages"
}
