package seckill

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/redis"
)

const stockKeyPrefix = "seckill:stock:"

// deductScript 原子预扣：读库存 → 判断 → 扣减，三步在 Redis 单线程内一次性完成，无竞态窗口。
// 返回约定：1 扣减成功 | 0 已售罄 | -1 库存未预热（fail-closed，拒绝请求而非伪装售罄）
var deductScript = goredis.NewScript(`
local stock = tonumber(redis.call('GET', KEYS[1]))
if stock == nil then
	return -1
end
if stock < tonumber(ARGV[1]) then
	return 0
end
redis.call('DECRBY', KEYS[1], ARGV[1])
return 1
`)

// restoreScript 条件回补：仅当库存 key 存在时才 INCRBY。
// key 丢失时返回 -1 拒绝盲补，防止凭空造出假库存，交由告警 + 对账修复。
var restoreScript = goredis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
	return redis.call('INCRBY', KEYS[1], ARGV[1])
end
return -1
`)

func stockKey(itemID int64) string {
	return fmt.Sprintf("%s%d", stockKeyPrefix, itemID)
}

// StockStore 秒杀库存 Redis 读写组件。
// key 格式与返回码均为秒杀域知识，放本模块而非 shared/redis。
type StockStore struct {
	rdb *redis.Client
}

// NewStockStore 构造函数。
func NewStockStore(rdb *redis.Client) *StockStore {
	return &StockStore{rdb: rdb}
}

// Warmup 预热：活动开启时把 MySQL 库存写入 Redis。
// SetNX 保证幂等（重复调用不覆盖已扣减的值），TTL 0 永不过期。
func (s *StockStore) Warmup(ctx context.Context, itemID int64, stock int) error {
	_, err := s.rdb.SetNX(ctx, stockKey(itemID), stock, 0).Result()
	if err != nil {
		return apperror.Wrap(apperror.CodeRedisError, "warmup seckill stock failed", err)
	}
	return nil
}

// Deduct 原子预扣 qty 件。售罄/未预热返回对应业务错误码，全程不触碰 MySQL。
func (s *StockStore) Deduct(ctx context.Context, itemID int64, qty int) error {
	res, err := deductScript.Run(ctx, s.rdb, []string{stockKey(itemID)}, qty).Int()
	if err != nil {
		return apperror.Wrap(apperror.CodeRedisError, "deduct seckill stock failed", err)
	}
	switch res {
	case -1:
		return apperror.New(apperror.CodeSeckillStockNotReady, "秒杀库存未就绪，请稍后再试")
	case 0:
		return apperror.New(apperror.CodeSeckillSoldOut, "秒杀商品已售罄")
	}
	return nil
}

// Restore 回补 qty 件。返回 false 表示 key 丢失未回补，调用方必须记录告警。
func (s *StockStore) Restore(ctx context.Context, itemID int64, qty int) (bool, error) {
	res, err := restoreScript.Run(ctx, s.rdb, []string{stockKey(itemID)}, qty).Int()
	if err != nil {
		return false, apperror.Wrap(apperror.CodeRedisError, "restore seckill stock failed", err)
	}
	return res >= 0, nil
}
