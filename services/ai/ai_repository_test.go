package ai

import (
	"testing"

	"ai-meeting/dto"
	"ai-meeting/models"
)

type fakeAIRepository struct {
	conversation *models.Conversation
	message      *models.Message
	properties   *models.AiProperties
	enabled      []models.AiProperties
}

func (f *fakeAIRepository) CreateConversation(conv *models.Conversation) error {
	f.conversation = conv
	return nil
}

func (f *fakeAIRepository) PageConversations(string, string, int, int) ([]models.Conversation, int64, error) {
	return nil, 0, nil
}

func (f *fakeAIRepository) UpdateConversation(string, string, map[string]interface{}) error {
	return nil
}

func (f *fakeAIRepository) EndConversation(string, string) error { return nil }

func (f *fakeAIRepository) DeleteConversation(string, string) error { return nil }

func (f *fakeAIRepository) FindConversationBySessionAndUser(string, string) (*models.Conversation, error) {
	return nil, nil
}

func (f *fakeAIRepository) ListMessagesAsc(string, string) ([]models.Message, error) {
	return nil, nil
}

func (f *fakeAIRepository) PageMessages(string, string, int, int) ([]models.Message, int64, error) {
	return nil, 0, nil
}

func (f *fakeAIRepository) CreateMessage(msg *models.Message) error {
	f.message = msg
	return nil
}

func (f *fakeAIRepository) CountMessages(string, string) (int64, error) { return 0, nil }

func (f *fakeAIRepository) FindAllEnabledAiProperties() ([]models.AiProperties, error) {
	return f.enabled, nil
}

func (f *fakeAIRepository) CreateAiProperties(prop *models.AiProperties) error {
	f.properties = prop
	return nil
}

func (f *fakeAIRepository) UpdateAiProperties(uint, map[string]interface{}) error { return nil }

func (f *fakeAIRepository) DeleteAiProperties(uint) error { return nil }

func (f *fakeAIRepository) FindAiPropertiesByID(uint) (*models.AiProperties, error) {
	return nil, nil
}

func (f *fakeAIRepository) PageAiProperties(int, int) ([]models.AiProperties, int64, error) {
	return nil, 0, nil
}

func (f *fakeAIRepository) ToggleAiPropertiesStatus(uint, bool) error { return nil }

func TestAiConversationServiceUsesInjectedRepository(t *testing.T) {
	repo := &fakeAIRepository{}
	svc := &AiConversationService{repo: repo}

	resp, err := svc.CreateConversationWithTitle("alice", 7, "你好")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if resp == nil || resp.SessionID == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if repo.conversation == nil || repo.conversation.UserID != "alice" ||
		repo.conversation.AgentID != 7 || repo.conversation.Scene != models.ConversationSceneAI {
		t.Fatalf("unexpected conversation: %+v", repo.conversation)
	}
}

func TestAiMessageServiceUsesInjectedRepository(t *testing.T) {
	repo := &fakeAIRepository{}
	svc := &AiMessageService{repo: repo}

	if err := svc.SaveMessage("s1", "alice", "user", "hello"); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if repo.message == nil || repo.message.SessionID != "s1" || repo.message.UserID != "alice" ||
		repo.message.Role != "user" || repo.message.Content != "hello" {
		t.Fatalf("unexpected message: %+v", repo.message)
	}
}

func TestAiPropertiesServiceUsesInjectedRepository(t *testing.T) {
	repo := &fakeAIRepository{enabled: []models.AiProperties{{ID: 1, Name: "deepseek"}}}
	svc := &AiPropertiesService{repo: repo}

	props, err := svc.GetAvailableAiModels()
	if err != nil {
		t.Fatalf("get models: %v", err)
	}
	if len(props) != 1 || props[0].Name != "deepseek" {
		t.Fatalf("unexpected models: %+v", props)
	}

	if err := svc.CreateAiProperties(dto.AiPropertiesCreateReqDTO{Name: "m", ModelType: "deepseek"}); err != nil {
		t.Fatalf("create properties: %v", err)
	}
	if repo.properties == nil || repo.properties.Name != "m" {
		t.Fatalf("unexpected properties: %+v", repo.properties)
	}
}
