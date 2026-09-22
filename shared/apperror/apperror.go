// Package apperror 定义项目统一的业务错误类型与错误码。
package apperror

import (
	"errors"
	"fmt"
)

// AppError 是项目统一的业务错误类型。
type AppError struct {
	Code int    // 对外错误码，如 5001
	Msg  string // 给前端/用户看的可读信息
	err  error  // 内部错误，仅供日志使用，禁止返回给客户端
}

// 常用业务错误的哨兵实例，全局共享，禁止修改字段。
var (
	ErrUserNotFound = &AppError{Code: CodeUserNotFound, Msg: "用户不存在"}
	ErrUserExists   = &AppError{Code: CodeUserExists, Msg: "用户已存在"}
	ErrInternal     = &AppError{Code: CodeUnknown, Msg: "系统繁忙，请稍后再试"}
)

// Error 返回带错误码的完整描述，仅供日志使用，禁止返回给客户端。
func (e *AppError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("code: %d, msg: %s, err: %v", e.Code, e.Msg, e.err)
	}
	return fmt.Sprintf("code: %d, msg: %s", e.Code, e.Msg)
}

// Unwrap 返回内部错误，使 errors.Is/As 能穿透 AppError 检查根因。
func (e *AppError) Unwrap() error {
	return e.err
}

// Is 按 Code 判断是否为同类错误，使不同实例的同码错误可判等。
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	return ok && e.Code == t.Code
}

// New 创建一个不带内部错误的业务错误。
func New(code int, msg string) *AppError {
	return &AppError{Code: code, Msg: msg}
}

// Wrap 包装一个内部错误，内部错误仅供日志使用，禁止返回给客户端。
func Wrap(code int, msg string, err error) *AppError {
	return &AppError{Code: code, Msg: msg, err: err}
}

// FromError 将任意错误转换为 AppError；非 AppError 统一兜底为 CodeUnknown，
// 原始错误被保留在内部，仅供日志使用。
func FromError(err error) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return Wrap(CodeUnknown, "系统繁忙，请稍后再试", err)
}
