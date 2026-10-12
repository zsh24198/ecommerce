package task

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/zsh24198/ecommerce/internal/order"
	"github.com/zsh24198/ecommerce/shared/logger"
)

// OrderExpireScanner 订单超时扫描器：周期性调用 OrderService.CancelExpiredOrders
// 取消超时未支付订单并回补库存。
//
// 与 order 包解耦：task 包仅持有 OrderService 接口，不感知订单内部实现。
// 调用方（main.go）负责构造 OrderService 并注入。
type OrderExpireScanner struct {
	svc      order.OrderService
	interval time.Duration
}

// NewOrderExpireScanner 构造扫描器。interval <= 0 时取默认 1 分钟。
func NewOrderExpireScanner(svc order.OrderService, interval time.Duration) *OrderExpireScanner {
	if interval <= 0 {
		interval = time.Minute
	}
	return &OrderExpireScanner{svc: svc, interval: interval}
}

// Start 启动扫描器 goroutine，直到 ctx 被取消才返回。
// 启动时立即执行一次，避免冷启动后等待首个 tick 的延迟。
// 单次扫描失败不退出，仅记日志，下个周期继续重试（DB/Redis 抖动是常态）。
// 调用方：go scanner.Start(ctx)
func (s *OrderExpireScanner) Start(ctx context.Context) {
	runOnce := func() {
		canceled, err := s.svc.CancelExpiredOrders(ctx)
		if err != nil {
			logger.Error(ctx, "order expire scan failed", zap.Error(err))
			return
		}
		if canceled > 0 {
			logger.Info(ctx, "order expire scan done", zap.Int("canceled", canceled))
		}
	}

	logger.Info(ctx, "order expire scanner starting", zap.Duration("interval", s.interval))
	runOnce() // 冷启动立即扫一次，不等首个 tick

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info(ctx, "order expire scanner stopped")
			return
		case <-ticker.C:
			runOnce()
		}
	}
}
