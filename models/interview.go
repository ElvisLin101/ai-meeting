package models

import (
	"time"
)

type InterviewSession struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SessionID  string    `gorm:"unique;size:64;not null" json:"session_id"`
	UserID     string    `gorm:"size:64;not null" json:"user_id"`
	Status     int       `gorm:"default:1" json:"status"`
	ResumePath string    `gorm:"size:500" json:"resume_path"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (InterviewSession) TableName() string {
	return "interview_sessions"
}

type InterviewRecord struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SessionID   string    `gorm:"size:64;index;not null" json:"session_id"`
	UserID      string    `gorm:"size:64;not null" json:"user_id"`
	QuestionNum string    `gorm:"size:32" json:"question_num"`
	Question    string    `gorm:"type:text" json:"question"`
	Answer      string    `gorm:"type:text" json:"answer"`
	Score       int       `json:"score"`
	Suggestions string    `gorm:"type:text" json:"suggestions"`
	CreatedAt   time.Time `json:"created_at"`
}

func (InterviewRecord) TableName() string {
	return "interview_records"
}

type InterviewQuestion struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SessionID  string    `gorm:"size:64;index;not null" json:"session_id"`
	Question   string    `gorm:"type:text;not null" json:"question"`
	QuestionNo string    `gorm:"size:32;not null" json:"question_no"`
	OrderNum   int       `json:"order_num"`
	Status     int       `gorm:"default:0" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

func (InterviewQuestion) TableName() string {
	return "interview_questions"
}
