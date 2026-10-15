package agent

// QueryOrderReq Agent 按订单号查询请求。
// user_id 由 Agent 显式传入（Agent Token 不携带用户身份），后端校验订单归属。
type QueryOrderReq struct {
	UserID  int64  `json:"user_id"  binding:"required,gt=0"`
	OrderNo string `json:"order_no" binding:"required,max=64"`
}

// AgentOrderDTO Agent 专用订单响应 DTO。
// 与 order.OrderInfo 分离：内部 DTO 可随意改，外部契约必须稳定。
type AgentOrderDTO struct {
	OrderNo     string `json:"order_no"`
	UserID      int64  `json:"user_id"`
	TotalAmount int64  `json:"total_amount_cents"` // 单位：分
	Status      int    `json:"status"`
}

// QueryProductReq Agent 按 SPU ID 查询商品请求。
type QueryProductReq struct {
	SpuID int64 `json:"spu_id" binding:"required,gt=0"`
}

// AgentProductDTO Agent 专用商品响应 DTO。
type AgentProductDTO struct {
	ID     int64         `json:"id"`
	Name   string        `json:"name"`
	Status uint8         `json:"status"`
	SKUs   []AgentSKUDTO `json:"skus"`
}

// AgentSKUDTO Agent 专用 SKU 响应 DTO。
type AgentSKUDTO struct {
	ID         int64 `json:"id"`
	PriceCents int64 `json:"price_cents"`
	Stock      int   `json:"stock"`
}

// QueryUserReq Agent 按用户 ID 查询请求。
type QueryUserReq struct {
	UserID int64 `json:"user_id" binding:"required,gt=0"`
}

// AgentUserDTO Agent 专用用户响应 DTO，只暴露非敏感字段。
type AgentUserDTO struct {
	ID     int64  `json:"id"`
	Phone  string `json:"phone"`
	Status int    `json:"status"`
}
