// Package user 的 handler 层：HTTP 请求 → DTO 校验 → 调用 service → 统一响应。
package user

import (
	"github.com/gin-gonic/gin"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/middleware"
)

// RegisterReq 注册请求体 DTO。只暴露前端可传字段，禁止直接使用 model.User。
type RegisterReq struct {
	Phone    string `json:"phone"    binding:"required,len=11"`
	Password string `json:"password" binding:"required,min=6"`
}

// LoginReq 登录请求体 DTO。
type LoginReq struct {
	Phone    string `json:"phone"    binding:"required,len=11"`
	Password string `json:"password" binding:"required,min=6"`
}

// RefreshReq 刷新 token 请求体 DTO。
type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Handler 持有 UserService 依赖，所有用户相关 HTTP 入口挂在它上面。
type Handler struct {
	svc UserService
}

// NewHandler 构造函数，由 main 注入 service。
func NewHandler(svc UserService) *Handler {
	return &Handler{svc: svc}
}

// Register 注册新用户。
func (h *Handler) Register(c *gin.Context) {
	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}

	info, err := h.svc.Register(c.Request.Context(), req.Phone, req.Password)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, info)
}

// Login 手机号+密码登录，成功返回 token 对。
func (h *Handler) Login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}

	pair, err := h.svc.Login(c.Request.Context(), req.Phone, req.Password)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, pair)
}

// RefreshToken 用 refresh token 换新 token 对。
func (h *Handler) RefreshToken(c *gin.Context) {
	var req RefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, apperror.New(apperror.CodeParamInvalid, err.Error()))
		return
	}

	pair, err := h.svc.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.OK(c, pair)
}