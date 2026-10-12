package seckill

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zsh24198/ecommerce/shared/cache"
)

// activityCacheTTL 缓存兜底过期：活动时间窗口是静态数据，显式失效为主，TTL 防遗漏（如多实例）。
// 叠加 ±10% 抖动防雪崩：避免大量活动 key 在同一时刻过期，洪峰瞬间全打 DB。
const activityCacheTTL = 5 * time.Second

type cachedActivity struct {
	activity *SeckillActivity
	items    []SeckillItem
	expireAt time.Time
}

// ActivityCache 进程内活动缓存：洪峰下把活动状态校验挡在 MySQL 之外，
// 是「内存 → Redis Lua → MySQL」漏斗的第一层。
// 三层防护：
//   - JitterTTL：防雪崩，过期时间 ±10% 抖动
//   - SingleFlight：防击穿，缓存 miss 时同 key 并发只回源一次
//   - 显式 Invalidate：活动开关变更时主动失效，秒级生效
type ActivityCache struct {
	repo  SeckillRepository
	sf    *cache.SingleFlight
	mu    sync.RWMutex
	items map[int64]*cachedActivity
}

func NewActivityCache(repo SeckillRepository, sf *cache.SingleFlight) *ActivityCache {
	return &ActivityCache{repo: repo, sf: sf, items: make(map[int64]*cachedActivity)}
}

// Get 返回活动与商品明细，未命中或过期时回源 MySQL 并写回。
// 防击穿：缓存 miss 时同 activityID 并发只回源一次，其他等待者共享结果。
// 第一个请求失败时所有等待者都拿到错误，由 ctx 超时兜底逃生。
func (c *ActivityCache) Get(ctx context.Context, activityID int64) (*SeckillActivity, []SeckillItem, error) {
	c.mu.RLock()
	cached, ok := c.items[activityID]
	c.mu.RUnlock()
	if ok && time.Now().Before(cached.expireAt) {
		return cached.activity, cached.items, nil
	}
	key := fmt.Sprintf("seckill:activity:%d", activityID)
	val, err, _ := c.sf.Do(ctx, key, func() (interface{}, error) {
		return c.loadFromRepo(ctx, activityID)
	})
	if err != nil {
		return nil, nil, err
	}
	ca := val.(*cachedActivity)
	return ca.activity, ca.items, nil
}

// loadFromRepo 回源 MySQL 加载活动与明细，组装为 cachedActivity 并写回缓存。
// 抽出来便于 SingleFlight 包裹和单元测试时直接测 DB 查询逻辑。
// 写回用 JitterTTL：基础 5s + ±10% 抖动，防大量活动 key 同时过期雪崩。
func (c *ActivityCache) loadFromRepo(ctx context.Context, activityID int64) (*cachedActivity, error) {
	act, err := c.repo.FindActivityByID(ctx, activityID)
	if err != nil {
		return nil, err
	}
	items, err := c.repo.FindItemsByActivityID(ctx, activityID)
	if err != nil {
		return nil, err
	}
	ca := &cachedActivity{activity: act, items: items, expireAt: cache.ExpireAt(activityCacheTTL)}
	c.mu.Lock()
	c.items[activityID] = ca
	c.mu.Unlock()
	return ca, nil
}

// Invalidate 显式失效：活动开关变更后调用，秒级生效。
func (c *ActivityCache) Invalidate(activityID int64) {
	c.mu.Lock()
	delete(c.items, activityID)
	c.mu.Unlock()
}
