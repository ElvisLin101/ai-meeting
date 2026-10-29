package agentctx

import (
	"context"
	"testing"

	"ai-meeting/models"
)

type fakeStoreRepository struct {
	mysqlRepository
	profile       *models.UserProfile
	savedProfile  *models.UserProfile
	summary       *models.CompressedContextRecord
	savedSummary  *models.CompressedContextRecord
	identity      *models.AgentIdentity
	savedIdentity *models.AgentIdentity
}

func (f *fakeStoreRepository) FindUserProfile(string) (*models.UserProfile, error) {
	return f.profile, nil
}

func (f *fakeStoreRepository) UpsertUserProfile(profile *models.UserProfile) error {
	f.savedProfile = profile
	return nil
}

func (f *fakeStoreRepository) FindCompressedContext(string) (*models.CompressedContextRecord, error) {
	return f.summary, nil
}

func (f *fakeStoreRepository) UpsertCompressedContext(record *models.CompressedContextRecord) error {
	f.savedSummary = record
	return nil
}

func (f *fakeStoreRepository) FindAgentIdentity(uint) (*models.AgentIdentity, error) {
	return f.identity, nil
}

func (f *fakeStoreRepository) UpsertAgentIdentity(identity *models.AgentIdentity) error {
	f.savedIdentity = identity
	return nil
}

func TestStoreProfileUsesRepositoryAndRedis(t *testing.T) {
	withTestRedis(t)
	repo := &fakeStoreRepository{}
	restore := setRepositoryForTest(repo)
	defer restore()

	profile := &UserProfileData{Version: 1, Goals: []string{"Go 后端"}}
	if err := SaveProfile(context.Background(), "u1", profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if repo.savedProfile == nil || repo.savedProfile.UserID != "u1" || repo.savedProfile.ProfileJSON == "" {
		t.Fatalf("repository not called: %+v", repo.savedProfile)
	}
	loaded := LoadProfile(context.Background(), "u1")
	if loaded == nil || len(loaded.Goals) != 1 || loaded.Goals[0] != "Go 后端" {
		t.Fatalf("load profile: %+v", loaded)
	}
}

func TestStoreSummaryUsesRepositoryAndRedis(t *testing.T) {
	withTestRedis(t)
	repo := &fakeStoreRepository{}
	restore := setRepositoryForTest(repo)
	defer restore()

	if err := SaveSummary(context.Background(), "s1", "u1", "old summary", 10, 100, 5); err != nil {
		t.Fatalf("save summary: %v", err)
	}
	if repo.savedSummary == nil || repo.savedSummary.Index != 10 {
		t.Fatalf("repository not called: %+v", repo.savedSummary)
	}
	summary, index := LoadSummary(context.Background(), "s1")
	if summary != "old summary" || index != 10 {
		t.Fatalf("load summary = %q/%d", summary, index)
	}
}

func TestStoreIdentityUsesRepositoryAndRedis(t *testing.T) {
	withTestRedis(t)
	repo := &fakeStoreRepository{}
	restore := setRepositoryForTest(repo)
	defer restore()

	identity := &AgentIdentityData{Name: "Tester", Role: "assistant", Version: 1}
	if err := SaveIdentity(context.Background(), 9, identity); err != nil {
		t.Fatalf("save identity: %v", err)
	}
	if repo.savedIdentity == nil || repo.savedIdentity.AgentID != 9 {
		t.Fatalf("repository not called: %+v", repo.savedIdentity)
	}
	loaded := LoadIdentity(context.Background(), 9)
	if loaded == nil || loaded.Name != "Tester" {
		t.Fatalf("load identity: %+v", loaded)
	}
}
