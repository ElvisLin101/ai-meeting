package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-meeting/config"

	"github.com/gin-gonic/gin"
)

// ============================================================
// JWT 认证中间件测试
// ============================================================

// setTestJWT 设置测试 JWT 配置: 防止 Expire=0 导致 token 立即过期
func setTestJWT(t *testing.T) {
	t.Helper()
	config.AppConfig.JWT.Secret = "test-secret"
	config.AppConfig.JWT.Expire = 3600
}

func TestGenerateToken(t *testing.T) {
	setTestJWT(t)
	token, err := GenerateToken("alice", "u1")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("token 为空")
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	setTestJWT(t)
	gin.SetMode(gin.TestMode)
	token, err := GenerateToken("alice", "u1")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	var gotUsername, gotUserID interface{}
	called := false
	r := gin.New()
	r.Use(AuthMiddleware())
	r.GET("/x", func(c *gin.Context) {
		called = true
		gotUsername, _ = c.Get("username")
		gotUserID, _ = c.Get("user_id")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler 未被调用")
	}
	if gotUsername != "alice" || gotUserID != "u1" {
		t.Errorf("username=%v user_id=%v, want alice/u1", gotUsername, gotUserID)
	}
}

func TestAuthMiddleware_NoHeader(t *testing.T) {
	setTestJWT(t)
	gin.SetMode(gin.TestMode)

	called := false
	var gotUsername interface{}
	r := gin.New()
	r.Use(AuthMiddleware())
	r.GET("/x", func(c *gin.Context) {
		called = true
		gotUsername, _ = c.Get("username")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil) // 无 Authorization
	r.ServeHTTP(w, req)

	if !called {
		t.Fatal("无 token 应放行到 handler")
	}
	if gotUsername != nil {
		t.Errorf("无 token 不应设置 username, got %v", gotUsername)
	}
}

func TestAuthMiddleware_NonBearer(t *testing.T) {
	setTestJWT(t)
	gin.SetMode(gin.TestMode)

	called := false
	r := gin.New()
	r.Use(AuthMiddleware())
	r.GET("/x", func(c *gin.Context) { called = true; c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Basic abcdef") // 非 Bearer 格式
	r.ServeHTTP(w, req)

	if !called {
		t.Fatal("非 Bearer 格式应放行")
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	setTestJWT(t)
	gin.SetMode(gin.TestMode)

	called := false
	var gotUsername interface{}
	r := gin.New()
	r.Use(AuthMiddleware())
	r.GET("/x", func(c *gin.Context) {
		called = true
		gotUsername, _ = c.Get("username")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	r.ServeHTTP(w, req)

	if !called {
		t.Fatal("无效 token 应放行(由 RequireAuth 拦截)")
	}
	if gotUsername != nil {
		t.Errorf("无效 token 不应设置 username, got %v", gotUsername)
	}
}

func TestRequireAuth_NoUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)

	called := false
	r := gin.New()
	r.Use(RequireAuth())
	r.GET("/x", func(c *gin.Context) { called = true; c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", w.Code)
	}
	if called {
		t.Fatal("未认证时 handler 不应被调用(应 Abort)")
	}
	if body := w.Body.String(); !strings.Contains(body, "Unauthorized") {
		t.Errorf("body = %s, want Unauthorized", body)
	}
}

func TestRequireAuth_WithUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)

	called := false
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("username", "alice"); c.Next() }) // 模拟已认证
	r.Use(RequireAuth())
	r.GET("/x", func(c *gin.Context) { called = true; c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", w.Code)
	}
	if !called {
		t.Fatal("已认证时 handler 应被调用")
	}
}
