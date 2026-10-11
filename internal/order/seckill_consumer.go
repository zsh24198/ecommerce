package order

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/zsh24198/ecommerce/shared/logger"
	"github.com/zsh24198/ecommerce/shared/redis"
)

// SeckillOrderMsg 秒杀下单消息。消费端独立定义，仅与 JSON 字段名对齐，禁止 import seckill 包。
type SeckillOrderMsg struct {
	EventID           string `json:"event_id"`
	UserID            int64  `json:"user_id"`
	ActivityID        int64  `json:"activity_id"`
	ItemID            int64  `json:"item_id"`
	SKUID             int64  `json:"sku_id"`
	SKUName           string `json:"sku_name"`
	Quantity          int    `json:"quantity"`
	SeckillPriceCents int64  `json:"seckill_price_cents"`
}

// SeckillStockRestorer 秒杀库存回补接口：由 seckill 模块的 StockStore 实现，
// 通过接口注入避免 order 直接 import seckill 的具体类型（跨域红线）。
type SeckillStockRestorer interface {
	Restore(ctx context.Context, itemID int64, qty int) (bool, error)
}

	const (
	seckillDedupTTL    = 24 * time.Hour
	seckillMaxRetries  = 3
	seckillDedupKeyFmt = "consumed:seckill:%s"
	seckillRetryKeyFmt = "consumed:seckill:retry:%s"
	seckillOrderKeyFmt = "order:seckill:%s" // event_id -> order_no，轮询接口读此 key
	seckillOrderTTL    = 1 * time.Hour
)

// SeckillOrderConsumer 秒杀下单消费者：解析 → 去重 → 异步建单。
type SeckillOrderConsumer struct {
	svc      OrderService
	redis    *redis.Client
	restorer SeckillStockRestorer
}

func NewSeckillOrderConsumer(svc OrderService, rdb *redis.Client, restorer SeckillStockRestorer) *SeckillOrderConsumer {
	return &SeckillOrderConsumer{svc: svc, redis: rdb, restorer: restorer}
}

// Handle 实现 kafka.MessageHandler：单条消息处理回调。
//
// 失败处理策略（核心：宁可少卖，绝不超卖）：
//   - 解析失败（毒消息）→ 跳过，不重投
//   - 重复消息（event_id 已消费）→ 跳过
//   - 建单失败且未超限 → 删去重占位，返回 error 触发 Kafka 重投（不回补库存，等重投建单成功消耗库存）
//   - 建单失败且超限 → 判定不可恢复，回补 Redis 库存，跳过消息避免分区卡死
func (c *SeckillOrderConsumer) Handle(ctx context.Context, msg *sarama.ConsumerMessage) error {
	var m SeckillOrderMsg
	if err := json.Unmarshal(msg.Value, &m); err != nil {
		logger.Error(ctx, "seckill order msg unmarshal failed, skip",
			zap.ByteString("value", msg.Value), zap.Error(err))
		return nil // 毒消息，跳过不重投
	}

	// 消费幂等：event_id 去重。Redis SETNX 快路径，DB (user_id,idempotency_key) 唯一索引兜底。
	dedupKey := fmt.Sprintf(seckillDedupKeyFmt, m.EventID)
	ok, err := c.redis.SetNX(ctx, dedupKey, 1, seckillDedupTTL).Result()
	if err != nil {
		return err // Redis 故障是可重试失败，重投
	}
	if !ok {
		logger.Info(ctx, "seckill order duplicate skipped", zap.String("event_id", m.EventID))
		return nil
	}

	orderNo, err := c.svc.CreateSeckillOrder(ctx, &m)
	if err != nil {
		c.redis.Del(ctx, dedupKey) // 失败删占位，让重投消息能重新走完整流程

		retries := c.incrRetry(ctx, m.EventID)
		if retries >= seckillMaxRetries {
			// 不可恢复失败：回补库存（少卖 1 件，可接受），跳过消息避免分区卡死
			logger.Error(ctx, "seckill order create failed after max retries, restoring stock",
				zap.String("event_id", m.EventID), zap.Int64("item_id", m.ItemID), zap.Error(err))
			if ok, rerr := c.restorer.Restore(ctx, m.ItemID, m.Quantity); rerr != nil || !ok {
				logger.Error(ctx, "seckill stock restore failed",
					zap.Int64("item_id", m.ItemID), zap.Error(rerr))
			}
			c.redis.Del(ctx, fmt.Sprintf(seckillRetryKeyFmt, m.EventID))
			return nil
		}
		return err // 未超限，返回 error 触发 Kafka 重投
	}

	logger.Info(ctx, "seckill order created",
		zap.String("event_id", m.EventID), zap.String("order_no", orderNo))

	// 建单成功后写 Redis 缓存，前端轮询接口直接读此 key，避免打 DB。
	// 写失败不影响订单本身（DB 已有），仅丢失快路径查询能力，有订单列表兜底。
	if err := c.redis.Set(ctx, fmt.Sprintf(seckillOrderKeyFmt, m.EventID), orderNo, seckillOrderTTL).Err(); err != nil {
		logger.Error(ctx, "seckill order cache write failed",
			zap.String("event_id", m.EventID), zap.String("order_no", orderNo), zap.Error(err))
	}
	return nil
}

// incrRetry 累加重试次数，返回当前值。
func (c *SeckillOrderConsumer) incrRetry(ctx context.Context, eventID string) int {
	key := fmt.Sprintf(seckillRetryKeyFmt, eventID)
	val, err := c.redis.Incr(ctx, key).Result()
	if err != nil {
		return 0 // Redis 故障时保守返回 0，走重投逻辑
	}
	c.redis.Expire(ctx, key, seckillDedupTTL)
	return int(val)
}
