package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMustUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ok", func(c *gin.Context) {
		c.Set("username", "alice")
		got, ok := mustUsername(c)
		if !ok || got != "alice" {
			t.Fatalf("mustUsername = %q, %v", got, ok)
		}
		c.Status(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("code = %d", w.Code)
	}

	r2 := gin.New()
	r2.GET("/missing", func(c *gin.Context) {
		if _, ok := mustUsername(c); ok {
			t.Fatal("missing username should fail")
		}
	})
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("missing username code = %d, want 401", w2.Code)
	}
}

func TestMustUserIDRejectsWrongType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/bad", func(c *gin.Context) {
		c.Set("user_id", 123)
		if _, ok := mustUserID(c); ok {
			t.Fatal("non-string user_id should fail")
		}
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/bad", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong type code = %d, want 401", w.Code)
	}
}
