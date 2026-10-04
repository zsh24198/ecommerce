-- 给 orders 表加幂等键字段，用于防重复下单的数据库兜底
ALTER TABLE orders
    ADD COLUMN idempotency_key VARCHAR(64) NOT NULL DEFAULT '' COMMENT '幂等键（前端生成的 UUID）' AFTER receiver_addr;

-- 唯一索引：同一用户的同一幂等键只能有一个未删除订单
-- 包含 deleted_at 是为了软删除后该 key 可重新使用
ALTER TABLE orders
    ADD UNIQUE KEY uk_orders_user_idempotency (user_id, idempotency_key, deleted_at);