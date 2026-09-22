package main

import (
	"github.com/gin-gonic/gin"
	"github.com/zsh24198/ecommerce/shared/config"
	"github.com/zsh24198/ecommerce/shared/logger"
	"github.com/zsh24198/ecommerce/shared/middleware"
)

func main() {
	// 先初始化 logger，否则 middleware.Logger 调用时 defaultLogger 为 nil → panic
	if err := logger.Init(config.LogConfig{
		Level:    "info",
		Filename: "logs/apptest.log",
		MaxSize:  10,
	}); err != nil {
		panic(err)
	}
	defer logger.Sync()

	r := gin.New()
	r.Use(
		middleware.CORS(),
		middleware.TraceID(),
		middleware.Logger(),
		middleware.Recovery(),
	)

	r.GET("/ping", func(c *gin.Context) {
		middleware.OK(c, gin.H{"pong": true})
	})
	r.GET("/panic", func(c *gin.Context) {
		var m *map[string]any
		(*m)["boom"] = true // 解引用 nil 指针 → panic
		middleware.OK(c, "never")
	})

	r.Run(":8080")
}
