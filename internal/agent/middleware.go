package agent

import (
	"crypto/subtle"

	"github.com/gin-gonic/gin"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/middleware"
)

// AgentAuth 校验 X-Agent-Token，失败返回 2001 段认证错误。
// 与用户 JWT 中间件分开：Agent 走 X-Agent-Token 头，用户走 Authorization 头，天然分流。
func AgentAuth(agentToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Agent-Token")
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(agentToken)) != 1 {
			middleware.Fail(c, apperror.New(apperror.CodeTokenInvalid, "Agent Token 无效"))
			c.Abort()
			return
		}
		c.Next()
	}
}
