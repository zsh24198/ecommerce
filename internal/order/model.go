package order

import (
	"time"

	"gorm.io/gorm"
)

// OrderStatus 订单状态。
type OrderStatus uint8

const (
	OrderStatusPending  OrderStatus = 1 // 待支付
	OrderStatusPaid     OrderStatus = 2 // 已支付
	OrderStatusShipped  OrderStatus = 3 // 已发货
	OrderStatusDone     OrderStatus = 4 // 已完成
	OrderStatusCanceled OrderStatus = 5 // 已取消
)

// Order 订单主表：一个用户一次下单生成一条。
type Order struct {
	ID            int64          `gorm:"column:id;primaryKey;autoIncrement"`
	OrderNo       string         `gorm:"column:order_no;type:varchar(32);not null;uniqueIndex:uk_orders_order_no"`
	UserID        int64          `gorm:"column:user_id;type:bigint unsigned;not null;index:idx_orders_user_id"`
	TotalAmount   int64          `gorm:"column:total_amount;type:bigint;not null;comment:总金额（分）"`
	Status        OrderStatus    `gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_orders_status"`
	ReceiverName  string         `gorm:"column:receiver_name;type:varchar(64);not null;default:''"`
	ReceiverPhone string         `gorm:"column:receiver_phone;type:varchar(20);not null;default:''"`
	ReceiverAddr  string         `gorm:"column:receiver_addr;type:varchar(512);not null;default:''"`
	CreatedAt     time.Time      `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;type:datetime(3)"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(3);index:idx_orders_deleted_at"`
}

func (Order) TableName() string {
	return "orders"
}

// OrderItem 订单明细表：一条订单对应多条明细，存商品快照。
type OrderItem struct {
	ID         int64          `gorm:"column:id;primaryKey;autoIncrement"`
	OrderID    int64          `gorm:"column:order_id;type:bigint unsigned;not null;index:idx_order_items_order_id"`
	SKUID      int64          `gorm:"column:sku_id;type:bigint unsigned;not null;index:idx_order_items_sku_id"`
	SKUName    string         `gorm:"column:sku_name;type:varchar(128);not null;comment:商品名快照"`
	PriceCents int64          `gorm:"column:price_cents;type:bigint;not null;comment:单价快照（分）"`
	Quantity   int            `gorm:"column:quantity;type:int;not null;default:1"`
	Subtotal   int64          `gorm:"column:subtotal;type:bigint;not null;comment:小计（分）"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt  time.Time      `gorm:"column:updated_at;type:datetime(3)"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(3)"`
}

func (OrderItem) TableName() string {
	return "order_items"
}
