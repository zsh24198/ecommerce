package product

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/datatypes"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/cache"
)

// CreateProductReq 创建商品请求。价格单位一律为"分"，浮点金额禁止进入系统。
// 与 user 模块不同：请求体含嵌套结构，故 DTO 由 service 层定义（接口契约的一部分）。
type CreateProductReq struct {
	Name        string         `json:"name"        binding:"required,max=128"`
	CategoryID  int64          `json:"category_id" binding:"gte=0"`
	BrandID     int64          `json:"brand_id"    binding:"gte=0"`
	MainImage   string         `json:"main_image"  binding:"required,max=512"`
	Images      []string       `json:"images"      binding:"omitempty,max=10,dive,max=512"`
	Description string         `json:"description"`
	SKUs        []CreateSKUReq `json:"skus"        binding:"required,min=1,max=100,dive"`
}

// CreateSKUReq 创建 SKU 子项。
type CreateSKUReq struct {
	Name               string            `json:"name"                 binding:"required,max=128"`
	Specs              map[string]string `json:"specs"                binding:"required"`
	PriceCents         int64             `json:"price_cents"          binding:"required,gt=0"`
	OriginalPriceCents int64             `json:"original_price_cents" binding:"gte=0"`
	Stock              int               `json:"stock"                binding:"gte=0"`
	Image              string            `json:"image"                binding:"omitempty,max=512"`
}

// SKUDTO 详情中的规格项。
type SKUDTO struct {
	ID                 int64             `json:"id"`
	Name               string            `json:"name"`
	Specs              map[string]string `json:"specs"`
	PriceCents         int64             `json:"price_cents"`
	OriginalPriceCents int64             `json:"original_price_cents"`
	Stock              int               `json:"stock"`
	Image              string            `json:"image"`
}

// ProductDetail 商品详情：SPU 共性 + 全部启用 SKU。
type ProductDetail struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	CategoryID  int64    `json:"category_id"`
	BrandID     int64    `json:"brand_id"`
	MainImage   string   `json:"main_image"`
	Images      []string `json:"images"`
	Description string   `json:"description"`
	Status      uint8    `json:"status"`
	SKUs        []SKUDTO `json:"skus"`
}

// ProductListItem 列表页条目：SPU 共性 + 价格区间（批量聚合而来）。
type ProductListItem struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	MainImage     string `json:"main_image"`
	MinPriceCents int64  `json:"min_price_cents"`
	MaxPriceCents int64  `json:"max_price_cents"`
}

// ProductService 商品模块业务接口，供 handler 层依赖。
type ProductService interface {
	// CreateProduct 创建商品（SPU + 全部 SKU 一起落库），返回详情。
	CreateProduct(ctx context.Context, req *CreateProductReq) (*ProductDetail, error)
	// GetProductDetail 商品详情。
	GetProductDetail(ctx context.Context, spuID int64) (*ProductDetail, error)
	// ListProducts 上架商品分页列表，带每个商品的价格区间。
	ListProducts(ctx context.Context, page, size int) ([]ProductListItem, int64, error)
	// UpdateSPUStatus 上架/下架商品。
	UpdateSPUStatus(ctx context.Context, spuID int64, status SPUStatus) error
	// GetSKUByID 按 ID 查询单个 SKU，供 order 模块下单时获取真实单价与商品名快照。
	GetSKUByID(ctx context.Context, skuID int64) (*SKU, error)
}

// productDetailCacheTTL 详情缓存基础 TTL，叠加 ±10% 抖动防雪崩。
const productDetailCacheTTL = 30 * time.Second

// productService 增加三层缓存防护依赖。
// bloom：防穿透，DB 中不存在的 ID 在 Test 阶段即被挡住
// cache：防击穿 + 提升读性能，热点 key 命中后直接返回不打 DB
// sf：防击穿，缓存 miss 时同 key 并发只回源一次
type productService struct {
	repo  ProductRepository
	cache cache.CacheStore[*ProductDetail]
	bloom cache.BloomFilter
	sf    *cache.SingleFlight
}

// NewProductService 构造函数。cache/bloom/sf 均由外部注入，便于测试替换。
func NewProductService(
	repo ProductRepository,
	c cache.CacheStore[*ProductDetail],
	b cache.BloomFilter,
	sf *cache.SingleFlight,
) ProductService {
	return &productService{repo: repo, cache: c, bloom: b, sf: sf}
}

// CreateProduct 组装模型 → 事务创建 → 创建成功后直接走详情查询组装返回，
// 避免两套 DTO 组装逻辑。spu.ID 与各 SKU.ID 已由 GORM 在事务内回填。
func (s *productService) CreateProduct(ctx context.Context, req *CreateProductReq) (*ProductDetail, error) {
	images, err := json.Marshal(req.Images)
	if err != nil {
		return nil, apperror.New(apperror.CodeParamInvalid, "轮播图格式错误")
	}
	spu := &SPU{
		Name:        req.Name,
		CategoryID:  req.CategoryID,
		BrandID:     req.BrandID,
		MainImage:   req.MainImage,
		Images:      datatypes.JSON(images),
		Description: req.Description,
		Status:      SPUStatusOnline, // 演示阶段直接上架；生产应默认下架走审核流程
	}
	skus := make([]SKU, 0, len(req.SKUs))
	for i := range req.SKUs {
		specs, err := json.Marshal(req.SKUs[i].Specs)
		if err != nil {
			return nil, apperror.New(apperror.CodeParamInvalid, "规格格式错误")
		}
		skus = append(skus, SKU{
			Name:               req.SKUs[i].Name,
			Specs:              datatypes.JSON(specs),
			PriceCents:         req.SKUs[i].PriceCents,
			OriginalPriceCents: req.SKUs[i].OriginalPriceCents,
			Stock:              req.SKUs[i].Stock,
			Image:              req.SKUs[i].Image,
		})
	}
	if err := s.repo.CreateSPUWithSKUs(ctx, spu, skus); err != nil {
		return nil, err
	}
	// C 方案增量写入：创建成功后立即把 SPU ID 塞入布隆。
	// 不做这步 → 新商品被布隆误判为不存在 → GetProductDetail 第一层即返回 404。
	// bloom.Add 是内存位操作不会失败，error 忽略但保留接口语义便于未来扩展。
	if s.bloom != nil {
		_ = s.bloom.Add(ctx, spu.ID)
	}
	return s.GetProductDetail(ctx, spu.ID)
}

// GetProductDetail 三层防护：Bloom 预判 → Cache 查 → SingleFlight 回源。
// 1. Bloom Test 返回 false → 直接 404，不打 DB（防穿透）
// 2. Cache 命中 → 直接返回（防击穿 + 读性能）
// 3. Cache miss → SingleFlight 合并并发回源（防击穿）
// 回源成功后写回 Cache（TTL 叠加抖动防雪崩）
func (s *productService) GetProductDetail(ctx context.Context, spuID int64) (*ProductDetail, error) {
	// 第一层：布隆过滤器预判。若 bloom 为 nil（未启用防护）则跳过。
	if s.bloom != nil {
		ok, err := s.bloom.Test(ctx, spuID)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeUnknown, "布隆查询失败", err)
		}
		if !ok {
			return nil, apperror.ErrProductNotFound
		}
	}

	// 第二层：Redis 缓存查询。若 cache 为 nil（未启用防护）则跳过。
	if s.cache != nil {
		key := fmt.Sprintf("product:detail:%d", spuID)
		detail, err := s.cache.Get(ctx, key)
		if err != nil {
			// Redis 异常采用 fail-open 策略，不阻断主流程，继续走回源
			// （与限流中间件一致，业务层兜底）
			detail = nil
		}
		if detail != nil {
			return detail, nil
		}
	}

	// 第三层：SingleFlight 合并并发回源。若 sf 为 nil 则直接回源。
	if s.sf != nil && s.cache != nil {
		key := fmt.Sprintf("product:detail:%d", spuID)
		val, err, _ := s.sf.Do(ctx, key, func() (interface{}, error) {
			return s.loadDetail(ctx, spuID)
		})
		if err != nil {
			return nil, err
		}
		detail := val.(*ProductDetail)
		// 回源成功后写回缓存（JitterTTL 防雪崩）
		_ = s.cache.Set(ctx, key, detail, cache.JitterTTL(productDetailCacheTTL))
		return detail, nil
	}

	// 未启用防护的兜底路径：直接回源
	return s.loadDetail(ctx, spuID)
}

// loadDetail 真正回源 DB 查询 + 组装详情。被 SingleFlight 包装调用。
func (s *productService) loadDetail(ctx context.Context, spuID int64) (*ProductDetail, error) {
	spu, err := s.repo.FindSPUByID(ctx, spuID)
	if err != nil {
		return nil, err
	}
	skus, err := s.repo.ListSKUsBySPUID(ctx, spuID)
	if err != nil {
		return nil, err
	}
	return buildDetail(spu, skus), nil
}

// buildDetail 组装详情 DTO。specs/轮播图 JSON 反序列化失败不阻断主流程，降级为空值。
func buildDetail(spu *SPU, skus []SKU) *ProductDetail {
	images := make([]string, 0)
	_ = json.Unmarshal(spu.Images, &images)
	d := &ProductDetail{
		ID:          spu.ID,
		Name:        spu.Name,
		CategoryID:  spu.CategoryID,
		BrandID:     spu.BrandID,
		MainImage:   spu.MainImage,
		Images:      images,
		Description: spu.Description,
		Status:      uint8(spu.Status),
		SKUs:        make([]SKUDTO, 0, len(skus)),
	}
	for i := range skus {
		specs := make(map[string]string)
		_ = json.Unmarshal(skus[i].Specs, &specs)
		d.SKUs = append(d.SKUs, SKUDTO{
			ID:                 skus[i].ID,
			Name:               skus[i].Name,
			Specs:              specs,
			PriceCents:         skus[i].PriceCents,
			OriginalPriceCents: skus[i].OriginalPriceCents,
			Stock:              skus[i].Stock,
			Image:              skus[i].Image,
		})
	}
	return d
}

// ListProducts 列表页：先查 SPU 分页，再用一条 GROUP BY 批量聚合价格区间（消灭 N+1）。
func (s *productService) ListProducts(ctx context.Context, page, size int) ([]ProductListItem, int64, error) {
	spus, total, err := s.repo.ListSPUs(ctx, page, size)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ProductListItem, 0, len(spus))
	if len(spus) == 0 {
		return items, total, nil
	}
	ids := make([]int64, 0, len(spus))
	for i := range spus {
		ids = append(ids, spus[i].ID)
	}
	ranges, err := s.repo.ListPriceRangeBySPUIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range spus {
		item := ProductListItem{ID: spus[i].ID, Name: spus[i].Name, MainImage: spus[i].MainImage}
		if r, ok := ranges[spus[i].ID]; ok {
			item.MinPriceCents = r.MinPriceCents
			item.MaxPriceCents = r.MaxPriceCents
		}
		items = append(items, item)
	}
	return items, total, nil
}

// GetSKUByID 按 ID 查询 SKU。返回 model.SKU 给同域调用方使用；
// 跨域调用方（如 order）只能读取字段，不得修改或持久化该对象。
func (s *productService) GetSKUByID(ctx context.Context, skuID int64) (*SKU, error) {
	return s.repo.FindSKUByID(ctx, skuID)
}

// UpdateSPUStatus 上架/下架。service 层做合法值校验，不让脏状态进 repo。
func (s *productService) UpdateSPUStatus(ctx context.Context, spuID int64, status SPUStatus) error {
	if status != SPUStatusOffline && status != SPUStatusOnline {
		return apperror.New(apperror.CodeParamInvalid, "非法的商品状态")
	}
	return s.repo.UpdateSPUStatus(ctx, spuID, status)
}
