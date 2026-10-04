CREATE TABLE orders (
    id             BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT COMMENT '订单主键',
    order_no       VARCHAR(32)      NOT NULL COMMENT '业务订单号，对外暴露',
    user_id        BIGINT UNSIGNED  NOT NULL COMMENT '下单用户ID',
    total_amount   BIGINT           NOT NULL COMMENT '订单总金额，单位分',
    status         TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1=待支付 2=已支付 3=已发货 4=已完成 5=已取消',
    receiver_name  VARCHAR(64)      NOT NULL DEFAULT '' COMMENT '收货人姓名',
    receiver_phone VARCHAR(20)      NOT NULL DEFAULT '' COMMENT '收货人电话',
    receiver_addr  VARCHAR(512)     NOT NULL DEFAULT '' COMMENT '收货地址',
    created_at     DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at     DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    deleted_at     DATETIME(3)      NULL COMMENT '软删除时间',

    PRIMARY KEY (id),
    UNIQUE KEY uk_orders_order_no (order_no),
    KEY idx_orders_user_id (user_id),
    KEY idx_orders_status (status),
    KEY idx_orders_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单主表';

CREATE TABLE order_items (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '订单明细主键',
    order_id     BIGINT UNSIGNED NOT NULL COMMENT '所属订单ID',
    sku_id       BIGINT UNSIGNED NOT NULL COMMENT 'SKU ID',
    sku_name     VARCHAR(128)    NOT NULL COMMENT '商品名快照',
    price_cents  BIGINT          NOT NULL COMMENT '单价快照，单位分',
    quantity     INT             NOT NULL DEFAULT 1 COMMENT '购买数量',
    subtotal     BIGINT          NOT NULL COMMENT '小计金额，单位分',
    created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    deleted_at   DATETIME(3)     NULL COMMENT '软删除时间',

    PRIMARY KEY (id),
    KEY idx_order_items_order_id (order_id),
    KEY idx_order_items_sku_id (sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单明细表';