DROP INDEX idx_orders_status_changed_at ON orders;

ALTER TABLE orders
    DROP COLUMN seckill_item_id,
    DROP COLUMN order_type,
    DROP COLUMN status_changed_at;