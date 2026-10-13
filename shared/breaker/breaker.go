// Package breaker 提供基于 sony/gobreaker 的熔断器封装。
// 为 Redis / Kafka 等依赖故障提供快速失败能力，避免雪崩。
package breaker

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	"github.com/zsh24198/ecommerce/shared/logger"
	"go.uber.org/zap"
)

// Config 熔断器配置。阈值和超时时间可调。
type Config struct {
	Name             string        // 实例名（日志+metrics 标签）
	FailureThreshold uint32        // 连续失败多少次后打开熔断器
	Timeout          time.Duration // 打开后多久进入 half-open 探测
	MaxRequests      uint32        // half-open 时允许的最大探测请求数
}

// DefaultConfig 生产环境推荐配置：5次失败打开，30s 后探测。
func DefaultConfig(name string) Config {
	return Config{
		Name:             name,
		FailureThreshold: 5,
		Timeout:          30 * time.Second,
		MaxRequests:      1,
	}
}

// Breaker 封装 gobreaker，提供 Execute + 状态观测。
type Breaker struct {
	cb *gobreaker.CircuitBreaker
}

// New 创建熔断器实例。
func New(cfg Config) *Breaker {
	var st gobreaker.Settings
	st.Name = cfg.Name
	st.ReadyToTrip = func(counts gobreaker.Counts) bool {
		return counts.ConsecutiveFailures >= cfg.FailureThreshold
	}
	st.Timeout = cfg.Timeout
	st.MaxRequests = cfg.MaxRequests
	st.OnStateChange = func(name string, old, new gobreaker.State) {
		logger.Warn(context.Background(), "breaker state changed",
			zap.String("name", name),
			zap.String("old", stateName(old)),
			zap.String("new", stateName(new)),
		)
	}
	return &Breaker{cb: gobreaker.NewCircuitBreaker(st)}
}

// Execute 执行 f，f 返回非 nil error 视为失败。
// 如果熔断器处于 Open 状态，立即返回 ErrOpen，不执行 f。
func (b *Breaker) Execute(f func() error) error {
	_, err := b.cb.Execute(func() (interface{}, error) {
		return nil, f()
	})
	return err
}

// State 当前状态名（用于日志/metrics 标签）。
func (b *Breaker) State() string { return stateName(b.cb.State()) }

func stateName(s gobreaker.State) string {
	switch s {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
