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
	OrderStatusDone        OrderStatus = 4 // 已完成
	OrderStatusCanceled    OrderStatus = 5 // 已取消
	OrderStatusPendingInfo OrderStatus = 6 // 待补充收货信息（秒杀单专用，补全后进入 Pending）
)

// OrderType 订单类型：区分库存回补路径。
// 普通单回补 MySQL skus.stock，秒杀单回补 Redis seckill:stock:{item_id}。
type OrderType uint8

const (
	OrderTypeNormal  OrderType = 0 // 普通订单，库存扣在 MySQL skus 表
	OrderTypeSeckill OrderType = 1 // 秒杀订单，库存扣在 Redis StockStore
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
	IdempotencyKey string        `gorm:"column:idempotency_key;type:varchar(64);not null;default:''"`
	OrderType      OrderType     `gorm:"column:order_type;type:tinyint unsigned;not null;default:0"`
	SeckillItemID  *int64        `gorm:"column:seckill_item_id;type:bigint unsigned;comment:秒杀商品 ID，仅秒杀单非空"`
	StatusChangedAt time.Time    `gorm:"column:status_changed_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3)"`
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
