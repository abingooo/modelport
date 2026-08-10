-- Replace the temporary V2 candidate workflow with explicit global trust,
-- global risk, and durable multi-node review state.

ALTER TABLE instruction_audit_v2_config
    ADD COLUMN IF NOT EXISTS allow_empty_fields BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS async_retry_schedule_seconds INT[] NOT NULL
        DEFAULT ARRAY[30, 120, 600, 3600, 21600];

ALTER TABLE instruction_audit_v2_config
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_retry_schedule;
ALTER TABLE instruction_audit_v2_config
    ADD CONSTRAINT chk_instruction_audit_v2_retry_schedule CHECK (
        cardinality(async_retry_schedule_seconds) BETWEEN 1 AND 12
        AND 0 < ALL(async_retry_schedule_seconds)
        AND 604800 >= ALL(async_retry_schedule_seconds)
    );

UPDATE instruction_audit_v2_config
SET ai_cache_ttl_seconds = 0,
    updated_at = NOW()
WHERE ai_cache_ttl_seconds <> 0;

ALTER TABLE instruction_audit_v2_ai_nodes
    ADD COLUMN IF NOT EXISTS slot VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS response_mode VARCHAR(16) NOT NULL DEFAULT 'auto',
    ADD COLUMN IF NOT EXISTS max_output_tokens INT NOT NULL DEFAULT 1024;

WITH ranked AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY enabled DESC, priority, id) AS position
    FROM instruction_audit_v2_ai_nodes
)
UPDATE instruction_audit_v2_ai_nodes node
SET slot = CASE ranked.position
        WHEN 1 THEN 'sync'
        WHEN 2 THEN 'async_1'
        WHEN 3 THEN 'async_2'
        WHEN 4 THEN 'async_3'
        ELSE ''
    END,
    enabled = CASE WHEN ranked.position <= 4 THEN node.enabled ELSE FALSE END,
    updated_at = NOW()
FROM ranked
WHERE node.id = ranked.id
  AND node.slot = '';

ALTER TABLE instruction_audit_v2_ai_nodes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_node_slot;
ALTER TABLE instruction_audit_v2_ai_nodes
    ADD CONSTRAINT chk_instruction_audit_v2_node_slot CHECK (
        slot IN ('', 'sync', 'async_1', 'async_2', 'async_3')
    );
ALTER TABLE instruction_audit_v2_ai_nodes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_node_response_mode;
ALTER TABLE instruction_audit_v2_ai_nodes
    ADD CONSTRAINT chk_instruction_audit_v2_node_response_mode CHECK (
        response_mode IN ('auto', 'json_schema', 'json_object')
    );
ALTER TABLE instruction_audit_v2_ai_nodes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_node_output_tokens;
ALTER TABLE instruction_audit_v2_ai_nodes
    ADD CONSTRAINT chk_instruction_audit_v2_node_output_tokens CHECK (
        max_output_tokens BETWEEN 128 AND 8192
    );

CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_v2_node_slot
    ON instruction_audit_v2_ai_nodes(slot)
    WHERE slot <> '';

-- V2 candidates were never authoritative policy. Remove them before narrowing
-- the status constraints; active, disabled and revoked administrator data stays.
DELETE FROM instruction_audit_v2_hash_scopes scope_link
WHERE scope_link.status = 'candidate'
   OR EXISTS (
       SELECT 1 FROM instruction_audit_v2_hashes hash
       WHERE hash.id = scope_link.hash_id AND hash.status = 'candidate'
   );
DELETE FROM instruction_audit_v2_hashes WHERE status = 'candidate';

ALTER TABLE instruction_audit_v2_hashes
    ADD COLUMN IF NOT EXISTS global_trust BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE instruction_audit_v2_hashes ALTER COLUMN status SET DEFAULT 'active';
ALTER TABLE instruction_audit_v2_hash_scopes ALTER COLUMN status SET DEFAULT 'active';

ALTER TABLE instruction_audit_v2_hashes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_hash_status;
ALTER TABLE instruction_audit_v2_hashes
    ADD CONSTRAINT chk_instruction_audit_v2_hash_status CHECK (
        status IN ('active', 'disabled', 'revoked')
    );
ALTER TABLE instruction_audit_v2_hashes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_hash_candidate_expiry;

ALTER TABLE instruction_audit_v2_hash_scopes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_hash_scope_status;
ALTER TABLE instruction_audit_v2_hash_scopes
    ADD CONSTRAINT chk_instruction_audit_v2_hash_scope_status CHECK (
        status IN ('active', 'disabled', 'revoked')
    );
ALTER TABLE instruction_audit_v2_hash_scopes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_hash_scope_expiry;

CREATE TABLE IF NOT EXISTS instruction_audit_v2_content_vault (
    id                    BIGSERIAL PRIMARY KEY,
    sha256                CHAR(64) NOT NULL UNIQUE,
    raw_ciphertext        BYTEA NOT NULL,
    content_bytes         BIGINT NOT NULL,
    stored_bytes          INT NOT NULL,
    observed_field        VARCHAR(16) NOT NULL DEFAULT '',
    encryption_key_version VARCHAR(64) NOT NULL DEFAULT 'instruction-evidence-v1',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_vault_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_vault_field CHECK (
        observed_field IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_vault_lengths CHECK (
        content_bytes > 0 AND stored_bytes > 0 AND octet_length(raw_ciphertext) > 0
    )
);

INSERT INTO instruction_audit_v2_content_vault (
    sha256, raw_ciphertext, content_bytes, stored_bytes, observed_field
)
SELECT sha256, raw_ciphertext, content_bytes, stored_bytes, observed_field
FROM instruction_audit_v2_hashes
WHERE raw_storage = 'full'
  AND raw_ciphertext IS NOT NULL
  AND content_bytes > 0
  AND stored_bytes > 0
ON CONFLICT (sha256) DO NOTHING;

ALTER TABLE instruction_audit_v2_hashes
    ADD COLUMN IF NOT EXISTS content_vault_id BIGINT
        REFERENCES instruction_audit_v2_content_vault(id) ON DELETE SET NULL;

UPDATE instruction_audit_v2_hashes hash
SET content_vault_id = vault.id
FROM instruction_audit_v2_content_vault vault
WHERE hash.content_vault_id IS NULL
  AND vault.sha256 = hash.sha256;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_hashes_global_runtime
    ON instruction_audit_v2_hashes(global_trust, status, sha256);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_risk_hashes (
    id                    BIGSERIAL PRIMARY KEY,
    sha256                CHAR(64) NOT NULL UNIQUE,
    content_vault_id      BIGINT NOT NULL
        REFERENCES instruction_audit_v2_content_vault(id) ON DELETE RESTRICT,
    observed_field        VARCHAR(16) NOT NULL DEFAULT '',
    status                VARCHAR(16) NOT NULL DEFAULT 'active',
    source                VARCHAR(24) NOT NULL,
    source_event_id       BIGINT REFERENCES instruction_audit_v2_events(id) ON DELETE SET NULL,
    reviewer_node_id      BIGINT REFERENCES instruction_audit_v2_ai_nodes(id) ON DELETE SET NULL,
    reviewer_model        VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version        VARCHAR(96) NOT NULL DEFAULT '',
    confidence            DOUBLE PRECISION,
    review_reason         VARCHAR(1000) NOT NULL DEFAULT '',
    review_category       VARCHAR(120) NOT NULL DEFAULT '',
    human_review_status   VARCHAR(24) NOT NULL DEFAULT 'pending',
    reviewed_by           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at           TIMESTAMPTZ,
    created_by            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_risk_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_risk_field CHECK (
        observed_field IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_risk_status CHECK (
        status IN ('active', 'disabled')
    ),
    CONSTRAINT chk_instruction_audit_v2_risk_source CHECK (
        source IN ('sync_ai', 'async_ai', 'manual')
    ),
    CONSTRAINT chk_instruction_audit_v2_risk_human_status CHECK (
        human_review_status IN ('pending', 'confirmed_risk')
    ),
    CONSTRAINT chk_instruction_audit_v2_risk_confidence CHECK (
        confidence IS NULL OR confidence BETWEEN 0 AND 1
    )
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_risk_runtime
    ON instruction_audit_v2_risk_hashes(status, sha256);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_risk_created
    ON instruction_audit_v2_risk_hashes(created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_review_jobs (
    id                    BIGSERIAL PRIMARY KEY,
    sha256                CHAR(64) NOT NULL UNIQUE,
    content_vault_id      BIGINT NOT NULL
        REFERENCES instruction_audit_v2_content_vault(id) ON DELETE RESTRICT,
    selected_field        VARCHAR(16) NOT NULL,
    source_event_id       BIGINT REFERENCES instruction_audit_v2_events(id) ON DELETE SET NULL,
    status                VARCHAR(16) NOT NULL DEFAULT 'pending',
    final_result          VARCHAR(16) NOT NULL DEFAULT '',
    pass_votes            SMALLINT NOT NULL DEFAULT 0,
    reject_votes          SMALLINT NOT NULL DEFAULT 0,
    retry_round           INT NOT NULL DEFAULT 0,
    next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner           VARCHAR(128) NOT NULL DEFAULT '',
    lease_expires_at      TIMESTAMPTZ,
    prompt_version        VARCHAR(96) NOT NULL,
    review_criteria       TEXT NOT NULL DEFAULT '',
    config_version        BIGINT NOT NULL,
    observe_only          BOOLEAN NOT NULL DEFAULT FALSE,
    sampled               BOOLEAN NOT NULL DEFAULT FALSE,
    sample_bytes          INT NOT NULL DEFAULT 0,
    content_bytes         BIGINT NOT NULL,
    last_error            VARCHAR(500) NOT NULL DEFAULT '',
    completed_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_job_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_job_field CHECK (
        selected_field IN ('instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_job_status CHECK (
        status IN ('pending', 'processing', 'retry', 'completed', 'failed')
    ),
    CONSTRAINT chk_instruction_audit_v2_job_result CHECK (
        final_result IN ('', 'pass', 'reject')
    ),
    CONSTRAINT chk_instruction_audit_v2_job_votes CHECK (
        pass_votes BETWEEN 0 AND 3 AND reject_votes BETWEEN 0 AND 3
    ),
    CONSTRAINT chk_instruction_audit_v2_job_lengths CHECK (
        retry_round >= 0 AND sample_bytes >= 0 AND content_bytes > 0
    )
);

ALTER TABLE instruction_audit_v2_review_jobs
    ADD COLUMN IF NOT EXISTS review_criteria TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS observe_only BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_jobs_due
    ON instruction_audit_v2_review_jobs(next_attempt_at, id)
    WHERE status IN ('pending', 'retry', 'processing');
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_jobs_status_created
    ON instruction_audit_v2_review_jobs(status, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_review_attempts (
    id                    BIGSERIAL PRIMARY KEY,
    job_id                BIGINT NOT NULL
        REFERENCES instruction_audit_v2_review_jobs(id) ON DELETE CASCADE,
    node_id               BIGINT REFERENCES instruction_audit_v2_ai_nodes(id) ON DELETE SET NULL,
    node_slot             VARCHAR(16) NOT NULL,
    node_name_snapshot    VARCHAR(120) NOT NULL DEFAULT '',
    reviewer_model        VARCHAR(255) NOT NULL DEFAULT '',
    attempt_no            INT NOT NULL,
    result                VARCHAR(16) NOT NULL,
    confidence            DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason                VARCHAR(1000) NOT NULL DEFAULT '',
    category              VARCHAR(120) NOT NULL DEFAULT '',
    prompt_version        VARCHAR(96) NOT NULL,
    sampled               BOOLEAN NOT NULL DEFAULT FALSE,
    latency_ms            INT NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_attempt_slot CHECK (
        node_slot IN ('async_1', 'async_2', 'async_3')
    ),
    CONSTRAINT chk_instruction_audit_v2_attempt_result CHECK (
        result IN ('pass', 'reject', 'uncertain', 'error', 'timeout', 'invalid')
    ),
    CONSTRAINT chk_instruction_audit_v2_attempt_confidence CHECK (
        confidence BETWEEN 0 AND 1
    ),
    CONSTRAINT chk_instruction_audit_v2_attempt_lengths CHECK (
        attempt_no > 0 AND latency_ms >= 0
    ),
    UNIQUE (job_id, node_slot, attempt_no)
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_attempts_job
    ON instruction_audit_v2_review_attempts(job_id, id);

ALTER TABLE instruction_audit_v2_events
    ADD COLUMN IF NOT EXISTS selected_field VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS selected_sha256 CHAR(64),
    ADD COLUMN IF NOT EXISTS review_job_id BIGINT
        REFERENCES instruction_audit_v2_review_jobs(id) ON DELETE SET NULL;

ALTER TABLE instruction_audit_v2_events
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_event_selected_field;
ALTER TABLE instruction_audit_v2_events
    ADD CONSTRAINT chk_instruction_audit_v2_event_selected_field CHECK (
        selected_field IN ('', 'instructions', 'input1')
        AND (selected_sha256 IS NULL OR selected_sha256 ~ '^[0-9a-f]{64}$')
    );

ALTER TABLE instruction_audit_v2_events
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_event_ai;
ALTER TABLE instruction_audit_v2_events
    ADD CONSTRAINT chk_instruction_audit_v2_event_ai CHECK (
        ai_result IN (
            'not_run', 'pass', 'reject', 'uncertain', 'error', 'queue_full',
            'timeout', 'invalid'
        )
        AND ai_reviewed_field IN ('', 'instructions', 'input1')
    );

ALTER TABLE instruction_audit_v2_ai_reviews
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_review_result;
ALTER TABLE instruction_audit_v2_ai_reviews
    ADD CONSTRAINT chk_instruction_audit_v2_review_result CHECK (
        result IN ('pass', 'reject', 'uncertain', 'error', 'timeout', 'invalid')
    );

ALTER TABLE instruction_audit_v2_events
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_event_outcome;
ALTER TABLE instruction_audit_v2_events
    ADD CONSTRAINT chk_instruction_audit_v2_event_outcome CHECK (
        outcome IN (
            'hash_pass', 'ai_pass', 'blocked', 'empty_pass',
            'user_allowlist_pass', 'observe_allow', 'risk_hash_blocked',
            'ai_review_pending'
        )
    );

ALTER TABLE instruction_audit_v2_raw_access_logs
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_access_resource;
ALTER TABLE instruction_audit_v2_raw_access_logs
    ADD CONSTRAINT chk_instruction_audit_v2_access_resource CHECK (
        resource_type IN ('event', 'hash', 'risk_hash', 'review_job')
    );

-- Candidate cleanup indexes no longer represent a valid state.
DROP INDEX IF EXISTS idx_instruction_audit_v2_hashes_candidate_expiry;
DROP INDEX IF EXISTS idx_instruction_audit_v2_hash_scopes_candidate_expiry;
