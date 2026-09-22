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
