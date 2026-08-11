package middleware

import (
	"ai-meeting/pkg/ecode"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Recovery 统一 panic 恢复中间件, 返回统一错误格式
// 替换 gin 默认的文本 panic 响应
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logrus.Errorf("panic recovered: %v\n%s", err, debug.Stack())
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":  ecode.ServerErr,
					"error": "internal server error",
				})
			}
		}()
		c.Next()
	}
}
