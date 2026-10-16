package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"

	"github.com/zsh24198/ecommerce/shared/config"
)

// Client 封装 go-redis，方便注入和 mock。
type Client struct {
	*goredis.Client
}

// New 根据配置创建 Redis 连接。
func New(cfg *config.RedisConfig) (*Client, error) {
	rdb := goredis.NewClient(&goredis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// OTel 埋点：每个 Redis 命令自动产生 client span
	// （main.go 中 redis.New 在 telemetry.Init 之后调用，能捕获到全局 TracerProvider）
	if err := redisotel.InstrumentTracing(rdb); err != nil {
		return nil, fmt.Errorf("redis instrument tracing: %w", err)
	}

	// 连接成功校验
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &Client{Client: rdb}, nil
}
