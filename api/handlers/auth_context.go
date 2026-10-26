package handlers

import (
	"ai-meeting/api/resp"
	"ai-meeting/pkg/ecode"

	"github.com/gin-gonic/gin"
)

// mustUsername 读取 JWT 中间件注入的 username；失败时统一返回 401 并中止。
func mustUsername(ctx *gin.Context) (string, bool) {
	value, exists := ctx.Get("username")
	username, ok := value.(string)
	if !exists || !ok || username == "" {
		resp.Fail(ctx, ecode.NotLogin, "Unauthorized")
		ctx.Abort()
		return "", false
	}
	return username, true
}

// mustUserID 读取 JWT 中间件注入的 user_id；失败时统一返回 401 并中止。
func mustUserID(ctx *gin.Context) (string, bool) {
	value, exists := ctx.Get("user_id")
	userID, ok := value.(string)
	if !exists || !ok || userID == "" {
		resp.Fail(ctx, ecode.NotLogin, "Unauthorized")
		ctx.Abort()
		return "", false
	}
	return userID, true
}
