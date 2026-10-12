package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// CacheStore 缓存存储抽象。泛型参数 T 为业务对象类型。
// service 层依赖接口而非具体实现，便于 mock 测试与未来扩展。
// trade-off：泛型接口的隐式实现要求实现类也得是泛型，需注入 decode。
type CacheStore[T any] interface {
	Get(ctx context.Context, key string) (T, error)
	Set(ctx context.Context, key string, val T, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// redisCache Redis 实现的泛型 CacheStore。必须泛型 struct 才能实现泛型接口。
type redisCache[T any] struct {
	rdb    *goredis.Client
	decode func([]byte) (T, error)
}

// NewRedisCache 构造 Redis CacheStore。
// decode 由调用方提供：cache 包不认识 T 的具体结构，序列化权交给业务方。
func NewRedisCache[T any](rdb *goredis.Client, decode func([]byte) (T, error)) CacheStore[T] {
	return &redisCache[T]{rdb: rdb, decode: decode}
}

// Get 反序列化失败返回错。key 不存在用 goredis.Nil 区分，返回 zero, nil。
// 调用方按 zero 值判断 hit/miss，比 errors.Is(err, errCacheMiss) 更简洁。
func (c *redisCache[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return zero, nil
		}
		return zero, err
	}
	return c.decode(data)
}

// Set 用 JSON 序列化。Set 和 Get 序列化策略对称：都走 JSON。
func (c *redisCache[T]) Set(ctx context.Context, key string, val T, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, data, ttl).Err()
}

// Del 删除 key。key 不存在不报错。
func (c *redisCache[T]) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}
