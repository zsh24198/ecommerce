package payment

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

// PaymentRepository 支付数据访问接口。
type PaymentRepository interface {
	// Create 创建支付单（待支付状态）。
	Create(ctx context.Context, p *Payment) error

	// GetByPaymentNo 按支付流水号查询，未找到返回 nil。
	GetByPaymentNo(ctx context.Context, paymentNo string) (*Payment, error)

	// MarkSuccess 事务内原子完成三件事：
	//   1. 更新支付单为成功（写入 transaction_id、paid_at、callback_raw）
	//   2. 更新订单状态：待支付(1) → 已支付(2)，带状态机条件 WHERE status=1
	//   3. 写入 payment.paid outbox 事件（与业务同事务，仅完整成功路径走到这里）
	// 若订单已非待支付状态（RowsAffected=0），返回 apperror.ErrOrderNotPending。
	// 利用 transaction_id 唯一索引做回调幂等兜底：重复回调命中 1062 → 返回 nil（已处理）。
	MarkSuccess(ctx context.Context, paymentNo, transactionID string, paidAt time.Time, callbackRaw string) error
}

type paymentRepo struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) PaymentRepository {
	return &paymentRepo{db: db}
}

// Create 创建待支付状态的支付单。
func (r *paymentRepo) Create(ctx context.Context, p *Payment) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return apperror.Wrap(apperror.CodeUnknown, "创建支付单失败", err)
	}
	return nil
}

// GetByPaymentNo 按支付流水号查询。
func (r *paymentRepo) GetByPaymentNo(ctx context.Context, paymentNo string) (*Payment, error) {
	p := &Payment{}
	err := r.db.WithContext(ctx).
		Where("payment_no = ? AND deleted_at IS NULL", paymentNo).
		First(p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询支付单失败", err)
	}
	return p, nil
}

// MarkSuccess 核心事务：更新支付单成功 + 更新订单状态已支付。
//
// 设计要点：
//  1. 先更新支付单：用 UPDATE ... WHERE transaction_id=” 保证只有首次回调能写入，
//     同时写入 transaction_id 触发唯一索引，重复回调会命中 1062 → 幂等。
//  2. 再更新订单：WHERE status=1 带状态机条件，防止已支付/已取消订单被重复处理。
//  3. 两步在同一事务内，任一步失败整体回滚，保证「支付流水」与「订单状态」一致。
func (r *paymentRepo) MarkSuccess(ctx context.Context, paymentNo, transactionID string, paidAt time.Time, callbackRaw string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 更新支付单为成功：WHERE status=1 确保只更新待支付的单
		res := tx.Model(&Payment{}).
			Where("payment_no = ? AND status = ?", paymentNo, PaymentStatusPending).
			Updates(map[string]any{
				"status":         PaymentStatusSuccess,
				"transaction_id": transactionID,
				"paid_at":        paidAt,
				"callback_raw":   callbackRaw,
			})
		if res.Error != nil {
			if isDuplicateKeyErr(res.Error) {
				// transaction_id 已存在 → 重复回调 → 幂等返回成功
				return nil
			}
			return apperror.Wrap(apperror.CodeUnknown, "更新支付单失败", res.Error)
		}
		if res.RowsAffected == 0 {
			// 支付单已不是待支付（可能已成功/失败），查一下确认幂等
			existing, err := r.getByPaymentNoTx(tx, paymentNo)
			if err != nil {
				return err
			}
			if existing != nil && existing.Status == PaymentStatusSuccess {
				return nil // 已处理成功，幂等
			}
			return apperror.ErrOrderNotPending
		}

		// 2. 更新订单状态：待支付 → 已支付，带状态机条件
		orderRes := tx.Table("orders").
			Where("order_no = (SELECT order_no FROM payments WHERE payment_no = ?) AND status = ?",
				paymentNo, 1).
			Update("status", 2)
		if orderRes.Error != nil {
			return apperror.Wrap(apperror.CodeUnknown, "更新订单状态失败", orderRes.Error)
		}
		if orderRes.RowsAffected == 0 {
			return apperror.ErrOrderNotPending
		}

		// 3. 同事务写 outbox 事件：订单状态也更新成功后才走到这里（幂等路径已提前返回）。
		// 查回支付单构建 payload（同事务内读，数据一致），消费方拿到事件即可用，无需反查业务库。
		paid, err := r.getByPaymentNoTx(tx, paymentNo)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(paymentPaidEventPayload{
			PaymentNo:     paid.PaymentNo,
			OrderNo:       paid.OrderNo,
			UserID:        paid.UserID,
			Amount:        paid.Amount,
			Channel:       paid.Channel,
			TransactionID: paid.TransactionID,
			PaidAt:        paidAt.Format(time.RFC3339Nano),
		})
		if err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "序列化支付事件失败", err)
		}
		if err := tx.Exec(
			"INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload) VALUES (?, ?, ?, ?)",
			"payment", paymentNo, "payment.paid", string(payload),
		).Error; err != nil {
			return apperror.Wrap(apperror.CodeUnknown, "写入 outbox 事件失败", err)
		}

		return nil
	})
}

// paymentPaidEventPayload payment.paid 事件消息体：支付完成的事实快照。
type paymentPaidEventPayload struct {
	PaymentNo     string         `json:"payment_no"`
	OrderNo       string         `json:"order_no"`
	UserID        int64          `json:"user_id"`
	Amount        int64          `json:"amount"`
	Channel       PaymentChannel `json:"channel"`
	TransactionID string         `json:"transaction_id"`
	PaidAt        string         `json:"paid_at"`
}

// getByPaymentNoTx 事务内查询支付单。
func (r *paymentRepo) getByPaymentNoTx(tx *gorm.DB, paymentNo string) (*Payment, error) {
	p := &Payment{}
	err := tx.Where("payment_no = ? AND deleted_at IS NULL", paymentNo).First(p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, apperror.Wrap(apperror.CodeUnknown, "查询支付单失败", err)
	}
	return p, nil
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
