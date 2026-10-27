package interview

import (
	"context"
	"testing"

	"ai-meeting/dto"
	"ai-meeting/models"
)

type fakeInterviewRepository struct {
	session *models.InterviewSession
	record  *models.InterviewRecord
	records []models.InterviewRecord
}

func (f *fakeInterviewRepository) CreateInterviewSession(_ context.Context, s *models.InterviewSession) error {
	f.session = s
	return nil
}

func (f *fakeInterviewRepository) EndInterviewSession(context.Context, string, string) error {
	return nil
}

func (f *fakeInterviewRepository) DeleteInterviewSession(context.Context, string) error { return nil }

func (f *fakeInterviewRepository) UpdateResumePath(context.Context, string, string, string) error {
	return nil
}

func (f *fakeInterviewRepository) FindInterviewSession(context.Context, string, string) (*models.InterviewSession, error) {
	return f.session, nil
}

func (f *fakeInterviewRepository) CreateConversation(*models.Conversation) error { return nil }

func (f *fakeInterviewRepository) DeleteConversation(string, string) error { return nil }

func (f *fakeInterviewRepository) PageConversations(string, string, int, int) ([]models.Conversation, int64, error) {
	return nil, 0, nil
}

func (f *fakeInterviewRepository) EndConversation(string, string) error { return nil }

func (f *fakeInterviewRepository) ListMessagesAsc(string, string) ([]models.Message, error) {
	return nil, nil
}

func (f *fakeInterviewRepository) PageMessages(string, string, int, int) ([]models.Message, int64, error) {
	return nil, 0, nil
}

func (f *fakeInterviewRepository) CreateInterviewRecord(_ context.Context, record *models.InterviewRecord) error {
	f.record = record
	return nil
}

func (f *fakeInterviewRepository) PageInterviewRecords(context.Context, string, string, int, int) ([]models.InterviewRecord, int64, error) {
	return f.records, int64(len(f.records)), nil
}

func (f *fakeInterviewRepository) FindInterviewRecordBySessionID(context.Context, string, string) (*models.InterviewRecord, error) {
	return f.record, nil
}

func (f *fakeInterviewRepository) FindTurnArchives(context.Context, string) ([]models.InterviewSessionTurnArchive, error) {
	return nil, nil
}

func TestInterviewRecordServiceUsesInjectedRepository(t *testing.T) {
	repo := &fakeInterviewRepository{}
	svc := &InterviewRecordService{repo: repo}

	if err := svc.SaveInterviewRecord("s1", "u1", dto.InterviewRecordSaveReqDTO{Question: "q", Answer: "a"}); err != nil {
		t.Fatalf("save record: %v", err)
	}
	if repo.record == nil || repo.record.SessionID != "s1" || repo.record.UserID != "u1" || repo.record.Question != "q" {
		t.Fatalf("unexpected record: %+v", repo.record)
	}
}
