// /root/ecommerce/shared/middleware/ratelimit_test.go
package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zsh24198/ecommerce/shared/logger"
	"go.uber.org/zap"
)

// TestMain 测试入口：注入 Nop logger，避免 ratelimit.go 的 logger.Error 在 nil logger 上 panic。
// Nop logger 会丢弃所有日志输出，测试输出不被污染。
func TestMain(m *testing.M) {
	logger.SetLogger(zap.NewNop())
	os.Exit(m.Run())
}

// mockRedis 实现 tokenBucketer 接口。
// evalResult 用 int64 而非 int：go-redis Cmd.Int() 内部对 cmd.val 做类型断言只接受 int64，
// 传 int 会走 default 分支返回 error，导致限流中间件误入 fail-open 路径。
// 使用官方 NewCmdResult(val, err) 构造返回值，避免 SetVal 类型未对齐的隐患。
type mockRedis struct {
	evalResult int64
	evalError  error
	callCount  int
	lastKey    string
	lastArgs   []interface{}
}

func (m *mockRedis) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *goredis.Cmd {
	m.callCount++
	m.lastKey = keys[0]
	m.lastArgs = args
	if m.evalError != nil {
		return goredis.NewCmdResult(nil, m.evalError)
	}
	return goredis.NewCmdResult(m.evalResult, nil)
}

// runWithUser 起真实 gin.Engine，在限流中间件前注入 user_id 模拟已登录。
func runWithUser(t *testing.T, mw gin.HandlerFunc, userID int64, mock *mockRedis) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 先注入 user_id（模拟 JWT 中间件已运行）
	r.Use(func(c *gin.Context) {
		if userID > 0 {
			c.Set(string(CtxKeyUserID), userID)
		}
		c.Next()
	})
	// 再挂限流中间件
	r.Use(mw)
	r.POST("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	r.ServeHTTP(w, req)
	return w
}

// ----- 用例 1：放行分支 -----
func TestPerKeyRateLimit_Allow(t *testing.T) {
	mock := &mockRedis{evalResult: 1} // 1=放行
	mw := PerKeyRateLimit(mock, 10, 5, UserKeyFunc())

	w := runWithUser(t, mw, 1001, mock)

	assert.Equal(t, 1, mock.callCount, "Redis 应被调用 1 次")
	assert.Equal(t, "rate:bucket:user:1001", mock.lastKey)
	assert.Equal(t, http.StatusOK, w.Code, "放行应返回 200")
}

// ----- 用例 2：拒绝分支 -----
func TestPerKeyRateLimit_Deny(t *testing.T) {
	mock := &mockRedis{evalResult: 0} // 0=拒绝
	mw := PerKeyRateLimit(mock, 10, 5, UserKeyFunc())

	w := runWithUser(t, mw, 1001, mock)

	assert.Equal(t, 1, mock.callCount)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "拒绝应返回 429")
}

// ----- 用例 3：Redis 异常 fail-open -----
func TestPerKeyRateLimit_RedisFailOpen(t *testing.T) {
	mock := &mockRedis{evalError: errors.New("redis connection refused")}
	mw := PerKeyRateLimit(mock, 10, 5, UserKeyFunc())

	w := runWithUser(t, mw, 1001, mock)

	assert.Equal(t, 1, mock.callCount, "Redis 应被调用 1 次（即使失败）")
	assert.Equal(t, http.StatusOK, w.Code, "Redis 异常时应 fail-open 放行")
}

// ----- 用例 4：key 为空时直接放行 -----
func TestPerKeyRateLimit_EmptyKeyBypass(t *testing.T) {
	mock := &mockRedis{evalResult: 1} // 即使返回 1，也不应该被读到
	emptyKeyFunc := func(c *gin.Context) string { return "" }
	mw := PerKeyRateLimit(mock, 10, 5, emptyKeyFunc)

	w := runWithUser(t, mw, 0, mock)

	assert.Equal(t, 0, mock.callCount, "空 key 时 Redis 不应被调用")
	assert.Equal(t, http.StatusOK, w.Code, "空 key 应直接放行")
}

// ----- 用例 5：UserKeyFunc 未登录返回空串 -----
func TestUserKeyFunc_NoUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	// 不 Set user_id 模拟未登录

	key := UserKeyFunc()(c)
	assert.Empty(t, key, "未登录时 UserKeyFunc 应返回空串")
}

// ----- 用例 6：UserKeyFunc 已登录返回 user:xxx -----
func TestUserKeyFunc_WithUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(string(CtxKeyUserID), int64(1001))

	key := UserKeyFunc()(c)
	assert.Equal(t, "user:1001", key)
}

// ----- 用例 7：IPKeyFunc -----
func TestIPKeyFunc(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.RemoteAddr = "192.168.1.100:12345"

	key := IPKeyFunc()(c)
	assert.Equal(t, "ip:192.168.1.100", key)
}

// ----- 用例 8：Lua 参数顺序 -----
func TestPerKeyRateLimit_LuaArgsOrder(t *testing.T) {
	mock := &mockRedis{evalResult: 1} // 1=放行
	mw := PerKeyRateLimit(mock, 10, 5, UserKeyFunc())

	_ = runWithUser(t, mw, 1001, mock)

	require.Len(t, mock.lastArgs, 4, "Lua 脚本应收到 4 个 ARGV")
	assert.Equal(t, 10, mock.lastArgs[0], "ARGV[1] 应为 capacity")
	assert.Equal(t, 5, mock.lastArgs[1], "ARGV[2] 应为 refill_per_sec")
	_, ok := mock.lastArgs[2].(int64)
	assert.True(t, ok, "ARGV[3] 应为 int64 毫秒时间戳")
	assert.Equal(t, 60000, mock.lastArgs[3], "ARGV[4] 应为 TTL 60000ms")
}
