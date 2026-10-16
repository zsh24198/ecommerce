// shared/logger/trace.go
package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// TraceIDFromContext 从 OTel span context 取 trace_id。
// 方案 A：trace_id 唯一来源是 OpenTelemetry，日志与 Trace 天然同源。
func TraceIDFromContext(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}
