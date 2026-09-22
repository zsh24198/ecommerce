package user

import (
	"context"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

// UserRepository 定义用户数据访问接口。
// 接口化的目的：
//  1. service 层依赖接口而非实现，便于单元测试时用 mock 替换；
//  2. Phase 5 Agent 接入时可透明替换实现（如加缓存层）。
type UserRepository interface {
	// FindByPhone 按手机号查询用户，找不到返回 apperror.ErrUserNotFound。
	FindByPhone(ctx context.Context, phone string) (*User, error)
	// FindByID 按用户 ID 查询用户，找不到返回 apperror.ErrUserNotFound。
	FindByID(ctx context.Context, id int64) (*User, error)
	// Create 创建用户，phone 唯一索引冲突时返回 apperror.ErrUserExists。
	// 创建成功后自增 ID 会写回 user.ID（因为传的是指针）。
	Create(ctx context.Context, user *User) error
}

// userRepo 是 UserRepository 的 GORM 实现。
// 小写开头（包内可见），对外只暴露接口类型，隐藏实现细节。
type userRepo struct {
	db *gorm.DB
}

// NewUserRepository 构造函数，由外部注入 *gorm.DB。
// 不在内部 gorm.Open，便于测试时替换为 mock DB 或 sqlite。
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepo{db: db}
}

// FindByPhone 按手机号查询用户。
func (r *userRepo) FindByPhone(ctx context.Context, phone string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&user).Error
	if err != nil {
		// GORM 查不到记录时返回 gorm.ErrRecordNotFound，
		// 统一转换为业务错误 apperror.ErrUserNotFound。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrUserNotFound
		}
		// 其他错误（如 DB 连接断开、SQL 语法错误等）兜底为 CodeUnknown，
		// 原始错误保留在内部仅供日志，不返回给客户端。
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询用户失败", err)
	}
	return &user, nil
}

// FindByID 按用户 ID 查询用户。
func (r *userRepo) FindByID(ctx context.Context, id int64) (*User, error) {
	var user User
	// GORM 主键查询的简写：First(&user, id) 等价于 Where("id = ?", id)
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrUserNotFound
		}
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询用户失败", err)
	}
	return &user, nil
}

// Create 创建用户。
func (r *userRepo) Create(ctx context.Context, user *User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		// phone 字段有唯一索引，重复插入时 MySQL 返回 1062 错误，
		// 转换为业务错误 apperror.ErrUserExists。
		if isDuplicateKeyErr(err) {
			return apperror.ErrUserExists
		}
		return apperror.Wrap(apperror.CodeUnknown, "创建用户失败", err)
	}
	return nil
}

// isDuplicateKeyErr 判断是否为 MySQL 唯一索引冲突错误。
// 优先用错误号 1062 精确判断（不依赖错误字符串），
// 兜底用字符串匹配，兼容 GORM 对底层错误的多层包装。
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return strings.Contains(err.Error(), "Duplicate entry")
}
