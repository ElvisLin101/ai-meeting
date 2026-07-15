package models

import (
	"time"
)

type CompressedContext struct {
	ID                string    `bson:"_id"`
	SessionID         string    `bson:"session_id"`
	MemoryScope       string    `bson:"memory_scope,omitempty"`
	CompressedContent string    `bson:"compressed_content"`
	Index             int       `bson:"index"`
	TotalTokenCount   int64     `bson:"total_token_count"`
	MessageCount      int       `bson:"message_count"`
	CreatedAt         time.Time `bson:"created_at"`
	UpdatedAt         time.Time `bson:"updated_at"`
}
