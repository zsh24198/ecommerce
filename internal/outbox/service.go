package outbox

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/zsh24198/ecommerce/shared/logger"
)

// Publisher 事件投递接口：任务 15 用 LogPublisher，任务 16 换成 KafkaPublisher，Relay 逻辑一行不改。
type Publisher interface {
	Publish(ctx context.Context, evt *OutboxEvent) error
}

// LogPublisher 日志投递器：模拟投递成功，Kafka 接入前的占位实现。
type LogPublisher struct{}

func NewLogPublisher() *LogPublisher { return &LogPublisher{} }

func (p *LogPublisher) Publish(ctx context.Context, evt *OutboxEvent) error {
	logger.Info(ctx, "outbox event published",
		zap.Uint64("event_id", evt.ID),
		zap.String("event_type", evt.EventType),
		zap.String("aggregate_type", evt.AggregateType),
		zap.String("aggregate_id", evt.AggregateID),
		zap.String("payload", evt.Payload))
	return nil
}

// OutboxRelayService Relay 服务接口：轮询 outbox 表并投递事件。
type OutboxRelayService interface {
	// Run 阻塞式主循环，需在独立 goroutine 中运行；ctx 取消时优雅退出。
	Run(ctx context.Context)
}

type outboxRelay struct {
	repo      OutboxRepository
	publisher Publisher
	interval  time.Duration
	batchSize int
}

func NewOutboxRelay(repo OutboxRepository, publisher Publisher) OutboxRelayService {
	return &outboxRelay{
		repo:      repo,
		publisher: publisher,
		interval:  time.Second,
		batchSize: 100,
	}
}

func (s *outboxRelay) Run(ctx context.Context) {
	logger.Info(ctx, "outbox relay started")
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info(ctx, "outbox relay stopped")
			return
		case <-ticker.C:
			s.relayOnce(ctx)
		}
	}
}

// relayOnce 单轮投递：任一条失败立即返回（退避 + 保序），事件留在表中下轮重试。
func (s *outboxRelay) relayOnce(ctx context.Context) {
	events, err := s.repo.PollPending(ctx, s.batchSize)
	if err != nil {
		logger.Error(ctx, "outbox relay poll failed", zap.Error(err))
		return
	}
	for i := range events {
		evt := &events[i]
		if err := s.publisher.Publish(ctx, evt); err != nil {
			logger.Error(ctx, "outbox relay publish failed",
				zap.Uint64("event_id", evt.ID), zap.Error(err))
			return
		}
		if err := s.repo.MarkPublished(ctx, evt.ID); err != nil {
			logger.Error(ctx, "outbox relay mark published failed",
				zap.Uint64("event_id", evt.ID), zap.Error(err))
			return
		}
	}
}
