package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Header 常量集中管理，避免魔法字符串
const (
	headerOrigin        = "Access-Control-Allow-Origin"
	headerMethods       = "Access-Control-Allow-Methods"
	headerHeaders       = "Access-Control-Allow-Headers"
	headerExposeHeaders = "Access-Control-Expose-Headers"
	headerCredentials   = "Access-Control-Allow-Credentials"
)

// defaultAllowHeaders 是允许客户端携带的请求头白名单
var defaultAllowHeaders = []string{
	"Origin",
	"Content-Type",
	"Accept",
	"Authorization",
	"X-Trace-ID", // 让前端能读响应头里的 trace_id，报障时给客服
}

// CORS 处理浏览器跨域：加允许头 + 预检短路
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		// 开发期：*，生产应从配置读白名单做严格匹配
		c.Header(headerOrigin, "*")
		c.Header(headerMethods, "GET, POST, PUT, DELETE, OPTIONS")
		c.Header(headerHeaders, strings.Join(defaultAllowHeaders, ", "))
		c.Header(headerExposeHeaders, "X-Trace-ID")
		c.Header(headerCredentials, "true")

		// 预检请求短路，不进业务链路
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
