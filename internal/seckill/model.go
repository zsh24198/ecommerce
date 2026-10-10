// /root/ecommerce/internal/seckill/model.go
package seckill

import (
	"time"

	"gorm.io/gorm"
)

// SeckillActivity 秒杀活动：时间窗口 + 运营开关双重控制状态。
// 状态推导规则：enabled==true AND now ∈ [start_time, end_time] 才"进行中"。
type SeckillActivity struct {
	ID        int64          `gorm:"column:id;primaryKey;autoIncrement"`
	Name      string         `gorm:"column:name;type:varchar(128);not null"`
	StartTime time.Time      `gorm:"column:start_time;type:datetime(3);not null;index:idx_seckill_activities_start_time"`
	EndTime   time.Time      `gorm:"column:end_time;type:datetime(3);not null"`
	Enabled   bool           `gorm:"column:enabled;type:tinyint unsigned;not null;default:1;index:idx_seckill_activities_enabled"`
	CreatedAt time.Time      `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:datetime(3)"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(3);index:idx_seckill_activities_deleted_at"`
}

func (SeckillActivity) TableName() string {
	return "seckill_activities"
}

// SeckillItem 秒杀商品明细：活动×SKU 维度，独立库存 + 三快照（名称/原价/秒杀价）。
// 注意：stock 是秒杀专属配额，与 skus.stock 完全独立，互不影响。
type SeckillItem struct {
	ID                 int64          `gorm:"column:id;primaryKey;autoIncrement"`
	ActivityID         int64          `gorm:"column:activity_id;type:bigint unsigned;not null;index:idx_seckill_items_activity_id"`
	SKUID              int64          `gorm:"column:sku_id;type:bigint unsigned;not null;index:idx_seckill_items_sku_id"`
	SKUNameSnapshot    string         `gorm:"column:sku_name_snapshot;type:varchar(128);not null"`
	OriginalPriceCents int64          `gorm:"column:original_price_cents;type:bigint;not null"`
	SeckillPriceCents  int64          `gorm:"column:seckill_price_cents;type:bigint;not null"`
	Stock              int            `gorm:"column:stock;type:int;not null;default:0"`
	LimitPerUser       int            `gorm:"column:limit_per_user;type:int;not null;default:1"`
	CreatedAt          time.Time      `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;type:datetime(3)"`
	DeletedAt          gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(3);index:idx_seckill_items_deleted_at"`
}

func (SeckillItem) TableName() string {
	return "seckill_items"
}
