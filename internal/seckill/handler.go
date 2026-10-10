// /root/ecommerce/internal/seckill/handler.go
package seckill

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/middleware"
)

// UpdateActivityEnabledReq 运营开关请求体。
// Enabled 用指针接收：false 是合法业务值，validator 的 required 对 bool 零值判空，
// 直接用 bool 会导致传 false 永远校验失败（product 模块 UpdateSPUStatusReq 同款教训）。
type UpdateActivityEnabledReq struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// Handler 持有 SeckillService 依赖，秒杀相关 HTTP 入口挂在它上面。
type Handler struct {
	svc SeckillService
}

// NewHandler 构造函数，由 main 注入 service。
func NewHandler(svc SeckillService) *Handler {
	return &Handler{svc: svc}
}

// CreateActivity POST /api/v1/seckill/activities
func (h *Handler) CreateActivity(c *gin.Context) {
	var req CreateActivityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}
	detail, err := h.svc.CreateActivity(c.Request.Context(), &req)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, detail)
}

// GetActivity GET /api/v1/seckill/activities/:id
func (h *Handler) GetActivity(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "活动ID非法"))
		return
	}
	detail, err := h.svc.GetActivity(c.Request.Context(), id)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, detail)
}

// ListActivities GET /api/v1/seckill/activities?page=1&size=20
func (h *Handler) ListActivities(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	list, total, err := h.svc.ListActivities(c.Request.Context(), page, size)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, gin.H{"list": list, "total": total, "page": page, "size": size})
}

// ListOngoingActivities GET /api/v1/seckill/ongoing
func (h *Handler) ListOngoingActivities(c *gin.Context) {
	list, err := h.svc.ListOngoingActivities(c.Request.Context())
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, gin.H{"list": list})
}

// UpdateActivityEnabled PUT /api/v1/seckill/activities/:id/enabled
func (h *Handler) UpdateActivityEnabled(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, "活动ID非法"))
		return
	}
	var req UpdateActivityEnabledReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}
	if err := h.svc.UpdateActivityEnabled(c.Request.Context(), id, *req.Enabled); err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, nil)
}
