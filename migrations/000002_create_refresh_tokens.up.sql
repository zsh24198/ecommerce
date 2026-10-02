CREATE TABLE refresh_tokens (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id    BIGINT UNSIGNED NOT NULL COMMENT '所属用户ID',
    token_hash CHAR(64)        NOT NULL COMMENT 'refresh token 的 SHA-256 十六进制哈希',
    expires_at DATETIME(3)     NOT NULL COMMENT '过期截止时间',
    revoked_at DATETIME(3)     NULL COMMENT '作废时间，NULL=有效',
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',

    PRIMARY KEY (id),
    UNIQUE KEY uk_refresh_tokens_hash (token_hash),
    KEY idx_refresh_tokens_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;