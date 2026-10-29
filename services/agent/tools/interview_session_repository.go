package tools

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"context"
)

type interviewSessionRepository interface {
	FindInterviewSession(ctx context.Context, sessionID, userID string) (*models.InterviewSession, error)
}

type mysqlInterviewSessionRepository struct{}

func (mysqlInterviewSessionRepository) FindInterviewSession(ctx context.Context, sessionID, userID string) (*models.InterviewSession, error) {
	return mysqlrepo.FindInterviewSession(ctx, sessionID, userID)
}

var interviewSessionRepo interviewSessionRepository = mysqlInterviewSessionRepository{}
