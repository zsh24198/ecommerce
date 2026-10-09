// /root/ecommerce/internal/outbox/kafka_publisher.go
package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/kafka"
	"github.com/zsh24198/ecommerce/shared/logger"
	"go.uber.org/zap"
)

// eventEnvelope Kafka 消息信封：外层是路由与幂等信息，payload 是业务数据原文。
type eventEnvelope struct {
	EventID       uint64          `json:"event_id"`
	EventType     string          `json:"event_type"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	Payload       json.RawMessage `json:"payload"`
	OccurredAt    time.Time       `json:"occurred_at"`
}

// KafkaPublisher 基于 Kafka 的事件投递器，实现 Publisher 接口。
type KafkaPublisher struct {
	producer    *kafka.Producer
	topicPrefix string
}

func NewKafkaPublisher(producer *kafka.Producer, topicPrefix string) *KafkaPublisher {
	return &KafkaPublisher{producer: producer, topicPrefix: topicPrefix}
}

// Publish 同步投递：topic = prefix + aggregate_type，key = aggregate_id。
func (p *KafkaPublisher) Publish(ctx context.Context, evt *OutboxEvent) error {
	env := eventEnvelope{
		EventID:       evt.ID,
		EventType:     evt.EventType,
		AggregateType: evt.AggregateType,
		AggregateID:   evt.AggregateID,
		Payload:       json.RawMessage(evt.Payload),
		OccurredAt:    evt.CreatedAt,
	}
	data, err := json.Marshal(env)
	if err != nil {
		return apperror.Wrap(apperror.CodeKafkaProduce, "marshal event envelope failed", err)
	}

	topic := p.topicPrefix + evt.AggregateType
	partition, offset, err := p.producer.SendMessage(topic, evt.AggregateID, data)
	if err != nil {
		return apperror.Wrap(apperror.CodeKafkaProduce, "kafka publish failed", err)
	}

	logger.Info(ctx, "outbox event delivered to kafka",
		zap.Uint64("event_id", evt.ID),
		zap.String("topic", topic),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset))
	return nil
}
