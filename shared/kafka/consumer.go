// /root/ecommerce/shared/kafka/consumer.go
package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/zsh24198/ecommerce/shared/config"
	"github.com/zsh24198/ecommerce/shared/logger"
)

// MessageHandler 单条消息处理回调：返回 error 表示处理失败，
// 当前分区停止处理（不提交 offset），新 session 重新投递，实现 at-least-once。
type MessageHandler func(ctx context.Context, msg *sarama.ConsumerMessage) error

// ConsumerGroup 消费组封装：拉取 → 逐条处理 → 成功后手动提交 offset。
type ConsumerGroup struct {
	group   string
	topics  []string
	cg      sarama.ConsumerGroup
	handler MessageHandler
}

func NewConsumerGroup(cfg *config.KafkaConfig, topics []string, handler MessageHandler) (*ConsumerGroup, error) {
	sc := sarama.NewConfig()
	sc.Version = sarama.MaxVersion
	sc.Consumer.Offsets.Initial = sarama.OffsetOldest
	// 关闭自动提交：处理成功才 Mark+Commit，崩溃最多重复消费，绝不丢消息
	sc.Consumer.Offsets.AutoCommit.Enable = false

	cg, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.Consumer.GroupID, sc)
	if err != nil {
		return nil, fmt.Errorf("kafka new consumer group: %w", err)
	}
	return &ConsumerGroup{group: cfg.Consumer.GroupID, topics: topics, cg: cg, handler: handler}, nil
}

// Run 阻塞式消费主循环：Rebalance 或处理失败后自动重新进入 Consume 恢复消费。
func (c *ConsumerGroup) Run(ctx context.Context) {
	logger.Info(ctx, "kafka consumer started",
		zap.Strings("topics", c.topics), zap.String("group", c.group))
	for {
		if err := c.cg.Consume(ctx, c.topics, &groupHandler{handler: c.handler}); err != nil {
			if ctx.Err() != nil || errors.Is(err, sarama.ErrClosedConsumerGroup) {
				break
			}
			logger.Error(ctx, "kafka consume failed, retry in 1s", zap.Error(err))
			time.Sleep(time.Second)
		}
		if ctx.Err() != nil {
			break
		}
	}
	logger.Info(ctx, "kafka consumer stopped")
}

// Close 释放消费组连接。
func (c *ConsumerGroup) Close() error { return c.cg.Close() }

type groupHandler struct{ handler MessageHandler }

func (h *groupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *groupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

// ConsumeClaim 逐条处理分区消息：成功才 Mark+Commit；失败则返回 error，分区停摆等待重投。
func (h *groupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		if err := h.handler(sess.Context(), msg); err != nil {
			logger.Error(sess.Context(), "kafka message handle failed",
				zap.String("topic", msg.Topic), zap.Int64("offset", msg.Offset), zap.Error(err))
			return err
		}
		sess.MarkMessage(msg, "")
		sess.Commit()
	}
	return nil
}
