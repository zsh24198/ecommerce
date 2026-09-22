CREATE TABLE users (
    id            BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT,
    phone         VARCHAR(20)      NOT NULL,
    password_hash VARCHAR(100)     NOT NULL,
    status        TINYINT UNSIGNED NOT NULL DEFAULT 1,
    created_at    DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at    DATETIME(3)      NULL,

    PRIMARY KEY (id),
    UNIQUE KEY uk_users_phone (phone),
    KEY idx_users_status (status),
    KEY idx_users_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;