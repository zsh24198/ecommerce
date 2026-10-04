CREATE TABLE payments (
    id             BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT COMMENT '支付流水主键',
    payment_no     VARCHAR(32)      NOT NULL COMMENT '支付流水号（我方生成，传给第三方）',
    order_no       VARCHAR(32)      NOT NULL COMMENT '关联订单号',
    user_id        BIGINT UNSIGNED  NOT NULL COMMENT '支付用户ID',
    amount         BIGINT           NOT NULL COMMENT '支付金额，单位分',
    channel        TINYINT UNSIGNED NOT NULL COMMENT '1=微信支付 2=支付宝',
    status         TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1=待支付 2=支付成功 3=支付失败',
    transaction_id VARCHAR(64)      NOT NULL DEFAULT '' COMMENT '第三方交易号，全局唯一，用于回调幂等',
    paid_at        DATETIME(3)      NULL COMMENT '支付成功时间',
    callback_raw   TEXT             NULL COMMENT '回调原始数据（排查用）',
    created_at     DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at     DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    deleted_at     DATETIME(3)      NULL COMMENT '软删除时间',

    PRIMARY KEY (id),
    UNIQUE KEY uk_payments_payment_no (payment_no),
    UNIQUE KEY uk_payments_transaction_id (transaction_id),
    KEY idx_payments_order_no (order_no),
    KEY idx_payments_user_id (user_id),
    KEY idx_payments_status (status),
    KEY idx_payments_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='支付流水表';