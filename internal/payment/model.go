package payment

import (
	"time"

	"gorm.io/gorm"
)

// PaymentStatus 支付状态。
type PaymentStatus uint8

const (
	PaymentStatusPending PaymentStatus = 1 // 待支付
	PaymentStatusSuccess PaymentStatus = 2 // 支付成功
	PaymentStatusFailed  PaymentStatus = 3 // 支付失败
)

// PaymentChannel 支付渠道。
type PaymentChannel uint8

const (
	PaymentChannelWechat PaymentChannel = 1 // 微信支付
	PaymentChannelAlipay PaymentChannel = 2 // 支付宝
)

// Payment 支付流水表：一笔支付对应一条记录。
// 核心约束：transaction_id 唯一索引 → 回调幂等兜底。
type Payment struct {
	ID            int64          `gorm:"column:id;primaryKey;autoIncrement"`
	PaymentNo     string         `gorm:"column:payment_no;type:varchar(32);not null;uniqueIndex:uk_payments_payment_no"`
	OrderNo       string         `gorm:"column:order_no;type:varchar(32);not null;index:idx_payments_order_no"`
	UserID        int64          `gorm:"column:user_id;type:bigint unsigned;not null;index:idx_payments_user_id"`
	Amount        int64          `gorm:"column:amount;type:bigint;not null;comment:支付金额（分）"`
	Channel       PaymentChannel `gorm:"column:channel;type:tinyint unsigned;not null"`
	Status        PaymentStatus  `gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_payments_status"`
	TransactionID string         `gorm:"column:transaction_id;type:varchar(64);not null;default:'';uniqueIndex:uk_payments_transaction_id"`
	PaidAt        *time.Time     `gorm:"column:paid_at;type:datetime(3)"`
	CallbackRaw   string         `gorm:"column:callback_raw;type:text;comment:回调原始数据"`
	CreatedAt     time.Time      `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;type:datetime(3)"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(3);index:idx_payments_deleted_at"`
}

func (Payment) TableName() string {
	return "payments"
}
