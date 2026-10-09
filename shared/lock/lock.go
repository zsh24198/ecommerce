// Package lock 提供基于 Redsync 的分布式锁：
// SET NX EX 抢锁 + Lua 安全释放（value 匹配才删）+ 看门狗自动续期。
// 本包只负责"锁机制"，key 的业务语义（如 lock:stock:{sku_id}）由调用方定义。
package lock

import (
	"context"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	goredislib "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/logger"
)

// DefaultExpiry 锁默认过期时间（TTL）。
const DefaultExpiry = 30 * time.Second

// Manager 分布式锁管理器，封装 Redsync。
type Manager struct {
	rs  *redsync.Redsync
	ttl time.Duration
}

// NewManager 基于已有 go-redis 客户端创建锁管理器；ttl <= 0 时用默认 TTL。
func NewManager(rdb *goredislib.Client, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = DefaultExpiry
	}
	pool := goredis.NewPool(rdb)
	return &Manager{rs: redsync.New(pool), ttl: ttl}
}

// Mutex 一把已获取的分布式锁。Acquire 成功后调用方必须 Unlock（建议 defer）。
type Mutex struct {
	mu      *redsync.Mutex
	name    string
	ttl     time.Duration
	stop    chan struct{} // 关闭即通知看门狗退出
	stopped chan struct{} // 看门狗退出后关闭，Unlock 等待它保证顺序
}

// Acquire 抢锁。抢不到立即失败（不重试），统一转成 5014 业务错误：
// 用户侧表现为"请求繁忙请重试"（HTTP 200），真实原因保留在错误链中进日志。
func (m *Manager) Acquire(ctx context.Context, name string) (*Mutex, error) {
	mu := m.rs.NewMutex(name,
		redsync.WithExpiry(m.ttl),
		redsync.WithTries(1),
	)
	if err := mu.LockContext(ctx); err != nil {
		return nil, apperror.Wrap(apperror.CodeLockConflict, "请求过于繁忙，请稍后重试", err)
	}

	dm := &Mutex{
		mu:      mu,
		name:    name,
		ttl:     m.ttl,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go dm.watchdog()
	return dm, nil
}

// watchdog 自动续期：每 TTL/3 续一次，留两倍冗余容忍单次失败。
// Extend 内部是"value 匹配才 PEXPIRE"的 Lua 脚本，不会替别人续锁。
// 续期失败说明锁已丢失（过期/被删），停止续期，锁将在一个 TTL 内自然消亡。
func (dm *Mutex) watchdog() {
	defer close(dm.stopped)
	ticker := time.NewTicker(dm.ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-dm.stop:
			return
		case <-ticker.C:
			// v4.18 起 ExtendContext 返回 (ok, err)：锁已丢失时可能不报错（ok=false, err=nil），
			// 两个都必须检查，否则看门狗会给一把不属于我们的锁空转续期
			ok, err := dm.mu.ExtendContext(context.Background())
			if err != nil || !ok {
				logger.Warn(context.Background(), "lock renew failed, stop watchdog",
					zap.String("lock", dm.name), zap.Error(err))
				return
			}
		}
	}
}

// Unlock 释放锁：先停看门狗（防止给正在释放的锁续期），再执行安全释放。
// 释放失败只告警不返回错误：锁最迟 TTL 后自动过期，不会死锁。
func (dm *Mutex) Unlock(ctx context.Context) {
	select {
	case <-dm.stop:
	default:
		close(dm.stop)
	}
	<-dm.stopped

	if _, err := dm.mu.UnlockContext(ctx); err != nil {
		logger.Warn(ctx, "lock unlock failed, will expire by ttl",
			zap.String("lock", dm.name), zap.Error(err))
	}
}
