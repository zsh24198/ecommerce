package order

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/zsh24198/ecommerce/internal/product"
	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/lock"
	redisclient "github.com/zsh24198/ecommerce/shared/redis"
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

// OrderInfo 订单信息 DTO，供跨域模块（如 payment）读取订单基本信息，避免暴露 model。
type OrderInfo struct {
	OrderNo     string
	UserID      int64
	TotalAmount int64
	Status      OrderStatus
}

// OrderService 订单模块业务接口。
type OrderService interface {
	// CreateOrder 创建订单（含扣库存），返回订单号。
	// idempotencyKey 由前端生成，用于防重复下单。
	CreateOrder(ctx context.Context, userID int64, idempotencyKey string, req *CreateOrderReq) (string, error)

	// GetOrderInfoByNo 按订单号查询订单基本信息，供 payment 模块创建支付单时校验订单归属与金额。
	GetOrderInfoByNo(ctx context.Context, orderNo string) (*OrderInfo, error)
}

type orderService struct {
	repo    OrderRepository
	product product.ProductService // 跨域调用：通过 product.Service 接口，不碰 product.Repository
	redis   *redisclient.Client
	locker  *lock.Manager // 分布式锁：按 SKU 粒度互斥，防并发超卖
}

func NewOrderService(repo OrderRepository, productSvc product.ProductService, rdb *redisclient.Client, locker *lock.Manager) OrderService {
	return &orderService{repo: repo, product: productSvc, redis: rdb, locker: locker}
}

// 幂等键在 Redis 中的占位符，表示首次请求正在处理中。
const idempotencyProcessing = "processing"

// 幂等键 Redis 过期时间：覆盖用户重试窗口（24h）。
const idempotencyTTL = 24 * time.Hour

// 轮询间隔与最大次数：最多等 500ms，覆盖正常订单创建耗时。
const (
	idempotencyPollInterval = 50 * time.Millisecond
	idempotencyMaxPolls     = 10
)

// CreateOrder 带幂等的下单入口：
// 1. 用 Redis SETNX 原子判断是否重复请求
// 2. 首次请求：执行业务，成功存结果，失败删占位
// 3. 重复请求：轮询等待首次请求的结果
func (s *orderService) CreateOrder(ctx context.Context, userID int64, idempotencyKey string, req *CreateOrderReq) (string, error) {
	rKey := fmt.Sprintf("idem:order:%d:%s", userID, idempotencyKey)

	// SETNX 原子占位：成功=首次请求，失败=重复请求
	ok, err := s.redis.SetNX(ctx, rKey, idempotencyProcessing, idempotencyTTL).Result()
	if err != nil {
		return "", apperror.Wrap(apperror.CodeRedisError, "redis setnx failed", err)
	}

	if !ok {
		// 重复请求：轮询等待首次请求的结果
		return s.pollIdempotencyResult(ctx, rKey)
	}

	// 首次请求：执行业务
	orderNo, err := s.createOrderCore(ctx, userID, idempotencyKey, req)
	if err != nil {
		// 业务失败：删占位，允许用户重新下单
		s.redis.Del(ctx, rKey)
		return "", err
	}

	// 业务成功：存结果（覆盖占位符），后续重复请求直接返回
	if err := s.redis.Set(ctx, rKey, orderNo, idempotencyTTL).Err(); err != nil {
		// Redis 存结果失败不影响订单（DB 已有），仅丢失快路径幂等能力
		return orderNo, nil
	}
	return orderNo, nil
}

// pollIdempotencyResult 轮询 Redis 获取首次请求的结果。
// 拿到非 processing 的值即返回；超时则提示稍后重试。
func (s *orderService) pollIdempotencyResult(ctx context.Context, rKey string) (string, error) {
	for i := 0; i < idempotencyMaxPolls; i++ {
		val, err := s.redis.Get(ctx, rKey).Result()
		if err != nil {
			return "", apperror.Wrap(apperror.CodeRedisError, "redis get failed", err)
		}
		if val != idempotencyProcessing {
			return val, nil // 拿到订单号
		}
		time.Sleep(idempotencyPollInterval)
	}
	return "", apperror.New(apperror.CodeIdempotencyConflict, "请求处理中，请稍后重试")
}

// createOrderCore 下单核心业务（不含幂等逻辑）：
// 1. 遍历商品，逐个查 SKU 拿真实单价与商品名（快照）
// 2. 服务端计算总金额
// 3. 组装订单与明细（含幂等键）
// 4. 事务内扣库存 + 写订单 + 写明细
func (s *orderService) createOrderCore(ctx context.Context, userID int64, idempotencyKey string, req *CreateOrderReq) (string, error) {
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

	// 多把锁按 SKUID 升序获取，打破"循环等待"条件防死锁；
	// 临界区只包住"扣库存+写订单"事务，查价/算价在锁外（正确性由原子 SQL 兜底）
	mus, err := s.acquireStockLocks(ctx, skuStocks)
	if err != nil {
		return "", err
	}
	defer func() {
		for _, m := range mus {
			m.Unlock(ctx) // 释放无等待，无需按序
		}
	}()

	order := &Order{
		OrderNo:        genOrderNo(),
		UserID:         userID,
		TotalAmount:    totalAmount,
		Status:         OrderStatusPending,
		ReceiverName:   req.ReceiverName,
		ReceiverPhone:  req.ReceiverPhone,
		ReceiverAddr:   req.ReceiverAddr,
		IdempotencyKey: idempotencyKey,
	}

	if err := s.repo.CreateOrderWithItems(ctx, order, items, skuStocks); err != nil {
		return "", err
	}
	return order.OrderNo, nil
}

// acquireStockLocks 按升序依次抢各 SKU 的库存锁；任何一把失败，
// 释放已持有的全部（全有或全无），不留"持有并等待"的残局。
func (s *orderService) acquireStockLocks(ctx context.Context, items []SkuStockItem) ([]*lock.Mutex, error) {
	sort.Slice(items, func(i, j int) bool { return items[i].SKUID < items[j].SKUID })
	mus := make([]*lock.Mutex, 0, len(items))
	for _, it := range items {
		mu, err := s.locker.Acquire(ctx, fmt.Sprintf("lock:stock:%d", it.SKUID))
		if err != nil {
			for _, held := range mus {
				held.Unlock(ctx)
			}
			return nil, err
		}
		mus = append(mus, mu)
	}
	return mus, nil
}

// GetOrderInfoByNo 按订单号查询订单基本信息。
func (s *orderService) GetOrderInfoByNo(ctx context.Context, orderNo string) (*OrderInfo, error) {
	o, err := s.repo.GetOrderByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, apperror.ErrOrderIllegal
	}
	return &OrderInfo{
		OrderNo:     o.OrderNo,
		UserID:      o.UserID,
		TotalAmount: o.TotalAmount,
		Status:      o.Status,
	}, nil
}

// genOrderNo 生成业务订单号：年月日时分秒 + 6位随机数。
// 演示用简单方案；生产环境应用雪花算法，保证分布式唯一且趋势递增。
func genOrderNo() string {
	return fmt.Sprintf("%s%06d", time.Now().Format("20060102150405"), time.Now().UnixNano()%1000000)
}
