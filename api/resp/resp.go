package resp

import (
	"ai-meeting/pkg/ecode"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ============================================================
// 统一响应出口
//
// 成功: 原样返回 data (HTTP 200), 不改变现有成功响应形态
// 失败: {"code": 错误码, "error": 消息}, HTTP 状态码由错误码映射
//       code 为新增字段, error 字段保持不变 → 前端逻辑不受影响
// ============================================================

// Respond 统一响应出口
func Respond(c *gin.Context, err error, data interface{}) {
	if err == nil {
		c.JSON(http.StatusOK, data)
		return
	}
	e := ecode.Cause(err)
	c.JSON(e.HTTPStatus(), gin.H{"code": e.Code(), "error": e.Message()})
}

// Fail 主动返回错误(非 error 透传场景, 如校验失败)
func Fail(c *gin.Context, code int, msg string) {
	e := ecode.New(code, msg)
	c.JSON(e.HTTPStatus(), gin.H{"code": e.Code(), "error": e.Message()})
}
