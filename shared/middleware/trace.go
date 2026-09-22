package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
	"github.com/zsh24198/ecommerce/shared/logger"
)

// HeaderTraceID 与上游/网关约定透传的 header
const HeaderTraceID = "X-Trace-ID"

func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader(HeaderTraceID)
		if tid == "" {
			tid = genTraceID()
		}
		c.Request = c.Request.WithContext(logger.WithTraceID(c.Request.Context(), tid))
		c.Header(HeaderTraceID, tid)
		c.Next()
	}
}

// genTraceID 16 字节随机数 → 32 位 hex；用标准库，不加 uuid 依赖
func genTraceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
