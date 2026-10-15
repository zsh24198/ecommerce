package agent

import (
	"github.com/gin-gonic/gin"

	"github.com/zsh24198/ecommerce/internal/order"
	"github.com/zsh24198/ecommerce/internal/product"
	"github.com/zsh24198/ecommerce/internal/user"
	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/middleware"
)

// Handler 持有各业务域 Service 接口，不直接依赖 repository/model。
type Handler struct {
	userSvc    user.UserService
	productSvc product.ProductService
	orderSvc   order.OrderService
}

// NewHandler 构造函数，依赖由 main.go 注入。
func NewHandler(u user.UserService, p product.ProductService, o order.OrderService) *Handler {
	return &Handler{userSvc: u, productSvc: p, orderSvc: o}
}

// QueryOrder 按订单号查询，校验订单归属后返回。
func (h *Handler) QueryOrder(c *gin.Context) {
	var req QueryOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "参数错误: "+err.Error()))
		return
	}
	info, err := h.orderSvc.GetOrderInfoByNo(c.Request.Context(), req.OrderNo)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	// Agent 传的 user_id 不可信，必须校验订单归属，防越权。
	if info.UserID != req.UserID {
		middleware.Fail(c, apperror.New(apperror.CodeForbidden, "无权查看该订单"))
		return
	}
	middleware.OK(c, AgentOrderDTO{
		OrderNo:     info.OrderNo,
		UserID:      info.UserID,
		TotalAmount: info.TotalAmount,
		Status:      int(info.Status),
	})
}

// QueryProduct 按 SPU ID 查询商品详情。
func (h *Handler) QueryProduct(c *gin.Context) {
	var req QueryProductReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "参数错误: "+err.Error()))
		return
	}
	detail, err := h.productSvc.GetProductDetail(c.Request.Context(), req.SpuID)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	skus := make([]AgentSKUDTO, 0, len(detail.SKUs))
	for _, s := range detail.SKUs {
		skus = append(skus, AgentSKUDTO{ID: s.ID, PriceCents: s.PriceCents, Stock: s.Stock})
	}
	middleware.OK(c, AgentProductDTO{
		ID:     detail.ID,
		Name:   detail.Name,
		Status: detail.Status,
		SKUs:   skus,
	})
}

// QueryUser 按用户 ID 查询脱敏信息。
func (h *Handler) QueryUser(c *gin.Context) {
	var req QueryUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "参数错误: "+err.Error()))
		return
	}
	info, err := h.userSvc.GetUserByID(c.Request.Context(), req.UserID)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, AgentUserDTO{ID: info.ID, Phone: info.Phone, Status: int(info.Status)})
}
