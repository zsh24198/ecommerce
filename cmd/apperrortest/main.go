package main

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

func main() {
	// 1. 哨兵判等：同一个指针
	fmt.Println(errors.Is(apperror.ErrUserNotFound, apperror.ErrUserNotFound))

	// 2. 同码不同实例：Is 按 Code 判等 → true
	fresh := apperror.New(apperror.CodeUserNotFound, "用户不存在")
	fmt.Println(errors.Is(fresh, apperror.ErrUserNotFound))

	// 3. 包装后穿透到根因
	wrapped := apperror.Wrap(apperror.CodeDBConnFailed, "系统繁忙", sql.ErrConnDone)
	fmt.Println(errors.Is(wrapped, sql.ErrConnDone))

	// 4. 非 AppError 兜底
	fallback := apperror.FromError(errors.New("某第三方库的错误"))
	fmt.Println(fallback.Code, fallback.Msg)
	fmt.Println(fallback) // Error() 含原始错误，供日志排查

	// 5. nil 安全
	fmt.Println(apperror.FromError(nil))
}
