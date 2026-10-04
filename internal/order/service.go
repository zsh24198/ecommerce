package order

import (
	"context"
	"fmt"
	"time"

	"github.com/zsh24198/ecommerce/internal/product"
	"github.com/zsh24198/ecommerce/shared/apperror"
)

// CreateOrderReq 下单请求 DTO。
// 注意：不传金额，金额由服务端根据 SKU 真实单价计算，客户端不可信。
type CreateOrderReq struct {
	Items         []OrderItemReq `json:"items"         binding:"required,min=1,max=100,dive"`
	ReceiverName  string         `json:"receiver_name" binding:"required,max=64"`
	ReceiverPhone string         `json:"receiver_phone" binding:"required,len=11"`
	ReceiverAddr  string         `json:"receiver_addr"  binding:"required,max=512"`
}

// OrderItemReq 单条下单商品。
type OrderItemReq struct {
	SKUID    int64 `json:"sku_id"    binding:"required,gt=0"`
	Quantity int   `json:"quantity"  binding:"required,gt=0,lt=10000"`
}

// OrderService 订单模块业务接口。
type OrderService interface {
	// CreateOrder 创建订单（含扣库存），返回订单号。
	CreateOrder(ctx context.Context, userID int64, req *CreateOrderReq) (string, error)
}

type orderService struct {
	repo    OrderRepository
	product product.ProductService // 跨域调用：通过 product.Service 接口，不碰 product.Repository
}

func NewOrderService(repo OrderRepository, productSvc product.ProductService) OrderService {
	return &orderService{repo: repo, product: productSvc}
}

// CreateOrder 下单主流程：
// 1. 遍历商品，逐个查 SKU 拿真实单价与商品名（快照）
// 2. 服务端计算总金额
// 3. 组装订单与明细
// 4. 事务内扣库存 + 写订单 + 写明细
func (s *orderService) CreateOrder(ctx context.Context, userID int64, req *CreateOrderReq) (string, error) {
	var totalAmount int64
	items := make([]OrderItem, 0, len(req.Items))
	skuStocks := make([]SkuStockItem, 0, len(req.Items))

	for _, it := range req.Items {
		// 跨域读：走 product.Service 接口，拿真实单价
		sku, err := s.product.GetSKUByID(ctx, it.SKUID)
		if err != nil {
			return "", err
		}
		if sku.Status != product.SKUStatusEnabled {
			return "", apperror.ErrSkuNotFound
		}

		subtotal := sku.PriceCents * int64(it.Quantity)
		totalAmount += subtotal

		// 订单明细：存商品名快照 + 单价快照
		items = append(items, OrderItem{
			SKUID:      sku.ID,
			SKUName:    sku.Name,
			PriceCents: sku.PriceCents,
			Quantity:   it.Quantity,
			Subtotal:   subtotal,
		})
		skuStocks = append(skuStocks, SkuStockItem{SKUID: sku.ID, Quantity: it.Quantity})
	}

	order := &Order{
		OrderNo:       genOrderNo(),
		UserID:        userID,
		TotalAmount:   totalAmount,
		Status:        OrderStatusPending,
		ReceiverName:  req.ReceiverName,
		ReceiverPhone: req.ReceiverPhone,
		ReceiverAddr:  req.ReceiverAddr,
	}

	if err := s.repo.CreateOrderWithItems(ctx, order, items, skuStocks); err != nil {
		return "", err
	}
	return order.OrderNo, nil
}

// genOrderNo 生成业务订单号：年月日时分秒 + 6位随机数。
// 演示用简单方案；生产环境应用雪花算法，保证分布式唯一且趋势递增。
func genOrderNo() string {
	return fmt.Sprintf("%s%06d", time.Now().Format("20060102150405"), time.Now().UnixNano()%1000000)
}
