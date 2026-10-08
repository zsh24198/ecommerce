CREATE TABLE outbox_events (
    id              BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT COMMENT '事件主键，同时是投递顺序',
    aggregate_type  VARCHAR(32)      NOT NULL COMMENT '聚合类型：order/payment',
    aggregate_id    VARCHAR(64)      NOT NULL COMMENT '聚合业务键：订单号/支付单号',
    event_type      VARCHAR(64)      NOT NULL COMMENT '事件类型：order.created 等',
    payload         JSON             NOT NULL COMMENT '事件消息体（业务字段快照）',
    published_at    DATETIME(3)      NULL COMMENT '投递成功时间，NULL=未投递',
    created_at      DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at      DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',

    PRIMARY KEY (id),
    KEY idx_outbox_relay (published_at, id),
    KEY idx_outbox_aggregate (aggregate_type, aggregate_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Outbox 事件发件箱表';