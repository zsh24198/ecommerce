package logger

import (
	"context"
	"os"
	"strings"

	"github.com/zsh24198/ecommerce/shared/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var defaultLogger *zap.Logger

// Init 根据配置初始化全局 logger，进程启动时调用一次
func Init(cfg config.LogConfig) error {
	// 定义日志格式，JSON 编码，机器可读，方便后续接 ELK/Loki
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())

	level := parseLevel(cfg.Level)

	// 流水线1：控制台
	consoleCore := zapcore.NewCore(enc, zapcore.Lock(os.Stdout), level)

	// 流水线2：文件 + lumberjack 切割
	fileWriter := &lumberjack.Logger{
		Filename:   cfg.Filename,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   true,
	}
	fileCore := zapcore.NewCore(enc, zapcore.AddSync(fileWriter), level)

	// AddCaller：日志里带上文件名和行号，排查时直接定位到代码位置
	// NewTee 把多路 Core 合并成一个（T 型分流：一条日志同时进控制台和文件）
	defaultLogger = zap.New(zapcore.NewTee(consoleCore, fileCore), zap.AddCaller())
	return nil
}

// parseLevel 配置字符串 → zap 级别；配置错误时兜底 info，保证常规可观测
func parseLevel(s string) zapcore.Level {
	switch strings.ToLower(s) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// log 统一出口：注入 trace_id 后按级别分发
func log(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	l := defaultLogger
	if tid := TraceIDFromContext(ctx); tid != "" {
		l = l.With(zap.String("trace_id", tid))
	}
	switch level {
	case zapcore.DebugLevel:
		l.Debug(msg, fields...)
	case zapcore.InfoLevel:
		l.Info(msg, fields...)
	case zapcore.WarnLevel:
		l.Warn(msg, fields...)
	case zapcore.ErrorLevel:
		l.Error(msg, fields...)
	case zapcore.FatalLevel:
		l.Fatal(msg, fields...)
	}
}

func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	log(ctx, zapcore.DebugLevel, msg, fields...)
}
func Info(ctx context.Context, msg string, fields ...zap.Field) {
	log(ctx, zapcore.InfoLevel, msg, fields...)
}
func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	log(ctx, zapcore.WarnLevel, msg, fields...)
}
func Error(ctx context.Context, msg string, fields ...zap.Field) {
	log(ctx, zapcore.ErrorLevel, msg, fields...)
}
func Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	log(ctx, zapcore.FatalLevel, msg, fields...)
}

// Sync 退出前调用，把缓冲区的日志刷进文件，防止丢日志
func Sync() {
	_ = defaultLogger.Sync()
}
