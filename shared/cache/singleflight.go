package cache

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

// SingleFlight 单飞封装：缓存击穿防护的核心组件。
// 同 key 并发请求只执行一次回源，其他请求共享结果。
// trade-off：第一个请求失败时所有等待者都拿到错误，调用方需加 fallback。
type SingleFlight struct {
	group singleflight.Group
}

// NewSingleFlight 构造单飞实例。无状态，可全局共享。
func NewSingleFlight() *SingleFlight {
	return &SingleFlight{}
}

// Do 执行 fn，同 key 并发只跑一次，其他请求共享结果。
// ctx 超时由调用方控制：若第一个请求卡死，等待者的 ctx 也会超时退出。
// shared=true 表示本次结果来自共享（你是等待者），false 表示独享（你是第一个）。
func (s *SingleFlight) Do(
	ctx context.Context,
	key string,
	fn func() (interface{}, error),
) (result interface{}, err error, shared bool) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
	}
	done := make(chan struct{})
	go func() {
		result, err, shared = s.group.Do(key, func() (interface{}, error) {
			return fn()
		})
		close(done)
	}()
	select {
	case <-done:
		return result, err, shared
	case <-ctx.Done():
		return nil, ctx.Err(), false
	}
}
