-- /root/ecommerce/migrations/000008_create_seckill_activities.up.sql
CREATE TABLE seckill_activities (
    id          BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT COMMENT '活动主键',
    name        VARCHAR(128)     NOT NULL COMMENT '活动名称',
    start_time  DATETIME(3)      NOT NULL COMMENT '开始时间',
    end_time    DATETIME(3)      NOT NULL COMMENT '结束时间',
    enabled     TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '运营开关 0=禁用 1=启用',
    created_at  DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at  DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    deleted_at  DATETIME(3)      NULL COMMENT '软删除时间',

    PRIMARY KEY (id),
    KEY idx_seckill_activities_start_time (start_time),
    KEY idx_seckill_activities_enabled (enabled),
    KEY idx_seckill_activities_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='秒杀活动表';

CREATE TABLE seckill_items (
    id                    BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '明细主键',
    activity_id           BIGINT UNSIGNED NOT NULL COMMENT '所属活动ID',
    sku_id                BIGINT UNSIGNED NOT NULL COMMENT '关联SKU ID',
    sku_name_snapshot     VARCHAR(128)    NOT NULL COMMENT 'SKU名称快照',
    original_price_cents  BIGINT          NOT NULL COMMENT '原价快照（划线价），单位分',
    seckill_price_cents   BIGINT          NOT NULL COMMENT '秒杀价，单位分',
    stock                 INT             NOT NULL DEFAULT 0 COMMENT '秒杀独立库存',
    limit_per_user        INT             NOT NULL DEFAULT 1 COMMENT '每人限购数量',
    created_at            DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at            DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    deleted_at            DATETIME(3)     NULL COMMENT '软删除时间',

    PRIMARY KEY (id),
    UNIQUE KEY uk_seckill_items_activity_id_sku_id (activity_id, sku_id),
    KEY idx_seckill_items_activity_id (activity_id),
    KEY idx_seckill_items_sku_id (sku_id),
    KEY idx_seckill_items_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='秒杀商品明细表';