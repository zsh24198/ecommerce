// shared/logger/trace.go
package logger

import "context"

// 自定义类型做 key，防止与其他包的 ctx key 冲突
type ctxKey string

const traceIDKey ctxKey = "trace_id"

// WithTraceID 把 trace_id 塞进 ctx，middleware 调用
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext 从 ctx 取出 trace_id，logger 内部调用
func TraceIDFromContext(ctx context.Context) string {
	if v := ctx.Value(traceIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
