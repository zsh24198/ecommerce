
CREATE TABLE spus (
    id           BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT COMMENT 'SPU 主键',
    name         VARCHAR(128)     NOT NULL COMMENT '商品名称',
    category_id  BIGINT UNSIGNED  NOT NULL DEFAULT 0 COMMENT '分类ID，0=未分类',
    brand_id     BIGINT UNSIGNED  NOT NULL DEFAULT 0 COMMENT '品牌ID，0=无品牌',
    main_image   VARCHAR(512)     NOT NULL DEFAULT '' COMMENT '主图URL',
    images       JSON             NULL COMMENT '轮播图URL数组',
    description  TEXT             NULL COMMENT '商品详情',
    status       TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0=下架 1=上架',
    sort         INT              NOT NULL DEFAULT 0 COMMENT '排序权重，越大越靠前',
    created_at   DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at   DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    deleted_at   DATETIME(3)      NULL COMMENT '软删除时间',

    PRIMARY KEY (id),
    KEY idx_spus_name (name),
    KEY idx_spus_category_id (category_id),
    KEY idx_spus_status (status),
    KEY idx_spus_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='SPU 商品共性表';

CREATE TABLE skus (
    id                   BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT COMMENT 'SKU 主键',
    spu_id               BIGINT UNSIGNED  NOT NULL COMMENT '所属SPU ID',
    name                 VARCHAR(128)     NOT NULL COMMENT 'SKU 名称',
    specs                JSON             NULL COMMENT '规格键值对，如 {"颜色":"黑色","容量":"128G"}',
    price_cents          BIGINT           NOT NULL COMMENT '售价，单位分',
    original_price_cents BIGINT           NOT NULL DEFAULT 0 COMMENT '原价（划线价），单位分',
    stock                INT              NOT NULL DEFAULT 0 COMMENT '库存数量',
    image                VARCHAR(512)     NOT NULL DEFAULT '' COMMENT 'SKU 主图URL',
    status               TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '0=禁用 1=启用',
    created_at           DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at           DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    deleted_at           DATETIME(3)      NULL COMMENT '软删除时间',

    PRIMARY KEY (id),
    KEY idx_skus_spu_id (spu_id),
    KEY idx_skus_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='SKU 规格表';
