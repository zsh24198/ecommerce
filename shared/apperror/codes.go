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
	CodeRedisError   = 4003 // Redis 操作失败
	CodeKafkaProduce = 4002 // Kafka 投递失败

	CodeStockNotEnough = 5001 // 库存不足
	CodeOrderIllegal   = 5002 // 订单状态非法
	CodeUserExists     = 5003 // 用户已存在
	CodeUserNotFound   = 5004 // 用户不存在
	CodeUserCreds      = 5005 // 手机号或密码错误
	CodeUserBanned     = 5006 // 账号已被封禁

	CodeProductNotFound   = 5007 // 商品不存在
	CodeSkuNotFound       = 5008 // SKU 不存在或不可售
	CodeIdempotencyConflict = 5009 // 幂等冲突：请求处理中，请稍后重试

	CodePaymentNotFound    = 5010 // 支付单不存在
	CodePaymentAmountErr   = 5011 // 支付金额不匹配
	CodePaymentSignInvalid = 5012 // 支付回调签名无效
	CodeOrderNotPending    = 5013 // 订单非待支付状态，无法支付
	CodeLockConflict       = 5014 // 抢锁失败：当前请求过于繁忙，请稍后重试

	CodeSeckillActivityNotFound = 5015 // 秒杀活动不存在
	CodeSeckillSKUBusy          = 5016 // SKU 已被其他启用中活动占用（时间重叠）
	CodeSeckillPriceInvalid     = 5017 // 秒杀价非法（须低于 SKU 现售价）

	CodeRateLimited = 6001 // 触发限流
	CodeDegraded    = 6002 // 服务降级
)
