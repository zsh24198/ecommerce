package apperror

// 错误码分段（与 HTTP 状态码解耦）：
// 1000-1999 通用客户端错误，2000-2999 认证授权，3000-3999 服务端内部，
// 4000-4999 第三方依赖，5000-5999 业务逻辑，6000-6999 限流熔断。
const (
	CodeParamInvalid = 1001 // 参数校验失败
	CodeBodyParse    = 1002 // 请求体解析失败

	CodeTokenInvalid = 2001 // token 无效
	CodeTokenExpired = 2002 // token 过期
	CodeForbidden    = 2003 // 权限不足

	CodeDBConnFailed = 3001 // 数据库连接失败
	CodeUnknown      = 3002 // 未知错误

	CodeRedisTimeout = 4001 // Redis 超时
	CodeKafkaProduce = 4002 // Kafka 投递失败

	CodeStockNotEnough = 5001 // 库存不足
	CodeOrderIllegal   = 5002 // 订单状态非法
	CodeUserExists     = 5003 // 用户已存在
	CodeUserNotFound   = 5004 // 用户不存在

	CodeRateLimited = 6001 // 触发限流
	CodeDegraded    = 6002 // 服务降级
)
