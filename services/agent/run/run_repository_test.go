package run

import (
	"sync/atomic"
	"testing"

	"ai-meeting/models"
)

type fakeRunRepository struct {
	steps []models.AgentStep
}

func (f *fakeRunRepository) CreateAgentRun(*models.AgentRun) error { return nil }

func (f *fakeRunRepository) FinishAgentRun(string, string, string, string, string, int, int) error {
	return nil
}

func (f *fakeRunRepository) UpdateAgentRunPlan(string, string) error { return nil }

func (f *fakeRunRepository) UpdateAgentRunContextReport(string, string) error { return nil }

func (f *fakeRunRepository) CreateAgentStep(step *models.AgentStep) error {
	f.steps = append(f.steps, *step)
	return nil
}

func (f *fakeRunRepository) CreateMessage(*models.Message) error { return nil }

func (f *fakeRunRepository) CountMessages(string, string) (int64, error) { return 0, nil }

func (f *fakeRunRepository) UpdateConversationMessageCount(string, string, int) error {
	return nil
}

func (f *fakeRunRepository) FindConversationBySessionAndUser(string, string) (*models.Conversation, error) {
	return nil, nil
}

func (f *fakeRunRepository) CreateConversation(*models.Conversation) error { return nil }

func TestStepRecorderUsesInjectedRepository(t *testing.T) {
	var seq atomic.Int64
	repo := &fakeRunRepository{}
	rec := &stepRecorder{repo: repo, runID: "r1", taskID: "t1", seq: &seq}

	if err := rec.RecordStep(&models.AgentStep{StepIndex: 99, Status: models.AgentStepStatusOK}); err != nil {
		t.Fatalf("record step: %v", err)
	}
	if len(repo.steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(repo.steps))
	}
	got := repo.steps[0]
	if got.RunID != "r1" || got.TaskID != "t1" || got.StepIndex != 1 {
		t.Fatalf("unexpected step: %+v", got)
	}
}
