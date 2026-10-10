package seckill

import (
	"context"
	"sync"
	"time"
)

// activityCacheTTL 缓存兜底过期：活动时间窗口是静态数据，显式失效为主，TTL 防遗漏（如多实例）。
const activityCacheTTL = 5 * time.Second

type cachedActivity struct {
	activity *SeckillActivity
	items    []SeckillItem
	expireAt time.Time
}

// ActivityCache 进程内活动缓存：洪峰下把活动状态校验挡在 MySQL 之外，
// 是「内存 → Redis Lua → MySQL」漏斗的第一层。
type ActivityCache struct {
	repo  SeckillRepository
	mu    sync.RWMutex
	items map[int64]*cachedActivity
}

func NewActivityCache(repo SeckillRepository) *ActivityCache {
	return &ActivityCache{repo: repo, items: make(map[int64]*cachedActivity)}
}

// Get 返回活动与商品明细，未命中或过期时回源 MySQL 并写回。
func (c *ActivityCache) Get(ctx context.Context, activityID int64) (*SeckillActivity, []SeckillItem, error) {
	c.mu.RLock()
	cached, ok := c.items[activityID]
	c.mu.RUnlock()
	if ok && time.Now().Before(cached.expireAt) {
		return cached.activity, cached.items, nil
	}
	act, err := c.repo.FindActivityByID(ctx, activityID)
	if err != nil {
		return nil, nil, err
	}
	items, err := c.repo.FindItemsByActivityID(ctx, activityID)
	if err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	c.items[activityID] = &cachedActivity{activity: act, items: items, expireAt: time.Now().Add(activityCacheTTL)}
	c.mu.Unlock()
	return act, items, nil
}

// Invalidate 显式失效：活动开关变更后调用，秒级生效。
func (c *ActivityCache) Invalidate(activityID int64) {
	c.mu.Lock()
	delete(c.items, activityID)
	c.mu.Unlock()
}
