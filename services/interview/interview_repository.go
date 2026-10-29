package interview

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"context"
)

// interviewRepository 是面试 facade/记录服务需要的最小存储接口。
type interviewRepository interface {
	CreateInterviewSession(ctx context.Context, session *models.InterviewSession) error
	EndInterviewSession(ctx context.Context, sessionID, userID string) error
	DeleteInterviewSession(ctx context.Context, sessionID string) error
	UpdateResumePath(ctx context.Context, sessionID, userID, resumePath string) error
	FindInterviewSession(ctx context.Context, sessionID, userID string) (*models.InterviewSession, error)

	CreateConversation(conv *models.Conversation) error
	DeleteConversation(sessionID, userID string) error
	PageConversations(userID, scene string, offset, limit int) ([]models.Conversation, int64, error)
	EndConversation(sessionID, userID string) error
	ListMessagesAsc(sessionID, userID string) ([]models.Message, error)
	PageMessages(sessionID, userID string, page, size int) ([]models.Message, int64, error)

	CreateInterviewRecord(ctx context.Context, record *models.InterviewRecord) error
	PageInterviewRecords(ctx context.Context, userID, sessionID string, page, size int) ([]models.InterviewRecord, int64, error)
	FindInterviewRecordBySessionID(ctx context.Context, sessionID, userID string) (*models.InterviewRecord, error)
	FindTurnArchives(ctx context.Context, sessionID string) ([]models.InterviewSessionTurnArchive, error)
}

type mysqlInterviewRepository struct{}

func (mysqlInterviewRepository) CreateInterviewSession(ctx context.Context, session *models.InterviewSession) error {
	return mysqlrepo.CreateInterviewSession(ctx, session)
}

func (mysqlInterviewRepository) EndInterviewSession(ctx context.Context, sessionID, userID string) error {
	return mysqlrepo.EndInterviewSession(ctx, sessionID, userID)
}

func (mysqlInterviewRepository) DeleteInterviewSession(ctx context.Context, sessionID string) error {
	return mysqlrepo.DeleteInterviewSession(ctx, sessionID)
}

func (mysqlInterviewRepository) UpdateResumePath(ctx context.Context, sessionID, userID, resumePath string) error {
	return mysqlrepo.UpdateResumePath(ctx, sessionID, userID, resumePath)
}

func (mysqlInterviewRepository) FindInterviewSession(ctx context.Context, sessionID, userID string) (*models.InterviewSession, error) {
	return mysqlrepo.FindInterviewSession(ctx, sessionID, userID)
}

func (mysqlInterviewRepository) CreateConversation(conv *models.Conversation) error {
	return mysqlrepo.CreateConversation(conv)
}

func (mysqlInterviewRepository) DeleteConversation(sessionID, userID string) error {
	return mysqlrepo.DeleteConversation(sessionID, userID)
}

func (mysqlInterviewRepository) PageConversations(userID, scene string, offset, limit int) ([]models.Conversation, int64, error) {
	return mysqlrepo.PageConversations(userID, scene, offset, limit)
}

func (mysqlInterviewRepository) EndConversation(sessionID, userID string) error {
	return mysqlrepo.EndConversation(sessionID, userID)
}

func (mysqlInterviewRepository) ListMessagesAsc(sessionID, userID string) ([]models.Message, error) {
	return mysqlrepo.ListMessagesAsc(sessionID, userID)
}

func (mysqlInterviewRepository) PageMessages(sessionID, userID string, page, size int) ([]models.Message, int64, error) {
	return mysqlrepo.PageMessages(sessionID, userID, page, size)
}

func (mysqlInterviewRepository) CreateInterviewRecord(ctx context.Context, record *models.InterviewRecord) error {
	return mysqlrepo.CreateInterviewRecord(ctx, record)
}

func (mysqlInterviewRepository) PageInterviewRecords(ctx context.Context, userID, sessionID string, page, size int) ([]models.InterviewRecord, int64, error) {
	return mysqlrepo.PageInterviewRecords(ctx, userID, sessionID, page, size)
}

func (mysqlInterviewRepository) FindInterviewRecordBySessionID(ctx context.Context, sessionID, userID string) (*models.InterviewRecord, error) {
	return mysqlrepo.FindInterviewRecordBySessionID(ctx, sessionID, userID)
}

func (mysqlInterviewRepository) FindTurnArchives(ctx context.Context, sessionID string) ([]models.InterviewSessionTurnArchive, error) {
	return mysqlrepo.FindTurnArchives(ctx, sessionID)
}
