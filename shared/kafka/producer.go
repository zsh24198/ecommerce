// /root/ecommerce/shared/kafka/producer.go
package kafka

import (
	"fmt"

	"github.com/IBM/sarama"

	"github.com/zsh24198/ecommerce/shared/config"
)

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
func (pr *Producer) SendMessage(topic, key string, value []byte) (int32, int64, error) {
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	}
	partition, offset, err := pr.p.SendMessage(msg)
	if err != nil {
		return 0, 0, fmt.Errorf("kafka send message: %w", err)
	}
	return partition, offset, nil
}

// Close 释放生产者连接。
func (pr *Producer) Close() error {
	return pr.p.Close()
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
