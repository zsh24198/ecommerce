package user

import (
	"time"

	"gorm.io/gorm"
)

type UserStatus uint8

const (
	UserStatusActive   UserStatus = 1
	UserStatusBanned   UserStatus = 2
	UserStatusInactive UserStatus = 3
)

type User struct {
	ID           int64          `gorm:"column:id;primaryKey;autoIncrement"`
	Phone        string         `gorm:"column:phone;type:varchar(20);not null;uniqueIndex:uk_users_phone"`
	PasswordHash string         `gorm:"column:password_hash;type:varchar(100);not null"`
	Status       UserStatus     `gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_users_status"`
	CreatedAt    time.Time      `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;type:datetime(3)"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(3);index:idx_users_deleted_at"`
}

func (User) TableName() string {
	return "users"
}

// RefreshToken refresh_tokens 表模型。只存 token 的 SHA-256 哈希，不存原文。
type RefreshToken struct {
	ID        int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    int64      `gorm:"column:user_id;type:bigint unsigned;not null;index:idx_refresh_tokens_user_id"`
	TokenHash string     `gorm:"column:token_hash;type:char(64);not null;uniqueIndex:uk_refresh_tokens_hash"`
	ExpiresAt time.Time  `gorm:"column:expires_at;type:datetime(3);not null"`
	RevokedAt *time.Time `gorm:"column:revoked_at;type:datetime(3)"`
	CreatedAt time.Time  `gorm:"column:created_at;type:datetime(3)"`
}

func (RefreshToken) TableName() string {
	return "refresh_tokens"
}
