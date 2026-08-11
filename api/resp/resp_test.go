package resp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ai-meeting/pkg/ecode"

	"github.com/gin-gonic/gin"
)

// ============================================================
// 统一响应出口测试: Respond 成功透传 / 错误码映射 / Fail
// ============================================================

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newTestCtx() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	return c, w
}

func TestRespond_Success(t *testing.T) {
	c, w := newTestCtx()
	Respond(c, nil, gin.H{"ok": true})

	if w.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("body = %s, want ok:true", w.Body.String())
	}
}

func TestRespond_StandardError(t *testing.T) {
	c, w := newTestCtx()
	Respond(c, ecode.New(ecode.NotLogin, "Unauthorized"), nil)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":-101`) || !strings.Contains(body, "Unauthorized") {
		t.Errorf("body = %s, want {code:-101, error:Unauthorized}", body)
	}
}

func TestRespond_WrappedError(t *testing.T) {
	// ecode.Wrap 包装后 Cause 应穿透错误码
	c, w := newTestCtx()
	Respond(c, ecode.Wrap(ecode.New(ecode.ErrDeepSeekCall, "upstream down"), "调用失败"), nil)

	if w.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502(ErrDeepSeekCall 注册的 HTTP 状态)", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":-4002`) {
		t.Errorf("body = %s, want code -4002", body)
	}
}

func TestRespond_PlainError(t *testing.T) {
	// 裸错误兜底 -500 并保留原文消息
	c, w := newTestCtx()
	Respond(c, errors.New("boom"), nil)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("code = %d, want 500", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":-500`) || !strings.Contains(body, "boom") {
		t.Errorf("body = %s, want {code:-500, error:boom}", body)
	}
}

func TestFail(t *testing.T) {
	c, w := newTestCtx()
	Fail(c, ecode.RequestErr, "bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":-400`) || !strings.Contains(body, "bad request") {
		t.Errorf("body = %s, want {code:-400, error:bad request}", body)
	}
}
