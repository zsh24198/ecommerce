package outbox

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/zsh24198/ecommerce/shared/apperror"
)

// OutboxRepository Outbox 数据访问接口。
type OutboxRepository interface {
	// PollPending 捞取未投递事件，按 id 升序保证投递顺序。
	PollPending(ctx context.Context, limit int) ([]OutboxEvent, error)
	// MarkPublished 标记事件投递成功。
	MarkPublished(ctx context.Context, id uint64) error
}

type outboxRepo struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) OutboxRepository {
	return &outboxRepo{db: db}
}

// PollPending 只捞未投递的事件，走 idx_outbox_relay (published_at, id) 索引。
func (r *outboxRepo) PollPending(ctx context.Context, limit int) ([]OutboxEvent, error) {
	var events []OutboxEvent
	err := r.db.WithContext(ctx).
		Where("published_at IS NULL").
		Order("id ASC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "轮询 outbox 事件失败", err)
	}
	return events, nil
}

// MarkPublished 带上 published_at IS NULL 条件：重复标记是无害的 no-op。
func (r *outboxRepo) MarkPublished(ctx context.Context, id uint64) error {
	res := r.db.WithContext(ctx).Model(&OutboxEvent{}).
		Where("id = ? AND published_at IS NULL", id).
		Update("published_at", time.Now())
	if res.Error != nil {
		return apperror.Wrap(apperror.CodeUnknown, "标记 outbox 事件已投递失败", res.Error)
	}
	return nil
}
