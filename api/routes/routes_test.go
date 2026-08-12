package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestPublicRoutesAccessible 公开接口不带 token 也必须可达(不被 RequireAuth 拦截)
func TestPublicRoutesAccessible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := SetupRouter()

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/xunzhi/v1/users/login"},
		{http.MethodPost, "/api/xunzhi/v1/users/register"},
		{http.MethodGet, "/api/xunzhi/v1/users/check-login"},
		{http.MethodGet, "/api/xunzhi/v1/users/is-admin"},
		{http.MethodGet, "/api/xunzhi/v1/users/has-username"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code == http.StatusUnauthorized {
			t.Errorf("%s %s should be public, got 401", c.method, c.path)
		}
	}
}

// TestProtectedRoutesRequireAuth 受保护接口不带 token 必须 401(强制认证生效)
func TestProtectedRoutesRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := SetupRouter()

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/xunzhi/v1/agents/sessions/s1/chat"},
		{http.MethodPost, "/api/xunzhi/v1/ai/sessions/s1/chat"},
		{http.MethodPost, "/api/xunzhi/v1/interview/sessions"},
		{http.MethodGet, "/api/xunzhi/v1/users/page"},
		{http.MethodPost, "/api/xunzhi/v1/users/admin"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s should require auth, got %d (want 401)", c.method, c.path, w.Code)
		}
	}
}
