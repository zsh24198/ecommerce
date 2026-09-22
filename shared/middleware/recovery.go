package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/logger"
	"go.uber.org/zap"
)

// Recovery 兜住 handler 及内层中间件的 panic：进程不崩，请求转 500，详情只进日志
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return // 没 panic，正常返回
			}
			err := fmt.Errorf("panic: %v", r) // panic 值是 any，统一格式化
			logger.Error(c.Request.Context(), "panic recovered",
				zap.Error(err),
				zap.String("stack", string(debug.Stack())),
			)
			if c.Writer.Written() { // 响应已写出一部分，不能再写
				return
			}
			Fail(c, apperror.Wrap(apperror.CodeUnknown, "系统繁忙，请稍后再试", err))
			c.Abort()
		}()
		c.Next()
	}
}
