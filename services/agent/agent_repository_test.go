package agent

import (
	"testing"

	"ai-meeting/dto"
	"ai-meeting/models"
)

type fakeAgentRepository struct {
	conversations []models.Conversation
	conversation  *models.Conversation
	total         int64
	offset        int
	limit         int
	messages      []models.Message
	message       *models.Message
	properties    *models.AgentProperties
	asset         *models.AgentFileAsset
}

func (f *fakeAgentRepository) CreateConversation(conv *models.Conversation) error {
	f.conversation = conv
	return nil
}

func (f *fakeAgentRepository) PageConversations(_, _ string, offset, limit int) ([]models.Conversation, int64, error) {
	f.offset, f.limit = offset, limit
	return f.conversations, f.total, nil
}

func (f *fakeAgentRepository) EndConversation(string, string) error { return nil }

func (f *fakeAgentRepository) FindConversationBySessionAndUser(string, string) (*models.Conversation, error) {
	return nil, nil
}

func (f *fakeAgentRepository) ListMessagesAsc(string, string) ([]models.Message, error) {
	return f.messages, nil
}

func (f *fakeAgentRepository) PageMessages(string, string, int, int) ([]models.Message, int64, error) {
	return f.messages, f.total, nil
}

func (f *fakeAgentRepository) CreateMessage(msg *models.Message) error {
	f.message = msg
	return nil
}

func (f *fakeAgentRepository) CountMessages(string, string) (int64, error) { return f.total, nil }

func (f *fakeAgentRepository) UpdateConversationMessageCount(string, string, int) error { return nil }

func (f *fakeAgentRepository) CreateAgentProperties(prop *models.AgentProperties) error {
	f.properties = prop
	return nil
}

func (f *fakeAgentRepository) UpdateAgentProperties(uint, map[string]interface{}) error { return nil }

func (f *fakeAgentRepository) DeleteAgentProperties(uint) error { return nil }

func (f *fakeAgentRepository) FindAgentPropertiesByName(string) (*models.AgentProperties, error) {
	return nil, nil
}

func (f *fakeAgentRepository) PageAgentProperties(int) ([]models.AgentProperties, int64, error) {
	return nil, 0, nil
}

func (f *fakeAgentRepository) CreateAgentFileAsset(asset *models.AgentFileAsset) error {
	f.asset = asset
	return nil
}

func TestAgentConversationServicePageUsesInjectedRepository(t *testing.T) {
	repo := &fakeAgentRepository{
		conversations: []models.Conversation{{SessionID: "s1", UserID: "alice", Title: "t"}},
		total:         1,
	}
	svc := &AgentConversationService{repo: repo}

	convs, total, err := svc.PageConversations("alice", dto.AgentConversationPageReqDTO{Page: 2, Size: 10})
	if err != nil {
		t.Fatalf("page conversations: %v", err)
	}
	if len(convs) != 1 || total != 1 || convs[0].SessionID != "s1" {
		t.Fatalf("unexpected result: %+v total=%d", convs, total)
	}
	if repo.offset != 10 || repo.limit != 10 {
		t.Fatalf("unexpected pagination offset=%d limit=%d", repo.offset, repo.limit)
	}
}

func TestAgentMessageServiceSaveUsesInjectedRepository(t *testing.T) {
	repo := &fakeAgentRepository{}
	svc := &AgentMessageService{repo: repo}

	if err := svc.SaveMessage("s1", "alice", "user", "hello"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if repo.message == nil || repo.message.SessionID != "s1" || repo.message.Content != "hello" {
		t.Fatalf("unexpected message: %+v", repo.message)
	}
}

func TestAgentPropertiesAndAssetUseInjectedRepository(t *testing.T) {
	repo := &fakeAgentRepository{}
	props := &AgentPropertiesService{repo: repo}
	files := &AgentFileAssetService{repo: repo}

	if err := props.Create(dto.AgentPropertiesReqDTO{Name: "a", Description: "d"}); err != nil {
		t.Fatalf("create properties: %v", err)
	}
	if repo.properties == nil || repo.properties.Name != "a" {
		t.Fatalf("unexpected properties: %+v", repo.properties)
	}

	if _, err := files.UploadAndPersist("s1", "resume", "alice", "r.pdf", "/tmp/r.pdf", 10); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if repo.asset == nil || repo.asset.Filename != "r.pdf" || repo.asset.UserID != "alice" {
		t.Fatalf("unexpected asset: %+v", repo.asset)
	}
}
