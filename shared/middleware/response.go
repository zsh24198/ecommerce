package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zsh24198/ecommerce/shared/apperror"
)

// OK 成功响应统一信封
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": data})
}

// Fail 失败响应：任意 error 经 FromError 兜底 → 统一信封
func Fail(c *gin.Context, err error) {
	appErr := apperror.FromError(err)
	c.JSON(httpStatusOf(appErr.Code), gin.H{"code": appErr.Code, "msg": appErr.Msg})
}

// httpStatusOf 错误码段 → HTTP 状态码（粗映射，规则变了只改这里）
func httpStatusOf(code int) int {
	switch {
	case code >= 6000:
		return http.StatusTooManyRequests
	case code >= 4000:
		return http.StatusBadGateway
	case code >= 3000:
		return http.StatusInternalServerError
	case code >= 2000:
		return http.StatusUnauthorized
	case code >= 1000:
		return http.StatusBadRequest
	default:
		return http.StatusOK // 5000 段业务错误走这里
	}
}
