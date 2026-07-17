package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type InterviewSession struct {
	SessionID  string    `bson:"_id" json:"session_id"`
	UserID     string    `bson:"user_id" json:"user_id"`
	Status     int       `bson:"status" json:"status"`
	ResumePath string    `bson:"resume_path" json:"resume_path"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
}

type InterviewRecord struct {
	MongoID     primitive.ObjectID `json:"-" bson:"_id,omitempty"`
	SessionID   string             `bson:"session_id" json:"session_id"`
	UserID      string             `bson:"user_id" json:"user_id"`
	QuestionNum string             `bson:"question_num" json:"question_num"`
	Question    string             `bson:"question" json:"question"`
	Answer      string             `bson:"answer" json:"answer"`
	Score       int                `bson:"score" json:"score"`
	Suggestions string             `bson:"suggestions" json:"suggestions"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
}

type InterviewQuestion struct {
	MongoID    primitive.ObjectID `json:"-" bson:"_id,omitempty"`
	SessionID  string             `bson:"session_id" json:"session_id"`
	Question   string             `bson:"question" json:"question"`
	QuestionNo string             `bson:"question_no" json:"question_no"`
	OrderNum   int                `bson:"order_num" json:"order_num"`
	Status     int                `bson:"status" json:"status"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}
