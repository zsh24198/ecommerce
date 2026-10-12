-- 任务 24：库存回补所需字段
-- - status_changed_at：状态变更时间，超时判定依据（updated_at 会被任何 UPDATE 刷新，无法精确判定超时）
-- - order_type：订单类型，0=普通 1=秒杀，区分回补路径（MySQL vs Redis）
-- - seckill_item_id：秒杀商品 ID，仅秒杀单非空，回补时定位 Redis key（seckill:stock:{item_id}）

ALTER TABLE orders
    ADD COLUMN status_changed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '状态变更时间，超时判定依据',
    ADD COLUMN order_type TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0=普通单 1=秒杀单',
    ADD COLUMN seckill_item_id BIGINT UNSIGNED NULL COMMENT '秒杀商品 ID，仅秒杀单非空，用于回补定位 Redis key';

-- 旧订单回填：status_changed_at = created_at，等价于"创建后状态未变更"
UPDATE orders SET status_changed_at = created_at;

-- 超时扫描复合索引：(status, status_changed_at)
CREATE INDEX idx_orders_status_changed_at ON orders (status, status_changed_at);