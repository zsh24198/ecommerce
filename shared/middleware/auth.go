// Package middleware 提供 JWT 鉴权中间件：从 Authorization 头解析 access token，
// 将 user_id 写入 gin.Context，供后续 handler 读取。
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/jwtx"
)

// ContextKeyUserID 是 gin.Context 中存放当前用户 ID 的键。
// 用自定义类型而非 string，避免与其他中间件的键冲突。
type ContextKey string

const (
	CtxKeyUserID ContextKey = "user_id"
)

// JWT 构造一个 JWT 鉴权中间件。jwtMgr 用于解析 token。
// 校验失败直接返回 401，阻止请求进入 handler。
func JWT(jwtMgr *jwtx.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			Fail(c, apperror.New(apperror.CodeTokenInvalid, "缺少 Authorization 头"))
			c.Abort()
			return
		}
		// 格式：Bearer <token>
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
			Fail(c, apperror.New(apperror.CodeTokenInvalid, "Authorization 格式错误"))
			c.Abort()
			return
		}
		claims, err := jwtMgr.Parse(parts[1], jwtx.TypAccess)
		if err != nil {
			Fail(c, err)
			c.Abort()
			return
		}
		// 将 user_id 存入 context，handler 通过 c.GetInt64(string(CtxKeyUserID)) 读取
		c.Set(string(CtxKeyUserID), claims.UserID)
		c.Next()
	}
}

// CurrentUserID 从 gin.Context 中读取当前登录用户 ID。
// 未登录（中间件未设置）返回 0。
func CurrentUserID(c *gin.Context) int64 {
	v, ok := c.Get(string(CtxKeyUserID))
	if !ok {
		return 0
	}
	id, _ := v.(int64)
	return id
}
