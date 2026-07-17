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
	SessionID  string    `bson:"_id" json:"session_id"`
	UserID     string    `bson:"user_id" json:"user_id"`
	AiID       uint      `bson:"ai_id" json:"ai_id"`
	Title      string    `bson:"title" json:"title"`
	Status     int       `bson:"status" json:"status"`
	MessageCnt int       `bson:"message_cnt" json:"message_cnt"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
}

type AiMessage struct {
	MongoID   primitive.ObjectID `json:"-" bson:"_id,omitempty"`
	SessionID string             `json:"session_id" bson:"session_id"`
	UserID    string             `json:"user_id" bson:"user_id"`
	Role      string             `json:"role" bson:"role"`
	Content   string             `json:"content" bson:"content"`
	Sequence  int                `json:"sequence" bson:"sequence"`
	CreatedAt time.Time          `json:"created_at" bson:"created_at"`
}
