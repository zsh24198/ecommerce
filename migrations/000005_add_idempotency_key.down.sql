ALTER TABLE orders DROP INDEX uk_orders_user_idempotency;
ALTER TABLE orders DROP COLUMN idempotency_key;