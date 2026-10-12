// /root/ecommerce/shared/middleware/ratelimit.go
package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/logger"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ----- 全局限流（单机内存令牌桶）-----

// GlobalRateLimit 所有路由共享一个 limiter，保护本机资源。
func GlobalRateLimit(ratePerSec, burst int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(ratePerSec), burst)
	return func(c *gin.Context) {
		if !limiter.Allow() {
			Fail(c, apperror.New(apperror.CodeRateLimited, "当前请求过多，请稍后重试"))
			c.Abort()
			return
		}
		c.Next()
	}
}

// ----- 按维度限流（Redis 令牌桶 Lua）-----

const tokenBucketLua = `local key = KEYS[1]
local cap = tonumber(ARGV[1])
local refill = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local ttl_ms = tonumber(ARGV[4])

local b = redis.call('HMGET', key, 'tokens', 'last_ms')
local tokens = tonumber(b[1])
local last_ms = tonumber(b[2])

if tokens == nil or last_ms == nil then
	tokens = cap
	last_ms = now_ms
end

local elapsed = (now_ms - last_ms) / 1000.0
tokens = tokens + elapsed * refill
if tokens > cap then tokens = cap end

local allowed = 0
if tokens >= 1 then
	tokens = tokens - 1
	allowed = 1
end

redis.call('HMSET', key, 'tokens', tokens, 'last_ms', now_ms)
redis.call('PEXPIRE', key, ttl_ms)
return allowed`

// tokenBucketer 抽象 Redis Eval 接口，便于 mock 测试。
type tokenBucketer interface {
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *goredis.Cmd
}

// PerKeyRateLimit 按 key 维度限流：每请求由 keyFunc 提取限流 key（如 user_id 或 IP）。
// capacity: 桶容量；refillPerSec: 每秒补充令牌数。
func PerKeyRateLimit(rdb tokenBucketer, capacity, refillPerSec int, keyFunc func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFunc(c)
		if key == "" { // 无 key 时直接放行（如未登录用户的 IP 限流场景兜底）
			c.Next()
			return
		}
		fullKey := "rate:bucket:" + key
		nowMs := time.Now().UnixMilli()
		ttlMs := 60 * 1000 // 冷数据 60s 兜底过期，避免内存泄漏

		res, err := rdb.Eval(c.Request.Context(), tokenBucketLua,
			[]string{fullKey},
			capacity, refillPerSec, nowMs, ttlMs,
		).Int()
		if err != nil {
			// Redis 异常时 fail-open：限流不应阻塞正常请求，但记日志告警
			logger.Error(c.Request.Context(), "rate limit redis eval failed",
				zap.String("key", fullKey), zap.Error(err))
			c.Next()
			return
		}
		if res != 1 {
			Fail(c, apperror.New(apperror.CodeRateLimited, "请求过于频繁，请稍后重试"))
			c.Abort()
			return
		}
		c.Next()
	}
}

// ----- keyFunc 工厂：复用逻辑 -----

// IPKeyFunc 提取客户端 IP 作为限流 key 前缀。
func IPKeyFunc() func(*gin.Context) string {
	return func(c *gin.Context) string {
		return "ip:" + c.ClientIP()
	}
}

// UserKeyFunc 从 gin.Context 提取已登录 user_id 作为限流 key 前缀。
// 必须挂在 JWT 中间件之后，否则 user_id 为 0，返回空串触发放行兜底。
func UserKeyFunc() func(*gin.Context) string {
	return func(c *gin.Context) string {
		uid := CurrentUserID(c)
		if uid == 0 {
			return ""
		}
		return "user:" + strconv.FormatInt(uid, 10)
	}
}
