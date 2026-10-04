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
// 返回订单号，前端用订单号去调支付接口。
func (h *Handler) CreateOrder(c *gin.Context) {
	var req CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}

	userID := middleware.CurrentUserID(c)
	if userID == 0 {
		middleware.Fail(c, apperror.New(apperror.CodeTokenInvalid, "未登录"))
		return
	}

	orderNo, err := h.svc.CreateOrder(c.Request.Context(), userID, &req)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, gin.H{"order_no": orderNo})
}
