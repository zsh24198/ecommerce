package main

import (
	"context"

	"github.com/zsh24198/ecommerce/shared/config"
	"github.com/zsh24198/ecommerce/shared/logger"
)

func main() {
	if err := logger.Init(config.LogConfig{
		Level:      "info",
		Filename:   "logs/app.log",
		MaxSize:    100,
		MaxBackups: 30,
		MaxAge:     7,
	}); err != nil {
		panic(err)
	}
	defer logger.Sync()

	ctx := logger.WithTraceID(context.Background(), "trace-test-001")
	logger.Info(ctx, "用户登录成功")
	logger.Error(ctx, "模拟一个错误")

	// 没有 trace_id 的日志
	logger.Info(context.Background(), "启动完成")
}
