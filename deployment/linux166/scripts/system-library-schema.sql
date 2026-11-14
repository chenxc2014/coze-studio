-- 系统资源发布登记与版本记录，不移动原资源，不复制用户数据。
CREATE TABLE IF NOT EXISTS system_resource_publications (
 kind VARCHAR(24) NOT NULL, resource_id BIGINT NOT NULL,
 source_space_id BIGINT NOT NULL, source_owner_id BIGINT NOT NULL,
 name VARCHAR(255) NOT NULL, description TEXT NOT NULL, icon TEXT NOT NULL,
 version VARCHAR(128) NOT NULL, origin VARCHAR(24) NOT NULL, status VARCHAR(24) NOT NULL,
 revision BIGINT NOT NULL, metadata_json LONGTEXT NOT NULL,
 published_by BIGINT NOT NULL, updated_by BIGINT NOT NULL,
 created_at BIGINT NOT NULL, updated_at BIGINT NOT NULL,
 PRIMARY KEY(kind,resource_id), KEY idx_system_resource_status(status,kind,updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS system_resource_releases (
 kind VARCHAR(24) NOT NULL, resource_id BIGINT NOT NULL, revision BIGINT NOT NULL,
 status VARCHAR(24) NOT NULL, version VARCHAR(128) NOT NULL,
 metadata_json LONGTEXT NOT NULL, operator_id BIGINT NOT NULL, created_at BIGINT NOT NULL,
 PRIMARY KEY(kind,resource_id,revision)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
