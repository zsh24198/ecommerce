package order

import (
	"context"

	"gorm.io/gorm"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

// OrderRepository 订单数据访问接口。
type OrderRepository interface {
	// CreateOrderWithItems 在事务内完成：扣库存 → 写订单主表 → 写订单明细。
	// skuItems 是已校验过的 SKU 列表（含真实单价与商品名快照），任一步失败整体回滚。
	CreateOrderWithItems(ctx context.Context, order *Order, items []OrderItem, skuItems []SkuStockItem) error
}

// SkuStockItem 扣库存所需的 SKU 信息：由 service 层从 product 模块查询后传入。
// 注意：order 模块不 import product.Model，用这个本地结构体传递必要字段。
type SkuStockItem struct {
	SKUID    int64
	Quantity int
}

type orderRepo struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepo{db: db}
}

// CreateOrderWithItems 事务编排：扣库存 + 写订单 + 写明细。
// 扣库存使用单条 SQL 条件更新（WHERE stock >= qty），数据库层面保证原子性，杜绝超卖。
func (r *orderRepo) CreateOrderWithItems(ctx context.Context, order *Order, items []OrderItem, skuItems []SkuStockItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 逐个扣库存：原子操作，受影响行数为 0 表示库存不足
		for _, si := range skuItems {
			res := tx.Table("skus").
				Where("id = ? AND stock >= ?", si.SKUID, si.Quantity).
				Update("stock", gorm.Expr("stock - ?", si.Quantity))
			if res.Error != nil {
				return apperror.Wrap(apperror.CodeUnknown, "扣减库存失败", res.Error)
			}
			if res.RowsAffected == 0 {
				return apperror.ErrStockNotEnough
			}
		}

		// 2. 写订单主表（GORM 回填自增 ID 到 order.ID）
		if err := tx.Create(order).Error; err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "创建订单失败", err)
		}

		// 3. 写订单明细：回填 order_id 后批量插入
		for i := range items {
			items[i].OrderID = order.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "创建订单明细失败", err)
		}

		return nil
	})
}
