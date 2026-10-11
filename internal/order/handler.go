package order

import (
	"github.com/gin-gonic/gin"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/middleware"
)

// Handler 持有 OrderService 依赖。
type Handler struct {
	svc OrderService
}

// NewHandler 构造函数，由 main 注入 service。
func NewHandler(svc OrderService) *Handler {
	return &Handler{svc: svc}
}

// CreateOrder 下单接口。需登录（由 JWT 中间件保证）。
// 幂等键 Idempotency-Key 从 HTTP Header 传入，由前端生成 UUID。
// 返回订单号，前端用订单号去调支付接口。
func (h *Handler) CreateOrder(c *gin.Context) {
	var req CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}

	// 幂等键：前端生成，放在 HTTP Header
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "缺少 Idempotency-Key 请求头"))
		return
	}

	userID := middleware.CurrentUserID(c)
	if userID == 0 {
		middleware.Fail(c, apperror.New(apperror.CodeTokenInvalid, "未登录"))
		return
	}

	orderNo, err := h.svc.CreateOrder(c.Request.Context(), userID, idempotencyKey, &req)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, gin.H{"order_no": orderNo})
}

// SeckillOrderStatusReq 秒杀订单状态查询请求。
type SeckillOrderStatusReq struct {
	EventID string `form:"event_id" binding:"required"`
}

// GetSeckillOrderStatus 秒杀订单状态轮询接口。
// 前端抢购拿到 event_id 后，用此接口轮询订单是否已由消费者异步创建。
func (h *Handler) GetSeckillOrderStatus(c *gin.Context) {
	var req SeckillOrderStatusReq
	if err := c.ShouldBindQuery(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}
	userID := middleware.CurrentUserID(c)
	if userID == 0 {
		middleware.Fail(c, apperror.New(apperror.CodeTokenInvalid, "未登录"))
		return
	}
	orderNo, status, err := h.svc.GetSeckillOrderByEventID(c.Request.Context(), userID, req.EventID)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, gin.H{"order_no": orderNo, "status": status})
}

// FillReceiverInfoReq 补填收货信息请求。
type FillReceiverInfoReq struct {
	ReceiverName  string `json:"receiver_name"  binding:"required,max=64"`
	ReceiverPhone string `json:"receiver_phone" binding:"required,len=11"`
	ReceiverAddr  string `json:"receiver_addr"  binding:"required,max=512"`
}

// FillReceiverInfo 秒杀单补填收货信息，将订单从 PendingInfo 推进到 Pending。
func (h *Handler) FillReceiverInfo(c *gin.Context) {
	var req FillReceiverInfoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}
	orderNo := c.Param("order_no")
	if orderNo == "" {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "缺少订单号"))
		return
	}
	userID := middleware.CurrentUserID(c)
	if userID == 0 {
		middleware.Fail(c, apperror.New(apperror.CodeTokenInvalid, "未登录"))
		return
	}
	if err := h.svc.FillReceiverInfo(c.Request.Context(), userID, orderNo, req.ReceiverName, req.ReceiverPhone, req.ReceiverAddr); err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, nil)
}
