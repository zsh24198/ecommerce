// /root/ecommerce/shared/kafka/producer.go
package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/zsh24198/ecommerce/shared/config"
)

// tracer 包级 Tracer：生产/消费两侧共用（同一 package）
var tracer = otel.Tracer("github.com/zsh24198/ecommerce/shared/kafka")

// Producer 封装 sarama 同步生产者：发送阻塞直到 broker 确认，失败返回 error。
type Producer struct {
	p sarama.SyncProducer
}

// NewProducer 根据 Kafka 配置创建同步生产者。
func NewProducer(cfg *config.KafkaConfig) (*Producer, error) {
	sc := sarama.NewConfig()
	sc.Producer.RequiredAcks = sarama.WaitForAll
	sc.Producer.Retry.Max = cfg.Producer.RetryMax
	sc.Producer.Return.Successes = true
	sc.Producer.Compression = parseCompression(cfg.Producer.Compression)

	p, err := sarama.NewSyncProducer(cfg.Brokers, sc)
	if err != nil {
		return nil, fmt.Errorf("kafka new sync producer: %w", err)
	}
	return &Producer{p: p}, nil
}

// SendMessage 同步发送一条消息：key 决定分区路由（同 key 落同分区，保证分区内有序）。
// 返回分区号和 offset，便于排查问题。
// ctx 用于串联链路：创建 producer span，并把 traceparent 注入消息 headers 跨进程传播。
func (pr *Producer) SendMessage(ctx context.Context, topic, key string, value []byte) (int32, int64, error) {
	ctx, span := tracer.Start(ctx, "kafka.send",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", topic),
		))
	defer span.End()

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	}
	// carrier 统一用指针切片，注入后转换为 ProducerMessage 要求的值切片
	var hs []*sarama.RecordHeader
	otel.GetTextMapPropagator().Inject(ctx, &saramaHeaderCarrier{headers: &hs})
	for _, h := range hs {
		msg.Headers = append(msg.Headers, *h)
	}

	partition, offset, err := pr.p.SendMessage(msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "kafka send failed")
		return 0, 0, fmt.Errorf("kafka send message: %w", err)
	}
	return partition, offset, nil
}

// Close 释放生产者连接。
func (pr *Producer) Close() error {
	return pr.p.Close()
}

// saramaHeaderCarrier 把 sarama 消息 headers 适配为 OTel TextMapCarrier，
// 供 propagator 在 Producer/Consumer 间读写 traceparent（两侧共用）。
type saramaHeaderCarrier struct{ headers *[]*sarama.RecordHeader }

func (c *saramaHeaderCarrier) Get(key string) string {
	for _, h := range *c.headers {
		if string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *saramaHeaderCarrier) Set(key, value string) {
	for i, h := range *c.headers {
		if string(h.Key) == key {
			(*c.headers)[i].Value = []byte(value)
			return
		}
	}
	*c.headers = append(*c.headers, &sarama.RecordHeader{Key: []byte(key), Value: []byte(value)})
}

func (c *saramaHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(*c.headers))
	for _, h := range *c.headers {
		keys = append(keys, string(h.Key))
	}
	return keys
}

func parseCompression(s string) sarama.CompressionCodec {
	switch s {
	case "gzip":
		return sarama.CompressionGZIP
	case "snappy":
		return sarama.CompressionSnappy
	case "lz4":
		return sarama.CompressionLZ4
	case "zstd":
		return sarama.CompressionZSTD
	default:
		return sarama.CompressionNone
	}
}
