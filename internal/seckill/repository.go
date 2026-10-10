// /root/ecommerce/internal/seckill/repository.go
package seckill

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

// SeckillRepository 秒杀模块数据访问接口。
type SeckillRepository interface {
	// CreateActivityWithItems 活动 + 明细同一事务落库。
	CreateActivityWithItems(ctx context.Context, act *SeckillActivity, items []SeckillItem) error
	// FindActivityByID 按 ID 查活动，不存在返回 ErrSeckillActivityNotFound。
	FindActivityByID(ctx context.Context, id int64) (*SeckillActivity, error)
	// FindItemsByActivityID 查活动下全部明细。
	FindItemsByActivityID(ctx context.Context, activityID int64) ([]SeckillItem, error)
	// ListActivities 活动分页（按开始时间倒序）。
	ListActivities(ctx context.Context, page, size int) ([]SeckillActivity, int64, error)
	// ListOngoingActivities 查当前进行中且启用的活动。
	ListOngoingActivities(ctx context.Context, now time.Time) ([]SeckillActivity, error)
	// FindOverlappingSKUs 返回这批 SKU 在 [start, end] 区间内
	// 已被其他启用中活动占用的 SKU ID 集合（空集合 = 无重叠）。
	FindOverlappingSKUs(ctx context.Context, skuIDs []int64, start, end time.Time) ([]int64, error)
	// UpdateActivityEnabled 更新运营开关。
	UpdateActivityEnabled(ctx context.Context, id int64, enabled bool) error
}

type seckillRepo struct {
	db *gorm.DB
}

// NewSeckillRepository 构造函数，由外部注入 *gorm.DB。
func NewSeckillRepository(db *gorm.DB) SeckillRepository {
	return &seckillRepo{db: db}
}

// CreateActivityWithItems 事务写入：先落活动表拿自增 ID，再批量落明细表。
// 并发下唯一索引 uk(activity_id, sku_id) 是最后防线：
// service 层的重叠校验与落库之间存在竞态窗口，两个并发请求可能都通过校验，
// 此处捕获 MySQL 1062 转业务错误，保证不会出现同一 SKU 的重复明细。
func (r *seckillRepo) CreateActivityWithItems(ctx context.Context, act *SeckillActivity, items []SeckillItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(act).Error; err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "创建秒杀活动失败", err)
		}
		for i := range items {
			items[i].ActivityID = act.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			var mysqlErr *mysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				return apperror.ErrSeckillSKUBusy
			}
			return apperror.Wrap(apperror.CodeUnknown, "创建秒杀商品失败", err)
		}
		return nil
	})
}

// FindActivityByID 按 ID 查活动。
func (r *seckillRepo) FindActivityByID(ctx context.Context, id int64) (*SeckillActivity, error) {
	var act SeckillActivity
	err := r.db.WithContext(ctx).First(&act, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.ErrSeckillActivityNotFound
		}
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询秒杀活动失败", err)
	}
	return &act, nil
}

// FindItemsByActivityID 查活动下全部明细。
func (r *seckillRepo) FindItemsByActivityID(ctx context.Context, activityID int64) ([]SeckillItem, error) {
	var items []SeckillItem
	err := r.db.WithContext(ctx).
		Where("activity_id = ?", activityID).
		Order("id ASC").
		Find(&items).Error
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询秒杀商品失败", err)
	}
	return items, nil
}

// ListActivities 运营视角分页。Session 开新会话使条件可安全复用于 Count 与 Find。
func (r *seckillRepo) ListActivities(ctx context.Context, page, size int) ([]SeckillActivity, int64, error) {
	var (
		acts  []SeckillActivity
		total int64
	)
	base := r.db.WithContext(ctx).Model(&SeckillActivity{}).Session(&gorm.Session{})
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, apperror.Wrap(apperror.CodeUnknown, "统计秒杀活动总数失败", err)
	}
	err := base.Order("start_time DESC, id DESC").
		Offset((page - 1) * size).Limit(size).
		Find(&acts).Error
	if err != nil {
		return nil, 0, apperror.Wrap(apperror.CodeUnknown, "查询秒杀活动列表失败", err)
	}
	return acts, total, nil
}

// ListOngoingActivities 秒杀首页视角：启用中且当前时间在窗口内的活动。
func (r *seckillRepo) ListOngoingActivities(ctx context.Context, now time.Time) ([]SeckillActivity, error) {
	var acts []SeckillActivity
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND start_time <= ? AND end_time > ?", true, now, now).
		Order("start_time ASC").
		Find(&acts).Error
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询进行中活动失败", err)
	}
	return acts, nil
}

// FindOverlappingSKUs 查这批 SKU 在 [start, end] 内已被其他启用中活动占用的集合。
// 重叠条件由德摩根律推导：NOT (a.end_time < start OR a.start_time > end)
//
//	= a.end_time >= start AND a.start_time <= end
//
// JOIN 的别名表不受 GORM 软删除自动作用，a.deleted_at 必须手动过滤。
func (r *seckillRepo) FindOverlappingSKUs(ctx context.Context, skuIDs []int64, start, end time.Time) ([]int64, error) {
	if len(skuIDs) == 0 {
		return []int64{}, nil
	}
	var ids []int64
	err := r.db.WithContext(ctx).
		Model(&SeckillItem{}).
		Joins("JOIN seckill_activities a ON a.id = seckill_items.activity_id AND a.enabled = 1 AND a.deleted_at IS NULL").
		Where("seckill_items.sku_id IN ?", skuIDs).
		Where("a.end_time >= ? AND a.start_time <= ?", start, end).
		Distinct().
		Pluck("seckill_items.sku_id", &ids).Error
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询SKU活动占用失败", err)
	}
	return ids, nil
}

// UpdateActivityEnabled 条件更新运营开关。影响行数为 0 说明活动不存在。
func (r *seckillRepo) UpdateActivityEnabled(ctx context.Context, id int64, enabled bool) error {
	res := r.db.WithContext(ctx).Model(&SeckillActivity{}).
		Where("id = ?", id).
		Update("enabled", enabled)
	if res.Error != nil {
		return apperror.Wrap(apperror.CodeUnknown, "更新活动开关失败", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperror.ErrSeckillActivityNotFound
	}
	return nil
}
