package product

import (
	"context"
	"encoding/json"

	"gorm.io/datatypes"

	"github.com/zsh24198/ecommerce/shared/apperror"
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
}

type productService struct {
	repo ProductRepository
}

// NewProductService 构造函数。
func NewProductService(repo ProductRepository) ProductService {
	return &productService{repo: repo}
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
	return s.GetProductDetail(ctx, spu.ID)
}

// GetProductDetail 详情 = SPU 共性 + 全部启用 SKU。
func (s *productService) GetProductDetail(ctx context.Context, spuID int64) (*ProductDetail, error) {
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

// UpdateSPUStatus 上架/下架。service 层做合法值校验，不让脏状态进 repo。
func (s *productService) UpdateSPUStatus(ctx context.Context, spuID int64, status SPUStatus) error {
	if status != SPUStatusOffline && status != SPUStatusOnline {
		return apperror.New(apperror.CodeParamInvalid, "非法的商品状态")
	}
	return s.repo.UpdateSPUStatus(ctx, spuID, status)
}
