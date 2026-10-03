package product

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/middleware"
)

// UpdateSPUStatusReq 上下架请求体。
// 注意：Status 不能加 required，因为 0 是合法业务值（下架），
// go-playground/validator 的 required 对数字零值会判空，导致传 0 永远校验失败。
type UpdateSPUStatusReq struct {
	Status int `json:"status" binding:"oneof=0 1"`
}

// Handler 持有 ProductService 依赖，商品相关 HTTP 入口挂在它上面。
type Handler struct {
	svc ProductService
}

// NewHandler 构造函数，由 main 注入 service。
func NewHandler(svc ProductService) *Handler {
	return &Handler{svc: svc}
}

// CreateProduct POST /api/v1/products
func (h *Handler) CreateProduct(c *gin.Context) {
	var req CreateProductReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}
	detail, err := h.svc.CreateProduct(c.Request.Context(), &req)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, detail)
}

// GetProductDetail GET /api/v1/products/:id
func (h *Handler) GetProductDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "商品ID非法"))
		return
	}
	detail, err := h.svc.GetProductDetail(c.Request.Context(), id)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, detail)
}

// ListProducts GET /api/v1/products?page=1&size=20
func (h *Handler) ListProducts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	items, total, err := h.svc.ListProducts(c.Request.Context(), page, size)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, gin.H{"list": items, "total": total, "page": page, "size": size})
}

// UpdateSPUStatus PUT /api/v1/products/:id/status
func (h *Handler) UpdateSPUStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "商品ID非法"))
		return
	}
	var req UpdateSPUStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}
	if err := h.svc.UpdateSPUStatus(c.Request.Context(), id, SPUStatus(req.Status)); err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, nil)
}
