// /root/ecommerce/internal/order/consumer.go
package order

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/logger"
	"github.com/zsh24198/ecommerce/shared/redis"
)

// EventEnvelope Kafka 消息信封：与 outbox 的投递结构对齐（消费端独立定义，禁止 import outbox 包）。
type EventEnvelope struct {
	EventID       uint64          `json:"event_id"`
	EventType     string          `json:"event_type"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	Payload       json.RawMessage `json:"payload"`
	OccurredAt    time.Time       `json:"occurred_at"`
}

// EventHandler 订单事件处理器：event_id 去重 + 模拟异步处理（发短信/更新统计）。
type EventHandler struct {
	redis *redis.Client
}

func NewEventHandler(rdb *redis.Client) *EventHandler {
	return &EventHandler{redis: rdb}
}

// Handle 消费流程：解析 → 去重 → 处理。解析失败的消息直接跳过（毒消息不卡分区）。
func (h *EventHandler) Handle(ctx context.Context, msg *sarama.ConsumerMessage) error {
	var env EventEnvelope
	if err := json.Unmarshal(msg.Value, &env); err != nil {
		logger.Error(ctx, "malformed event, skip",
			zap.String("topic", msg.Topic), zap.ByteString("value", msg.Value), zap.Error(err))
		return nil
	}

	dedupKey := fmt.Sprintf("consumed:order:%d", env.EventID)
	ok, err := h.redis.SetNX(ctx, dedupKey, 1, 24*time.Hour).Result()
	if err != nil {
		return apperror.Wrap(apperror.CodeRedisError, "redis dedup setnx failed", err)
	}
	if !ok {
		logger.Info(ctx, "duplicate event skipped", zap.Uint64("event_id", env.EventID))
		return nil
	}

	if err := h.process(ctx, &env); err != nil {
		h.redis.Del(ctx, dedupKey)
		return err
	}
	return nil
}

// process 模拟业务处理：真实场景这里调用短信服务、更新统计表等下游动作。
func (h *EventHandler) process(ctx context.Context, env *EventEnvelope) error {
	switch env.EventType {
	case "order.created":
		logger.Info(ctx, "order event processed, sms sent",
			zap.String("order_no", env.AggregateID), zap.Uint64("event_id", env.EventID))
	default:
		logger.Info(ctx, "event ignored", zap.String("event_type", env.EventType))
	}
	return nil
}
