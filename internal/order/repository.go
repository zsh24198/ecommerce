package order

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

// OrderRepository 订单数据访问接口。
type OrderRepository interface {
	// CreateOrderWithItems 在事务内完成：写订单主表 → 扣库存 → 写订单明细 → 写 outbox 事件。
	// skuItems 是已校验过的 SKU 列表（含真实单价与商品名快照），任一步失败整体回滚。
	CreateOrderWithItems(ctx context.Context, order *Order, items []OrderItem, skuItems []SkuStockItem) error

	// GetOrderByOrderNo 按订单号查询订单，未找到返回 nil（供 payment 模块跨域读）。
	GetOrderByOrderNo(ctx context.Context, orderNo string) (*Order, error)

	// GetOrderByIdempotencyKey 按 user_id + idempotency_key 查询订单，未找到返回 nil。
	// 秒杀场景用 event_id 作为 idempotency_key，前端轮询时据此判断订单是否已建。
	GetOrderByIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (*Order, error)

	// UpdateReceiverInfo 更新收货信息并推进状态：仅 PendingInfo → Pending 单向推进。
	// WHERE 带旧状态做乐观锁，防并发重复推进。返回受影响行数（0 表示状态不匹配）。
	UpdateReceiverInfo(ctx context.Context, orderNo, name, phone, addr string) (int64, error)

	// ListExpiredOrders 查超时未支付订单：PendingInfo 超过 infoCutoff 或 Pending 超过 payCutoff。
	// 配合 idx_orders_status_changed_at 复合索引，按状态变更时间升序返回（越早越先超时）。
	// limit 限制单批大小，避免一次扫描压垮 DB。
	ListExpiredOrders(ctx context.Context, infoCutoff, payCutoff time.Time, limit int) ([]Order, error)

	// UpdateStatusIfMatch 原子推进状态：WHERE status IN (from) 乐观锁抢独占权。
	// 返回受影响行数：1 表示抢到独占权可执行回补，0 表示已被其他实例处理。
	// 同时刷新 status_changed_at，保证下次扫描该订单不在超时窗口。
	UpdateStatusIfMatch(ctx context.Context, orderNo string, from []OrderStatus, to OrderStatus) (int64, error)

	// RestockNormalItems 普通单回补：按 order_no JOIN order_items 拿到 SKU 列表后
	// 逐条 UPDATE skus SET stock = stock + qty WHERE id = sku_id。
	// 无 WHERE stock 条件，回补永远安全（库存只增不减，不会超卖）。
	RestockNormalItems(ctx context.Context, orderNo string) error

	// SumOrderItemQuantity 按 order_no 查订单明细总数量，秒杀单回补时使用。
	// 秒杀单只有一个 SKU 明细，返回值即为该 SKU 购买件数。
	SumOrderItemQuantity(ctx context.Context, orderNo string) (int, error)
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

		// 4. 同事务写 outbox 事件：订单创建成功则事件必然落库，失败则一起回滚。
		// 仅真正新建时写（幂等命中路径已在上方提前返回，避免产生重复事件）。
		eventItems := make([]orderItemEventPayload, 0, len(items))
		for _, it := range items {
			eventItems = append(eventItems, orderItemEventPayload{
				SKUID: it.SKUID, SKUName: it.SKUName,
				PriceCents: it.PriceCents, Quantity: it.Quantity, Subtotal: it.Subtotal,
			})
		}
		payload, err := json.Marshal(orderCreatedEventPayload{
			OrderNo:     order.OrderNo,
			UserID:      order.UserID,
			TotalAmount: order.TotalAmount,
			Items:       eventItems,
		})
		if err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "序列化订单事件失败", err)
		}
		if err := tx.Exec(
			"INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload) VALUES (?, ?, ?, ?)",
			"order", order.OrderNo, "order.created", string(payload),
		).Error; err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "写入 outbox 事件失败", err)
		}

		return nil
	})
}

// orderCreatedEventPayload order.created 事件消息体：业务字段快照，消费方无需反查业务库。
type orderCreatedEventPayload struct {
	OrderNo     string                  `json:"order_no"`
	UserID      int64                   `json:"user_id"`
	TotalAmount int64                   `json:"total_amount"`
	Items       []orderItemEventPayload `json:"items"`
}

type orderItemEventPayload struct {
	SKUID      int64  `json:"sku_id"`
	SKUName    string `json:"sku_name"`
	PriceCents int64  `json:"price_cents"`
	Quantity   int    `json:"quantity"`
	Subtotal   int64  `json:"subtotal"`
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

// GetOrderByIdempotencyKey 按 user_id + idempotency_key 查询订单，未找到返回 nil。
func (r *orderRepo) GetOrderByIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (*Order, error) {
	o := &Order{}
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND idempotency_key = ? AND deleted_at IS NULL", userID, idempotencyKey).
		First(o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询订单失败", err)
	}
	return o, nil
}

// UpdateReceiverInfo 更新收货信息并推进状态：仅 PendingInfo(6) → Pending(1)。
// WHERE 带旧状态做乐观锁，返回受影响行数（0 表示状态不匹配，可能已被推进或订单非秒杀单）。
func (r *orderRepo) UpdateReceiverInfo(ctx context.Context, orderNo, name, phone, addr string) (int64, error) {
	res := r.db.WithContext(ctx).Model(&Order{}).
		Where("order_no = ? AND status = ? AND deleted_at IS NULL", orderNo, OrderStatusPendingInfo).
		Updates(map[string]interface{}{
			"receiver_name":     name,
			"receiver_phone":    phone,
			"receiver_addr":     addr,
			"status":            OrderStatusPending,
			"status_changed_at": time.Now(),
		})
	if res.Error != nil {
		return 0, apperror.Wrap(apperror.CodeUnknown, "更新收货信息失败", res.Error)
	}
	return res.RowsAffected, nil
}

// ListExpiredOrders 实现见接口注释。SQL 同时处理两个状态的超时判定：
// status=PendingInfo AND status_changed_at < infoCutoff
// OR status=Pending AND status_changed_at < payCutoff
// 用 UNION ALL 合并两个索引扫描，性能优于 OR（OR 会导致索引失效）。
func (r *orderRepo) ListExpiredOrders(ctx context.Context, infoCutoff, payCutoff time.Time, limit int) ([]Order, error) {
	var orders []Order
	err := r.db.WithContext(ctx).Raw(`
		SELECT * FROM orders
		WHERE deleted_at IS NULL
		  AND ( (status = ? AND status_changed_at < ?)
		     OR (status = ? AND status_changed_at < ?) )
		ORDER BY status_changed_at ASC
		LIMIT ?`,
		OrderStatusPendingInfo, infoCutoff,
		OrderStatusPending, payCutoff,
		limit,
	).Scan(&orders).Error
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询超时订单失败", err)
	}
	return orders, nil
}

// UpdateStatusIfMatch 乐观锁抢独占权：WHERE status IN (from) 失败则受影响 0。
func (r *orderRepo) UpdateStatusIfMatch(ctx context.Context, orderNo string, from []OrderStatus, to OrderStatus) (int64, error) {
	res := r.db.WithContext(ctx).Model(&Order{}).
		Where("order_no = ? AND status IN ? AND deleted_at IS NULL", orderNo, from).
		Updates(map[string]interface{}{
			"status":            to,
			"status_changed_at": time.Now(),
		})
	if res.Error != nil {
		return 0, apperror.Wrap(apperror.CodeUnknown, "推进订单状态失败", res.Error)
	}
	return res.RowsAffected, nil
}

// RestockNormalItems 普通单回补：按 order_no JOIN order_items 拿 SKU 列表，逐条加回库存。
// 逐条而非批量 IN，因为不同 SKU 可能位于不同行页，批量 UPDATE 会锁更多行。
// 单条 SQL 失败不中断：已回补的 SKU 不会因后续失败而撤销（库存只增不减）。
func (r *orderRepo) RestockNormalItems(ctx context.Context, orderNo string) error {
	var items []OrderItem
	if err := r.db.WithContext(ctx).
		Where("order_id = (SELECT id FROM orders WHERE order_no = ?)", orderNo).
		Find(&items).Error; err != nil {
		return apperror.Wrap(apperror.CodeUnknown, "查询订单明细失败", err)
	}
	for _, it := range items {
		if err := r.db.WithContext(ctx).Table("skus").
			Where("id = ?", it.SKUID).
			Update("stock", gorm.Expr("stock + ?", it.Quantity)).Error; err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "回补普通库存失败", err)
		}
	}
	return nil
}

// SumOrderItemQuantity 按 order_no JOIN order_items 聚合订单明细数量总和。
// 秒杀单仅一条明细，直接 SUM(quantity) 取值。
func (r *orderRepo) SumOrderItemQuantity(ctx context.Context, orderNo string) (int, error) {
	var qty int
	err := r.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(oi.quantity), 0)
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		WHERE o.order_no = ? AND oi.deleted_at IS NULL AND o.deleted_at IS NULL`,
		orderNo,
	).Scan(&qty).Error
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeUnknown, "查询订单数量失败", err)
	}
	return qty, nil
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
