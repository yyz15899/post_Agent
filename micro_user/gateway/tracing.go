package main

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// 初始化OpenTelemetry

// 选择导出方式(将采集到的数据发送到外部)
func newExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	exporter, err := otlptracegrpc.New(ctx) // 从env中读取配置OTEL_EXPORTER_OTLP_TRACES_ENDPOIN()
	if err != nil {
		log.Printf("创建Exporter失败:%v", err)
		return nil, err
	}
	return exporter, nil
}

// 配置追踪管理器
func newTraceProvider(exp sdktrace.SpanExporter) *sdktrace.TracerProvider {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("otle_test"), // 后续在追踪平台中据此识别服务
		),
	)
	if err != nil {
		log.Printf("配置失败")
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp), // 批量导出追踪数据,减少流式开销
		sdktrace.WithResource(r),  // 追加配置信息
	)
}

// 初始化
func InitOtle(ctx context.Context) (*sdktrace.TracerProvider, error) {
	exp, err := newExporter(ctx)
	if err != nil {
		log.Fatalf("failed to initialize exporter: %v", err)
		return nil, err
	}
	// 创建跟踪器
	tp := newTraceProvider(exp)

	otel.SetTracerProvider(tp) // 注册全局provider

	return tp, nil
	// 设置追踪器
	// tracer := tp.Tracer("otle_test")

	// 创建span
	// ctx, span := tracer.Start(ctx, "span01") // 创建新的ctx,将流程串起来

	// 结束span
	// defer span.End()
}
