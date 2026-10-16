package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

// HeaderTraceID 与上游/网关约定透传的 header，供前端/客服报障携带
const HeaderTraceID = "X-Trace-ID"

// TraceID 把当前 OTel server span 的 trace_id 写入响应头。
// 必须在 otelgin 中间件之后注册（依赖其创建的 server span）。
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if sc := trace.SpanContextFromContext(c.Request.Context()); sc.IsValid() {
			c.Header(HeaderTraceID, sc.TraceID().String())
		}
		c.Next()
	}
}
