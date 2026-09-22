package apperror_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

func TestErrorsIs(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{"哨兵与自身判等", apperror.ErrUserNotFound, apperror.ErrUserNotFound, true},
		{"同码不同实例判等", apperror.New(apperror.CodeUserNotFound, "用户不存在"), apperror.ErrUserNotFound, true},
		{"不同码不相等", apperror.ErrUserExists, apperror.ErrUserNotFound, false},
		{"包装后穿透根因", apperror.Wrap(apperror.CodeDBConnFailed, "系统繁忙", sql.ErrConnDone), sql.ErrConnDone, true},
		{"目标不是 AppError", apperror.ErrUserNotFound, sql.ErrConnDone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, errors.Is(tt.err, tt.target))
		})
	}
}

func TestFromError(t *testing.T) {
	t.Run("AppError 原样返回", func(t *testing.T) {
		err := apperror.New(apperror.CodeUserExists, "用户已存在")
		assert.Equal(t, err, apperror.FromError(err))
	})

	t.Run("非 AppError 兜底且保留原始错误", func(t *testing.T) {
		raw := errors.New("某第三方库的错误")
		got := apperror.FromError(raw)
		require.NotNil(t, got)
		assert.Equal(t, apperror.CodeUnknown, got.Code)
		assert.Equal(t, "系统繁忙，请稍后再试", got.Msg)
		assert.ErrorIs(t, got, raw)
	})

	t.Run("nil 返回 nil", func(t *testing.T) {
		assert.Nil(t, apperror.FromError(nil))
	})
}

func TestErrorString(t *testing.T) {
	t.Run("无内部错误", func(t *testing.T) {
		err := apperror.New(apperror.CodeStockNotEnough, "库存不足")
		assert.Equal(t, "code: 5001, msg: 库存不足", err.Error())
	})

	t.Run("有内部错误", func(t *testing.T) {
		err := apperror.Wrap(apperror.CodeDBConnFailed, "系统繁忙", sql.ErrConnDone)
		assert.Contains(t, err.Error(), "err: sql: connection is already closed")
	})
}
