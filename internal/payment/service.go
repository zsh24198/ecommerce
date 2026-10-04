package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zsh24198/ecommerce/internal/order"
	"github.com/zsh24198/ecommerce/shared/apperror"
)

// CreatePaymentReq 创建支付单请求 DTO。
type CreatePaymentReq struct {
	OrderNo string `json:"order_no" binding:"required"`
	Channel int    `json:"channel"  binding:"required,oneof=1 2"` // 1=微信 2=支付宝
}

// CallbackReq 支付回调请求 DTO。
// 注意：amount 用 string 接收，避免 JSON 数字精度问题；status 用 string 接收回调状态。
type CallbackReq struct {
	PaymentNo     string `json:"payment_no"     binding:"required"`
	OrderNo       string `json:"order_no"       binding:"required"`
	Amount        string `json:"amount"         binding:"required"`
	Channel       int    `json:"channel"        binding:"required,oneof=1 2"`
	TransactionID string `json:"transaction_id" binding:"required"`
	Status        string `json:"status"         binding:"required"` // success / failed
	PaidAt        string `json:"paid_at"        binding:"required"`
	Sign          string `json:"sign"           binding:"required"`
}

// PaymentService 支付模块业务接口。
type PaymentService interface {
	// CreatePayment 为指定订单创建支付单，返回支付流水号。
	// 校验：订单存在、归属当前用户、状态为待支付。
	CreatePayment(ctx context.Context, userID int64, req *CreatePaymentReq) (string, error)

	// HandleCallback 处理支付平台异步回调。
	// 流程：验签 → 查支付单 → 校验金额 → 校验订单状态 → 事务内更新支付单+订单。
	HandleCallback(ctx context.Context, req *CallbackReq) error
}

type paymentService struct {
	repo   PaymentRepository
	order  order.OrderService // 跨域调用：通过 order.Service 接口
	signer *Signer
}

// NewPaymentService 构造函数。
func NewPaymentService(repo PaymentRepository, orderSvc order.OrderService, signer *Signer) PaymentService {
	return &paymentService{repo: repo, order: orderSvc, signer: signer}
}

// CreatePayment 创建支付单。
func (s *paymentService) CreatePayment(ctx context.Context, userID int64, req *CreatePaymentReq) (string, error) {
	// 1. 跨域读：通过 order.Service 接口获取订单信息
	orderInfo, err := s.order.GetOrderInfoByNo(ctx, req.OrderNo)
	if err != nil {
		return "", err
	}
	if orderInfo == nil {
		return "", apperror.ErrOrderIllegal
	}

	// 2. 校验订单归属（防止用户 A 给用户 B 的订单付款）
	if orderInfo.UserID != userID {
		return "", apperror.ErrForbidden
	}

	// 3. 校验订单状态：只有待支付才能创建支付单
	if orderInfo.Status != order.OrderStatusPending {
		return "", apperror.ErrOrderNotPending
	}

	// 4. 创建支付单
	p := &Payment{
		PaymentNo: genPaymentNo(),
		OrderNo:   req.OrderNo,
		UserID:    userID,
		Amount:    orderInfo.TotalAmount,
		Channel:   PaymentChannel(req.Channel),
		Status:    PaymentStatusPending,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return "", err
	}
	return p.PaymentNo, nil
}

// HandleCallback 处理支付异步回调。
//
// 完整链路：
//  1. 验签：确认回调来自支付平台，防伪造
//  2. 查支付单：确认支付单存在
//  3. 金额校验：回调金额 == 订单金额，防篡改
//  4. 事务内更新：支付单成功 + 订单已支付（带状态机条件 + transaction_id 唯一索引幂等）
func (s *paymentService) HandleCallback(ctx context.Context, req *CallbackReq) error {
	// 1. 验签
	params := map[string]string{
		"payment_no":     req.PaymentNo,
		"order_no":       req.OrderNo,
		"amount":         req.Amount,
		"channel":        fmt.Sprintf("%d", req.Channel),
		"transaction_id": req.TransactionID,
		"status":         req.Status,
		"paid_at":        req.PaidAt,
		"sign":           req.Sign,
	}
	if !s.signer.VerifySign(params) {
		return apperror.ErrPaymentSign
	}

	// 2. 查支付单
	p, err := s.repo.GetByPaymentNo(ctx, req.PaymentNo)
	if err != nil {
		return err
	}
	if p == nil {
		return apperror.ErrPaymentNotFound
	}

	// 3. 金额校验（防篡改：回调金额必须等于订单应付金额）
	var callbackAmount int64
	if _, err := fmt.Sscanf(req.Amount, "%d", &callbackAmount); err != nil {
		return apperror.New(apperror.CodeParamInvalid, "amount 格式非法")
	}
	if callbackAmount != p.Amount {
		return apperror.ErrPaymentAmount
	}

	// 4. 只处理成功回调；失败回调仅记录日志，不更新订单（演示简化）
	if req.Status != "success" {
		return nil
	}

	// 5. 解析支付时间
	paidAt, err := time.ParseInLocation("2006-01-02 15:04:05", req.PaidAt, time.Local)
	if err != nil {
		return apperror.New(apperror.CodeParamInvalid, "paid_at 格式非法")
	}

	// 6. 序列化完整回调参数作为 callbackRaw，便于事后排查
	callbackRaw, err := json.Marshal(params)
	if err != nil {
		return apperror.Wrap(apperror.CodeUnknown, "序列化回调参数失败", err)
	}

	// 7. 事务内更新支付单 + 订单状态（幂等由 transaction_id 唯一索引兜底）
	return s.repo.MarkSuccess(ctx, req.PaymentNo, req.TransactionID, paidAt, string(callbackRaw))
}

// genPaymentNo 生成支付流水号：P + 年月日时分秒 + 6位随机数。
func genPaymentNo() string {
	return fmt.Sprintf("P%s%06d", time.Now().Format("20060102150405"), time.Now().UnixNano()%1000000)
}
