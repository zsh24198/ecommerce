package cache

import (
	"math/rand"
	"time"
)

// JitterTTL 在基础 TTL 上叠加 ±10% 随机抖动，防止大量 key 同时过期引发雪崩。
// 公式：base + rand(0, base/10)
// 不用 rand(0, base) 是为了避免期望 TTL 腰斩、缓存命中率崩塌。
func JitterTTL(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	jitter := time.Duration(rand.Int63n(int64(base) / 10))
	return base + jitter
}

// ExpireAt 基于当前时间和抖动 TTL 计算绝对过期时刻。
func ExpireAt(base time.Duration) time.Time {
	return time.Now().Add(JitterTTL(base))
}
