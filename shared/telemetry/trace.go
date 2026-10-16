// shared/telemetry/tracer.go
package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/zsh24198/ecommerce/shared/config"
)

// Init 初始化全局 TracerProvider 与跨进程传播器。
// 返回 shutdown：进程退出前必须调用，flush 缓冲区里未上报的 Trace（防丢数据）。
func Init(ctx context.Context, cfg config.TelemetryConfig) (func(context.Context) error, error) {
	exp, err := newExporter(ctx, cfg)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
		))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

// newExporter 按配置选择导出器：otlp 上报 Collector/Jaeger，默认 stdout（低配过渡方案）。
func newExporter(ctx context.Context, cfg config.TelemetryConfig) (sdktrace.SpanExporter, error) {
	if cfg.Exporter == "otlp" {
		return otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cfg.Endpoint),
			otlptracehttp.WithInsecure(),
		)
	}
	return stdouttrace.New(stdouttrace.WithPrettyPrint())
}
