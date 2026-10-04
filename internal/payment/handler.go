package payment

import (
	"github.com/gin-gonic/gin"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/middleware"
)

// Handler 持有 PaymentService 依赖。
type Handler struct {
	svc PaymentService
}

// NewHandler 构造函数，由 main 注入 service。
func NewHandler(svc PaymentService) *Handler {
	return &Handler{svc: svc}
}

// CreatePayment 创建支付单接口。需登录（JWT 中间件保证）。
// 前端传入订单号和支付渠道，返回支付流水号，前端用它去调起支付。
func (h *Handler) CreatePayment(c *gin.Context) {
	var req CreatePaymentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}

	userID := middleware.CurrentUserID(c)
	if userID == 0 {
		middleware.Fail(c, apperror.New(apperror.CodeTokenInvalid, "未登录"))
		return
	}

	paymentNo, err := h.svc.CreatePayment(c.Request.Context(), userID, &req)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, gin.H{"payment_no": paymentNo})
}

// Callback 支付异步回调接口（POST /api/v1/payments/callback）。
// 由微信/支付宝服务器主动调用，无需登录。
//
// 关键设计：无论成功失败，都返回 HTTP 200 + 约定格式，避免第三方无限重试。
// 成功返回 {"code": "SUCCESS"}，失败返回 {"code": "FAIL", "msg": "..."}。
func (h *Handler) Callback(c *gin.Context) {
	var req CallbackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数解析失败，返回 FAIL 让第三方重试
		c.JSON(200, gin.H{"code": "FAIL", "msg": "参数解析失败"})
		return
	}

	if err := h.svc.HandleCallback(c.Request.Context(), &req); err != nil {
		// 业务失败（验签失败、金额不匹配等），返回 FAIL
		// 注意：幂等重复回调会返回 nil（成功），不会走到这里
		c.JSON(200, gin.H{"code": "FAIL", "msg": err.Error()})
		return
	}

	// 处理成功，返回 SUCCESS 告知第三方不再重试
	c.JSON(200, gin.H{"code": "SUCCESS"})
}
