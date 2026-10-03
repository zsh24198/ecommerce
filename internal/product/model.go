package product

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SPUStatus SPU 上下架状态。
type SPUStatus uint8

const (
	SPUStatusOffline SPUStatus = 0 // 下架
	SPUStatusOnline  SPUStatus = 1 // 上架
)

// SKUStatus SKU 启用状态。
type SKUStatus uint8

const (
	SKUStatusDisabled SKUStatus = 0 // 禁用
	SKUStatusEnabled  SKUStatus = 1 // 启用
)

// SPU 标准产品单元：存一款商品所有规格共享的共性信息，不含价格。
type SPU struct {
	ID          int64          `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string         `gorm:"column:name;type:varchar(128);not null;index:idx_spus_name"`
	CategoryID  int64          `gorm:"column:category_id;type:bigint unsigned;not null;default:0;index:idx_spus_category_id"`
	BrandID     int64          `gorm:"column:brand_id;type:bigint unsigned;not null;default:0"`
	MainImage   string         `gorm:"column:main_image;type:varchar(512);not null;default:''"`
	Images      datatypes.JSON `gorm:"column:images;type:json"`
	Description string         `gorm:"column:description;type:text"`
	Status      SPUStatus      `gorm:"column:status;type:tinyint unsigned;not null;default:0;index:idx_spus_status"`
	Sort        int            `gorm:"column:sort;type:int;not null;default:0"`
	CreatedAt   time.Time      `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;type:datetime(3)"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(3);index:idx_spus_deleted_at"`
}

func (SPU) TableName() string {
	return "spus"
}

// SKU 库存量单位：具体可售规格，价格（分）、库存、规格都挂在 SKU 上。
type SKU struct {
	ID                 int64          `gorm:"column:id;primaryKey;autoIncrement"`
	SPUID              int64          `gorm:"column:spu_id;type:bigint unsigned;not null;index:idx_skus_spu_id"`
	Name               string         `gorm:"column:name;type:varchar(128);not null"`
	Specs              datatypes.JSON `gorm:"column:specs;type:json"`
	PriceCents         int64          `gorm:"column:price_cents;type:bigint;not null"`
	OriginalPriceCents int64          `gorm:"column:original_price_cents;type:bigint;not null;default:0"`
	Stock              int            `gorm:"column:stock;type:int;not null;default:0"`
	Image              string         `gorm:"column:image;type:varchar(512);not null;default:''"`
	Status             SKUStatus      `gorm:"column:status;type:tinyint unsigned;not null;default:1"`
	CreatedAt          time.Time      `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;type:datetime(3)"`
	DeletedAt          gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(3);index:idx_skus_deleted_at"`
}

func (SKU) TableName() string {
	return "skus"
}
