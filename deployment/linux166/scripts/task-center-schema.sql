-- 任务中心独立表，不修改已有工作流、知识库或账号表。
CREATE TABLE IF NOT EXISTS taskcenter_tasks (
 id BIGINT NOT NULL PRIMARY KEY, space_id BIGINT NOT NULL, owner_id BIGINT NOT NULL,
 name VARCHAR(100) NOT NULL, kind VARCHAR(16) NOT NULL,
 workflow_id BIGINT NOT NULL, workflow_name VARCHAR(255) NOT NULL, version VARCHAR(128) NOT NULL,
 fields_json LONGTEXT NOT NULL, client_key VARCHAR(64) NOT NULL, request_hash CHAR(64) NOT NULL,
 concurrency INT NOT NULL, timeout_seconds INT NOT NULL, latest_run_id BIGINT NOT NULL DEFAULT 0,
 created_at BIGINT NOT NULL, updated_at BIGINT NOT NULL,
 UNIQUE KEY uk_taskcenter_task_request(owner_id,client_key),
 KEY idx_taskcenter_task_space(space_id,kind,updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS taskcenter_runs (
 id BIGINT NOT NULL PRIMARY KEY, task_id BIGINT NOT NULL, owner_id BIGINT NOT NULL,
 status VARCHAR(24) NOT NULL, total INT NOT NULL, succeeded INT NOT NULL DEFAULT 0,
 failed INT NOT NULL DEFAULT 0, cancelled INT NOT NULL DEFAULT 0, interrupted INT NOT NULL DEFAULT 0,
 stored_bytes BIGINT NOT NULL DEFAULT 0, retry_of BIGINT NOT NULL DEFAULT 0,
 client_key VARCHAR(64) NOT NULL, request_hash CHAR(64) NOT NULL, worker_key VARCHAR(64) NOT NULL,
 created_at BIGINT NOT NULL, started_at BIGINT NOT NULL DEFAULT 0, finished_at BIGINT NOT NULL DEFAULT 0,
 UNIQUE KEY uk_taskcenter_run_request(owner_id,client_key),
 KEY idx_taskcenter_run_queue(status,created_at), KEY idx_taskcenter_run_task(task_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS taskcenter_items (
 id BIGINT NOT NULL PRIMARY KEY, run_id BIGINT NOT NULL, position INT NOT NULL,
 status VARCHAR(24) NOT NULL, input_json LONGTEXT NOT NULL, output_json LONGTEXT NOT NULL,
 error TEXT NOT NULL, execution_id BIGINT NOT NULL DEFAULT 0, worker_key VARCHAR(64) NOT NULL,
 started_at BIGINT NOT NULL DEFAULT 0, finished_at BIGINT NOT NULL DEFAULT 0,
 UNIQUE KEY uk_taskcenter_item_position(run_id,position), KEY idx_taskcenter_item_queue(run_id,status,position)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
