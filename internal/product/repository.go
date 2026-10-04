package product

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

// ProductRepository 商品数据访问接口。
// 接口化与 user 模块同理：service 依赖接口，便于 mock 测试与 Phase 5 替换实现。
type ProductRepository interface {
	// CreateSPUWithSKUs 在一个事务内创建 SPU 及其全部 SKU，任一步失败整体回滚。
	// SPU 插入成功后自增 ID 回填到 spu.ID，并同步写入每条 SKU 的 SPUID。
	CreateSPUWithSKUs(ctx context.Context, spu *SPU, skus []SKU) error
	// FindSPUByID 按 ID 查询 SPU，找不到返回 apperror.ErrProductNotFound。
	FindSPUByID(ctx context.Context, id int64) (*SPU, error)
	// ListSPUs 分页查询上架商品，返回当前页列表与总数。
	ListSPUs(ctx context.Context, page, size int) ([]SPU, int64, error)
	// ListSKUsBySPUID 查询某 SPU 下全部启用状态的 SKU，按 id 升序保证展示顺序稳定。
	ListSKUsBySPUID(ctx context.Context, spuID int64) ([]SKU, error)
	// UpdateSPUStatus 更新 SPU 上下架状态，记录不存在返回 apperror.ErrProductNotFound。
	UpdateSPUStatus(ctx context.Context, id int64, status SPUStatus) error
	// FindSKUByID 按 ID 查询单个 SKU，找不到返回 apperror.ErrSkuNotFound。
	FindSKUByID(ctx context.Context, id int64) (*SKU, error)
	// ListPriceRangeBySPUIDs 批量聚合各 SPU 的最低/最高价（一条 GROUP BY 消灭 N+1 查询）。
	ListPriceRangeBySPUIDs(ctx context.Context, spuIDs []int64) (map[int64]PriceRange, error)
}

// PriceRange 某 SPU 下启用 SKU 的价格区间（单位：分）。
type PriceRange struct {
	MinPriceCents int64
	MaxPriceCents int64
}

// productRepo 是 ProductRepository 的 GORM 实现，包内可见，对外只暴露接口。
type productRepo struct {
	db *gorm.DB
}

// NewProductRepository 构造函数，由外部注入 *gorm.DB。
func NewProductRepository(db *gorm.DB) ProductRepository {
	return &productRepo{db: db}
}

// CreateSPUWithSKUs 事务写入：先落父表 SPU 拿自增 ID，再批量落子表 SKU。
// 闭包内必须全部使用 tx（事务会话）而非 db（连接池），否则写操作脱离事务无法回滚。
func (r *productRepo) CreateSPUWithSKUs(ctx context.Context, spu *SPU, skus []SKU) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(spu).Error; err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "创建商品失败", err)
		}
		for i := range skus {
			skus[i].SPUID = spu.ID
			skus[i].Status = SKUStatusEnabled
		}
		if err := tx.Create(&skus).Error; err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "创建商品规格失败", err)
		}
		return nil
	})
}

// FindSPUByID 按 ID 查询 SPU。
func (r *productRepo) FindSPUByID(ctx context.Context, id int64) (*SPU, error) {
	var spu SPU
	err := r.db.WithContext(ctx).First(&spu, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrProductNotFound
		}
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询商品失败", err)
	}
	return &spu, nil
}

// ListSPUs 分页查询上架商品。Session 开启新会话模式，使同一批条件可安全复用于 Count 与 Find。
func (r *productRepo) ListSPUs(ctx context.Context, page, size int) ([]SPU, int64, error) {
	var (
		spus  []SPU
		total int64
	)
	base := r.db.WithContext(ctx).Model(&SPU{}).
		Where("status = ?", SPUStatusOnline).
		Session(&gorm.Session{})
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, apperror.Wrap(apperror.CodeUnknown, "统计商品总数失败", err)
	}
	err := base.Order("sort DESC, id DESC").
		Offset((page - 1) * size).Limit(size).
		Find(&spus).Error
	if err != nil {
		return nil, 0, apperror.Wrap(apperror.CodeUnknown, "查询商品列表失败", err)
	}
	return spus, total, nil
}

// ListSKUsBySPUID 查询某 SPU 下启用的 SKU。
func (r *productRepo) ListSKUsBySPUID(ctx context.Context, spuID int64) ([]SKU, error) {
	var skus []SKU
	err := r.db.WithContext(ctx).
		Where("spu_id = ? AND status = ?", spuID, SKUStatusEnabled).
		Order("id ASC").
		Find(&skus).Error
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询商品规格失败", err)
	}
	return skus, nil
}

// UpdateSPUStatus 条件更新 SPU 状态。影响行数为 0 说明记录不存在（或已软删除），转为业务错误。
func (r *productRepo) UpdateSPUStatus(ctx context.Context, id int64, status SPUStatus) error {
	res := r.db.WithContext(ctx).Model(&SPU{}).
		Where("id = ?", id).
		Update("status", status)
	if res.Error != nil {
		return apperror.Wrap(apperror.CodeUnknown, "更新商品状态失败", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperror.ErrProductNotFound
	}
	return nil
}

// FindSKUByID 按 ID 查询单个 SKU。
func (r *productRepo) FindSKUByID(ctx context.Context, id int64) (*SKU, error) {
	var sku SKU
	err := r.db.WithContext(ctx).First(&sku, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrSkuNotFound
		}
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询SKU失败", err)
	}
	return &sku, nil
}

// ListPriceRangeBySPUIDs 批量聚合各 SPU 的最低/最高价。
// 匿名字段显式标注 column，避免 GORM 命名策略把 SPUID 推断成 spuid 导致映射失败。
func (r *productRepo) ListPriceRangeBySPUIDs(ctx context.Context, spuIDs []int64) (map[int64]PriceRange, error) {
	if len(spuIDs) == 0 {
		return map[int64]PriceRange{}, nil
	}
	var rows []struct {
		SPUID         int64 `gorm:"column:spu_id"`
		MinPriceCents int64 `gorm:"column:min_price_cents"`
		MaxPriceCents int64 `gorm:"column:max_price_cents"`
	}
	err := r.db.WithContext(ctx).Model(&SKU{}).
		Select("spu_id, MIN(price_cents) AS min_price_cents, MAX(price_cents) AS max_price_cents").
		Where("spu_id IN ? AND status = ?", spuIDs, SKUStatusEnabled).
		Group("spu_id").
		Scan(&rows).Error
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "统计商品价格区间失败", err)
	}
	out := make(map[int64]PriceRange, len(rows))
	for _, row := range rows {
		out[row.SPUID] = PriceRange{MinPriceCents: row.MinPriceCents, MaxPriceCents: row.MaxPriceCents}
	}
	return out, nil
}
