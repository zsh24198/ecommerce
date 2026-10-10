package seckill

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/zsh24198/ecommerce/internal/product"
	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/kafka"
	"github.com/zsh24198/ecommerce/shared/logger"
)

// ActivityStatus 活动状态：由 service 动态推导（先判开关，再判时间窗口），不落库。
type ActivityStatus uint8

const (
	ActivityStatusDisabled ActivityStatus = 0 // 运营手动禁用
	ActivityStatusUpcoming ActivityStatus = 1 // 未开始
	ActivityStatusOngoing  ActivityStatus = 2 // 进行中
	ActivityStatusEnded    ActivityStatus = 3 // 已结束
)

// CreateSeckillItemReq 创建活动的商品子项。价格单位一律为"分"。
type CreateSeckillItemReq struct {
	SKUID             int64 `json:"sku_id"              binding:"required,gt=0"`
	SeckillPriceCents int64 `json:"seckill_price_cents" binding:"required,gt=0"`
	Stock             int   `json:"stock"               binding:"required,gte=1"`
	LimitPerUser      int   `json:"limit_per_user"      binding:"required,gte=1"`
}

// CreateActivityReq 创建活动请求：活动 + 商品一次配齐（方案 A）。
// 时间用 "yyyy-MM-dd HH:mm:ss" 格式字符串，由 service 层解析，对运营后台更友好。
type CreateActivityReq struct {
	Name      string                 `json:"name"       binding:"required,max=128"`
	StartTime string                 `json:"start_time" binding:"required,len=19"`
	EndTime   string                 `json:"end_time"   binding:"required,len=19"`
	Items     []CreateSeckillItemReq `json:"items"      binding:"required,min=1,max=100,dive"`
}

// SeckillItemDTO 秒杀商品明细（含三快照）。
type SeckillItemDTO struct {
	ID                 int64  `json:"id"`
	SKUID              int64  `json:"sku_id"`
	SKUNameSnapshot    string `json:"sku_name"`
	OriginalPriceCents int64  `json:"original_price_cents"`
	SeckillPriceCents  int64  `json:"seckill_price_cents"`
	Stock              int    `json:"stock"`
	LimitPerUser       int    `json:"limit_per_user"`
}

// ActivityDetail 活动详情：基础信息 + 动态状态 + 全部商品明细。
type ActivityDetail struct {
	ID        int64            `json:"id"`
	Name      string           `json:"name"`
	StartTime time.Time        `json:"start_time"`
	EndTime   time.Time        `json:"end_time"`
	Enabled   bool             `json:"enabled"`
	Status    ActivityStatus   `json:"status"`
	Items     []SeckillItemDTO `json:"items"`
}

// ActivityListItem 列表页条目：不带明细。
type ActivityListItem struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time"`
	Enabled   bool           `json:"enabled"`
	Status    ActivityStatus `json:"status"`
}

// SeckillService 秒杀模块业务接口。
type SeckillService interface {
	// CreateActivity 创建活动（校验 + 快照 + 事务落库），返回详情。
	CreateActivity(ctx context.Context, req *CreateActivityReq) (*ActivityDetail, error)
	// GetActivity 活动详情（含动态状态与明细）。
	GetActivity(ctx context.Context, id int64) (*ActivityDetail, error)
	// ListActivities 活动分页列表（运营视角，含全部状态）。
	ListActivities(ctx context.Context, page, size int) ([]ActivityListItem, int64, error)
	// ListOngoingActivities 进行中活动列表（秒杀首页视角）。
	ListOngoingActivities(ctx context.Context) ([]ActivityListItem, error)
	// UpdateActivityEnabled 运营开关：一键禁用/启用活动。
	UpdateActivityEnabled(ctx context.Context, id int64, enabled bool) error
	// DoSeckill 秒杀下单：内存校验 + Redis 预扣 + Kafka 异步建单。
	DoSeckill(ctx context.Context, userID, activityID int64, req *DoSeckillReq) error
}

const activityTimeLayout = "2006-01-02 15:04:05"

type seckillService struct {
	repo     SeckillRepository
	product  product.ProductService
	cache    *ActivityCache
	stock    *StockStore
	producer *kafka.Producer
}

// NewSeckillService 构造函数。跨域读 SKU 走 product.Service 接口，不碰 product.Repository。
func NewSeckillService(repo SeckillRepository, productSvc product.ProductService, cache *ActivityCache, stock *StockStore, producer *kafka.Producer) SeckillService {
	return &seckillService{repo: repo, product: productSvc, cache: cache, stock: stock, producer: producer}
}

// CreateActivity 创建活动：时间校验 → 逐 SKU 校验+快照 → 重叠校验 → 事务落库。
// 校验均为纯读操作、创建为低频运营操作，不上分布式锁；
// 理论上的 check-then-act 竞态由唯一索引兜底同活动重复，跨活动重叠靠运营规范兜底。
func (s *seckillService) CreateActivity(ctx context.Context, req *CreateActivityReq) (*ActivityDetail, error) {
	start, err := time.ParseInLocation(activityTimeLayout, req.StartTime, time.Local)
	if err != nil {
		return nil, apperror.New(apperror.CodeParamInvalid, "开始时间格式错误，应为 yyyy-MM-dd HH:mm:ss")
	}
	end, err := time.ParseInLocation(activityTimeLayout, req.EndTime, time.Local)
	if err != nil {
		return nil, apperror.New(apperror.CodeParamInvalid, "结束时间格式错误，应为 yyyy-MM-dd HH:mm:ss")
	}
	if !start.Before(end) {
		return nil, apperror.New(apperror.CodeParamInvalid, "结束时间必须晚于开始时间")
	}

	act := &SeckillActivity{Name: req.Name, StartTime: start, EndTime: end, Enabled: true}
	items := make([]SeckillItem, 0, len(req.Items))
	skuIDs := make([]int64, 0, len(req.Items))
	for i := range req.Items {
		// 跨域读：通过 product.Service 拿真实数据填快照，顺便校验 SKU 存在且可售
		sku, err := s.product.GetSKUByID(ctx, req.Items[i].SKUID)
		if err != nil {
			return nil, err
		}
		if req.Items[i].SeckillPriceCents >= sku.PriceCents {
			return nil, apperror.New(apperror.CodeSeckillPriceInvalid,
				fmt.Sprintf("SKU %d 秒杀价必须低于现售价 %d 分", sku.ID, sku.PriceCents))
		}
		items = append(items, SeckillItem{
			SKUID:              sku.ID,
			SKUNameSnapshot:    sku.Name,
			OriginalPriceCents: sku.OriginalPriceCents,
			SeckillPriceCents:  req.Items[i].SeckillPriceCents,
			Stock:              req.Items[i].Stock,
			LimitPerUser:       req.Items[i].LimitPerUser,
		})
		skuIDs = append(skuIDs, sku.ID)
	}

	overlapped, err := s.repo.FindOverlappingSKUs(ctx, skuIDs, start, end)
	if err != nil {
		return nil, err
	}
	if len(overlapped) > 0 {
		return nil, apperror.New(apperror.CodeSeckillSKUBusy,
			fmt.Sprintf("SKU %v 在该时间段已被其他启用中活动占用", overlapped))
	}

	if err := s.repo.CreateActivityWithItems(ctx, act, items); err != nil {
		return nil, err
	}
	// 复用详情查询组装返回，避免两套 DTO 组装逻辑（对齐 product.CreateProduct 风格）
	return s.GetActivity(ctx, act.ID)
}

// statusOf 动态推导活动状态：先判开关再判时间。
// 时间窗口语义为左闭右开 [start, end)：now >= end 即已结束，与 SQL 的 end_time > now 保持一致。
func statusOf(a *SeckillActivity, now time.Time) ActivityStatus {
	if !a.Enabled {
		return ActivityStatusDisabled
	}
	switch {
	case now.Before(a.StartTime):
		return ActivityStatusUpcoming
	case !now.Before(a.EndTime):
		return ActivityStatusEnded
	default:
		return ActivityStatusOngoing
	}
}

// GetActivity 详情 = 基础信息 + 动态状态 + 全部明细。
func (s *seckillService) GetActivity(ctx context.Context, id int64) (*ActivityDetail, error) {
	act, err := s.repo.FindActivityByID(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.FindItemsByActivityID(ctx, id)
	if err != nil {
		return nil, err
	}
	return buildDetail(act, items), nil
}

// ListActivities 运营视角分页，全状态可见。
func (s *seckillService) ListActivities(ctx context.Context, page, size int) ([]ActivityListItem, int64, error) {
	acts, total, err := s.repo.ListActivities(ctx, page, size)
	if err != nil {
		return nil, 0, err
	}
	now := time.Now()
	list := make([]ActivityListItem, 0, len(acts))
	for i := range acts {
		list = append(list, ActivityListItem{
			ID: acts[i].ID, Name: acts[i].Name,
			StartTime: acts[i].StartTime, EndTime: acts[i].EndTime,
			Enabled: acts[i].Enabled, Status: statusOf(&acts[i], now),
		})
	}
	return list, total, nil
}

// ListOngoingActivities 秒杀首页视角：仅启用中且窗口内的活动。
func (s *seckillService) ListOngoingActivities(ctx context.Context) ([]ActivityListItem, error) {
	acts, err := s.repo.ListOngoingActivities(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	list := make([]ActivityListItem, 0, len(acts))
	for i := range acts {
		list = append(list, ActivityListItem{
			ID: acts[i].ID, Name: acts[i].Name,
			StartTime: acts[i].StartTime, EndTime: acts[i].EndTime,
			Enabled: acts[i].Enabled, Status: ActivityStatusOngoing,
		})
	}
	return list, nil
}

// UpdateActivityEnabled 运营开关：秒杀的紧急刹车。
func (s *seckillService) UpdateActivityEnabled(ctx context.Context, id int64, enabled bool) error {
	if err := s.repo.UpdateActivityEnabled(ctx, id, enabled); err != nil {
		return err
	}
	s.cache.Invalidate(id)
	if enabled {
		_, items, err := s.cache.Get(ctx, id)
		if err != nil {
			return err
		}
		for i := range items {
			if err := s.stock.Warmup(ctx, items[i].ID, items[i].Stock); err != nil {
				return err
			}
		}
	}
	return nil
}

// DoSeckillReq 秒杀下单请求。
type DoSeckillReq struct {
	ItemID   int64 `json:"item_id"  binding:"required,gt=0"`
	Quantity int   `json:"quantity" binding:"required,min=1,max=100"`
}

// KafkaTopicSeckillOrder 秒杀下单事件 topic，消费端（任务 21）订阅。
const KafkaTopicSeckillOrder = "seckill-order"

// SeckillOrderEvent 秒杀下单事件：快照随消息走，消费端建单无需回查秒杀表。
type SeckillOrderEvent struct {
	EventID           string `json:"event_id"`
	UserID            int64  `json:"user_id"`
	ActivityID        int64  `json:"activity_id"`
	ItemID            int64  `json:"item_id"`
	SKUID             int64  `json:"sku_id"`
	SKUName           string `json:"sku_name"`
	Quantity          int    `json:"quantity"`
	SeckillPriceCents int64  `json:"seckill_price_cents"`
}

// DoSeckill 秒杀下单：内存校验 → Redis Lua 预扣 → 同步发 Kafka 异步建单。
// 预扣成功即受理成功；发送失败立即回补（宁可少卖，绝不超卖）。
func (s *seckillService) DoSeckill(ctx context.Context, userID, activityID int64, req *DoSeckillReq) error {
	act, items, err := s.cache.Get(ctx, activityID)
	if err != nil {
		return err
	}
	if !act.Enabled || statusOf(act, time.Now()) != ActivityStatusOngoing {
		return apperror.New(apperror.CodeSeckillNotOngoing, "活动未开始或已结束")
	}
	var item *SeckillItem
	for i := range items {
		if items[i].ID == req.ItemID {
			item = &items[i]
			break
		}
	}
	if item == nil {
		return apperror.New(apperror.CodeParamInvalid, "商品不在该秒杀活动中")
	}
	if req.Quantity > item.LimitPerUser {
		return apperror.New(apperror.CodeParamInvalid, "超过单用户限购数量")
	}
	if err := s.stock.Deduct(ctx, req.ItemID, req.Quantity); err != nil {
		return err
	}
	evt := SeckillOrderEvent{
		EventID:           uuid.NewString(),
		UserID:            userID,
		ActivityID:        activityID,
		ItemID:            item.ID,
		SKUID:             item.SKUID,
		SKUName:           item.SKUNameSnapshot,
		Quantity:          req.Quantity,
		SeckillPriceCents: item.SeckillPriceCents,
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		_, _ = s.stock.Restore(ctx, item.ID, req.Quantity)
		return apperror.Wrap(apperror.CodeUnknown, "marshal seckill order event", err)
	}
	key := strconv.FormatInt(userID, 10)
	if _, _, err := s.producer.SendMessage(KafkaTopicSeckillOrder, key, payload); err != nil {
		if ok, rerr := s.stock.Restore(ctx, item.ID, req.Quantity); rerr != nil || !ok {
			logger.Error(ctx, "seckill stock restore failed", zap.Int64("item_id", item.ID), zap.Error(rerr))
		}
		return apperror.Wrap(apperror.CodeKafkaProduce, "抢购请求受理失败，请稍后重试", err)
	}
	return nil
}

// buildDetail 组装详情 DTO，状态推导时间统一取一次 now，保证同一请求内状态一致。
func buildDetail(act *SeckillActivity, items []SeckillItem) *ActivityDetail {
	now := time.Now()
	d := &ActivityDetail{
		ID: act.ID, Name: act.Name,
		StartTime: act.StartTime, EndTime: act.EndTime,
		Enabled: act.Enabled, Status: statusOf(act, now),
		Items: make([]SeckillItemDTO, 0, len(items)),
	}
	for i := range items {
		d.Items = append(d.Items, SeckillItemDTO{
			ID:                 items[i].ID,
			SKUID:              items[i].SKUID,
			SKUNameSnapshot:    items[i].SKUNameSnapshot,
			OriginalPriceCents: items[i].OriginalPriceCents,
			SeckillPriceCents:  items[i].SeckillPriceCents,
			Stock:              items[i].Stock,
			LimitPerUser:       items[i].LimitPerUser,
		})
	}
	return d
}
