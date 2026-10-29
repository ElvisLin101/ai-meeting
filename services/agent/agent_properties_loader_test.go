package agent

import (
	"testing"

	"ai-meeting/models"
)

type fakeLoaderRepository struct {
	active  []models.AgentProperties
	byID    *models.AgentProperties
	byName  *models.AgentProperties
	loadErr error
}

func (f *fakeLoaderRepository) ListActiveAgentProperties() ([]models.AgentProperties, error) {
	return f.active, f.loadErr
}

func (f *fakeLoaderRepository) FindAgentPropertiesByID(uint) (*models.AgentProperties, error) {
	return f.byID, nil
}

func (f *fakeLoaderRepository) FindAgentPropertiesByName(string) (*models.AgentProperties, error) {
	return f.byName, nil
}

func TestAgentPropertiesLoaderUsesInjectedRepository(t *testing.T) {
	repo := &fakeLoaderRepository{
		active: []models.AgentProperties{{ID: 1, Name: "general", IsEnabled: true}},
		byID:   &models.AgentProperties{ID: 2, Name: "fallback", IsEnabled: true},
		byName: &models.AgentProperties{ID: 3, Name: "named", IsEnabled: true},
	}
	loader := &AgentPropertiesLoader{repo: repo}
	loader.RefreshActiveAgents()

	if got := loader.GetByAgentID(1); got == nil || got.Name != "general" {
		t.Fatalf("cache hit failed: %+v", got)
	}
	if got := loader.GetByAgentID(2); got == nil || got.Name != "fallback" {
		t.Fatalf("db fallback failed: %+v", got)
	}
	if got := loader.GetByAgentName("named"); got == nil || got.ID != 3 {
		t.Fatalf("name fallback failed: %+v", got)
	}
}
