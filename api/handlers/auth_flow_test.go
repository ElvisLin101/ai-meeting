package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestContext(method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestAgentRunBindRequestRequiresAuth(t *testing.T) {
	c, w := newTestContext(http.MethodPost, "/agent/runs", `{"input":"hi"}`)
	ctrl := &AgentRunController{}
	if _, ok := ctrl.bindRequest(c); ok {
		t.Fatal("missing auth should fail")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
}

func TestAgentRunBindRequestValid(t *testing.T) {
	c, w := newTestContext(http.MethodPost, "/agent/runs", `{"session_id":"s1","agent_id":2,"input":"hi"}`)
	c.Set("username", "alice")
	c.Set("user_id", "u1")
	ctrl := &AgentRunController{}
	req, ok := ctrl.bindRequest(c)
	if !ok {
		t.Fatalf("bind failed: %s", w.Body.String())
	}
	if req.Username != "alice" || req.UserID != "u1" || req.SessionID != "s1" || req.AgentID != 2 || req.Input != "hi" {
		t.Fatalf("unexpected request: %+v", req)
	}
}

func TestAgentRunBindRequestInvalidJSON(t *testing.T) {
	c, w := newTestContext(http.MethodPost, "/agent/runs", `{`)
	c.Set("username", "alice")
	c.Set("user_id", "u1")
	ctrl := &AgentRunController{}
	if _, ok := ctrl.bindRequest(c); ok {
		t.Fatal("invalid JSON should fail")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
}

func TestAgentRunGetAndPageRequireAuth(t *testing.T) {
	ctrl := &AgentRunController{}

	c, w := newTestContext(http.MethodGet, "/agent/runs/r1", "")
	ctrl.Get(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Get code = %d, want 401", w.Code)
	}

	c, w = newTestContext(http.MethodGet, "/agent/runs", "")
	ctrl.Page(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Page code = %d, want 401", w.Code)
	}
}

func TestInterviewCreateSessionRequiresAuth(t *testing.T) {
	c, w := newTestContext(http.MethodPost, "/interview/sessions", "")
	ctrl := &InterviewSessionController{}
	ctrl.CreateSession(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
}

func TestAgentCreateSessionRequiresAuth(t *testing.T) {
	c, w := newTestContext(http.MethodPost, "/agents/sessions", `{}`)
	ctrl := &AgentController{}
	ctrl.CreateSession(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
}
