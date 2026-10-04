package order

import (
	"context"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

// OrderRepository 订单数据访问接口。
type OrderRepository interface {
	// CreateOrderWithItems 在事务内完成：扣库存 → 写订单主表 → 写订单明细。
	// skuItems 是已校验过的 SKU 列表（含真实单价与商品名快照），任一步失败整体回滚。
	CreateOrderWithItems(ctx context.Context, order *Order, items []OrderItem, skuItems []SkuStockItem) error

	// GetOrderByOrderNo 按订单号查询订单，未找到返回 nil（供 payment 模块跨域读）。
	GetOrderByOrderNo(ctx context.Context, orderNo string) (*Order, error)
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

// CreateOrderWithItems 事务编排：写订单（唯一索引判重）→ 扣库存 → 写明细。
// 扣库存使用单条 SQL 条件更新（WHERE stock >= qty），数据库层面保证原子性，杜绝超卖。
// 若命中 idempotency_key 唯一索引（1062），说明订单已存在，回填已有订单号后返回成功（幂等兜底）。
func (r *orderRepo) CreateOrderWithItems(ctx context.Context, order *Order, items []OrderItem, skuItems []SkuStockItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 先写订单主表：利用 (user_id, idempotency_key) 唯一索引做幂等兜底
		if err := tx.Create(order).Error; err != nil {
			if isDuplicateKeyErr(err) {
				// 同用户同幂等键的订单已存在，回填已有订单号，不扣库存（之前已扣过）
				existing := &Order{}
				if err := tx.Where("user_id = ? AND idempotency_key = ? AND deleted_at IS NULL",
					order.UserID, order.IdempotencyKey).First(existing).Error; err != nil {
					return apperror.Wrap(apperror.CodeUnknown, "查询已有订单失败", err)
				}
				order.ID = existing.ID
				order.OrderNo = existing.OrderNo
				return nil
			}
			return apperror.Wrap(apperror.CodeUnknown, "创建订单失败", err)
		}

		// 2. 逐个扣库存：原子操作，受影响行数为 0 表示库存不足
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

// GetOrderByOrderNo 按订单号查询订单，未找到返回 nil + nil。
func (r *orderRepo) GetOrderByOrderNo(ctx context.Context, orderNo string) (*Order, error) {
	o := &Order{}
	if err := r.db.WithContext(ctx).Where("order_no = ? AND deleted_at IS NULL", orderNo).First(o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询订单失败", err)
	}
	return o, nil
}

// isDuplicateKeyErr 判断是否为 MySQL 唯一索引冲突错误（1062）。
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
