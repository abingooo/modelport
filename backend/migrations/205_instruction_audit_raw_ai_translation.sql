CREATE TABLE IF NOT EXISTS instruction_audit_runtime_config (
    id                              SMALLINT PRIMARY KEY DEFAULT 1,
    max_body_bytes                  BIGINT NOT NULL DEFAULT 67108864,
    parse_timeout_ms                INT NOT NULL DEFAULT 500,
    max_inflight_body_bytes         BIGINT NOT NULL DEFAULT 268435456,
    pass_event_retention_days       INT NOT NULL DEFAULT 7,
    raw_content_retention_days      INT NOT NULL DEFAULT 30,
    ai_enabled                      BOOLEAN NOT NULL DEFAULT FALSE,
    ai_base_url                     TEXT NOT NULL DEFAULT '',
    ai_model                        VARCHAR(255) NOT NULL DEFAULT '',
    ai_token_ciphertext             TEXT NOT NULL DEFAULT '',
    ai_timeout_ms                   INT NOT NULL DEFAULT 5000,
    ai_max_concurrency              INT NOT NULL DEFAULT 8,
    ai_min_confidence               DOUBLE PRECISION NOT NULL DEFAULT 0.95,
    ai_per_user_rpm                 INT NOT NULL DEFAULT 2,
    ai_per_user_daily_limit         INT NOT NULL DEFAULT 10,
    ai_global_daily_limit           INT NOT NULL DEFAULT 100,
    ai_prompt_version               VARCHAR(64) NOT NULL DEFAULT 'instruction-review-v1',
    translation_enabled             BOOLEAN NOT NULL DEFAULT FALSE,
    external_translation_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    translation_base_url            TEXT NOT NULL DEFAULT '',
    translation_model               VARCHAR(255) NOT NULL DEFAULT '',
    translation_token_ciphertext    TEXT NOT NULL DEFAULT '',
    translation_timeout_ms          INT NOT NULL DEFAULT 15000,
    translation_max_concurrency     INT NOT NULL DEFAULT 2,
    translation_chunk_bytes         INT NOT NULL DEFAULT 12000,
    translation_max_bytes           INT NOT NULL DEFAULT 1048576,
    translation_result_ttl_seconds  INT NOT NULL DEFAULT 1800,
    updated_by                      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_runtime_singleton CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_runtime_body_limit CHECK (
        max_body_bytes BETWEEN 1048576 AND 134217728
    ),
    CONSTRAINT chk_instruction_audit_runtime_parse_timeout CHECK (
        parse_timeout_ms BETWEEN 50 AND 5000
    ),
    CONSTRAINT chk_instruction_audit_runtime_inflight_limit CHECK (
        max_inflight_body_bytes BETWEEN max_body_bytes AND 2147483648
    ),
    CONSTRAINT chk_instruction_audit_runtime_retention CHECK (
        pass_event_retention_days BETWEEN 1 AND 90
        AND raw_content_retention_days BETWEEN 1 AND 3650
    ),
    CONSTRAINT chk_instruction_audit_runtime_ai CHECK (
        ai_timeout_ms BETWEEN 100 AND 30000
        AND ai_max_concurrency BETWEEN 1 AND 64
        AND ai_min_confidence BETWEEN 0.5 AND 1.0
        AND ai_per_user_rpm BETWEEN 1 AND 120
        AND ai_per_user_daily_limit BETWEEN 1 AND 1000
        AND ai_global_daily_limit BETWEEN 1 AND 100000
    ),
    CONSTRAINT chk_instruction_audit_runtime_translation CHECK (
        translation_timeout_ms BETWEEN 100 AND 60000
        AND translation_max_concurrency BETWEEN 1 AND 16
        AND translation_chunk_bytes BETWEEN 1024 AND 65536
        AND translation_max_bytes BETWEEN translation_chunk_bytes AND 1048576
        AND translation_result_ttl_seconds BETWEEN 60 AND 86400
    )
);

INSERT INTO instruction_audit_runtime_config (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_hash_raw_contents (
    hash_id                 BIGINT PRIMARY KEY REFERENCES instruction_audit_hashes(id) ON DELETE CASCADE,
    ciphertext              BYTEA,
    raw_content_status      VARCHAR(32) NOT NULL DEFAULT 'raw_content_unavailable',
    content_bytes           INT NOT NULL DEFAULT 0,
    hash_algorithm          VARCHAR(24) NOT NULL DEFAULT 'sha256',
    normalization_version   VARCHAR(64) NOT NULL DEFAULT 'identity_utf8_v1',
    encryption_key_version  VARCHAR(64) NOT NULL DEFAULT '',
    raw_expires_at          TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_hash_raw_status CHECK (
        raw_content_status IN (
            'stored', 'raw_content_unavailable', 'expired', 'encryption_unavailable'
        )
        AND content_bytes >= 0
        AND hash_algorithm = 'sha256'
        AND normalization_version = 'identity_utf8_v1'
        AND (
            (
                raw_content_status = 'stored'
                AND ciphertext IS NOT NULL
                AND octet_length(ciphertext) > 0
                AND content_bytes > 0
                AND btrim(encryption_key_version) <> ''
                AND raw_expires_at IS NOT NULL
            )
            OR (
                raw_content_status <> 'stored'
                AND ciphertext IS NULL
                AND encryption_key_version = ''
            )
        )
    )
);

INSERT INTO instruction_audit_hash_raw_contents (hash_id, raw_content_status)
SELECT h.id, 'raw_content_unavailable'
FROM instruction_audit_hashes h
ON CONFLICT (hash_id) DO NOTHING;

ALTER TABLE instruction_audit_hashes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_hash_status;
ALTER TABLE instruction_audit_hashes
    ADD CONSTRAINT chk_instruction_audit_hash_status CHECK (
        status IN ('candidate', 'active', 'disabled', 'expired', 'revoked')
    );

ALTER TABLE instruction_audit_rule_sets
    ADD COLUMN IF NOT EXISTS system_managed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE instruction_audit_rule_sets
    ADD COLUMN IF NOT EXISTS system_key VARCHAR(160) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_rule_sets_system_key
    ON instruction_audit_rule_sets(system_key)
    WHERE system_managed = TRUE AND system_key <> '';

CREATE TABLE IF NOT EXISTS instruction_audit_ai_reviews (
    id                    BIGSERIAL PRIMARY KEY,
    event_id              BIGINT REFERENCES instruction_audit_events(id) ON DELETE SET NULL,
    request_id            VARCHAR(128) NOT NULL DEFAULT '',
    user_id               BIGINT REFERENCES users(id) ON DELETE SET NULL,
    group_id              BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    client_type           VARCHAR(32) NOT NULL DEFAULT 'unknown',
    model                 VARCHAR(255) NOT NULL DEFAULT '',
    reviewed_source       VARCHAR(24) NOT NULL,
    reviewed_sha256       CHAR(64) NOT NULL,
    result                VARCHAR(24) NOT NULL,
    approved_source       VARCHAR(24),
    confidence            DOUBLE PRECISION NOT NULL DEFAULT 0,
    review_reason         VARCHAR(1000) NOT NULL DEFAULT '',
    reviewer_model        VARCHAR(255) NOT NULL,
    prompt_version        VARCHAR(64) NOT NULL,
    latency_ms            INT NOT NULL DEFAULT 0,
    automatic_hash_id     BIGINT REFERENCES instruction_audit_hashes(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_ai_source CHECK (
        reviewed_source IN ('instructions', 'input1')
        AND (approved_source IS NULL OR approved_source IN ('instructions', 'input1'))
    ),
    CONSTRAINT chk_instruction_audit_ai_digest CHECK (reviewed_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_ai_result CHECK (result IN ('pass', 'reject', 'uncertain', 'error')),
    CONSTRAINT chk_instruction_audit_ai_confidence CHECK (confidence BETWEEN 0 AND 1),
    CONSTRAINT chk_instruction_audit_ai_latency CHECK (latency_ms >= 0)
);

ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS ai_review_id BIGINT REFERENCES instruction_audit_ai_reviews(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS instruction_audit_hash_sources (
    id                    BIGSERIAL PRIMARY KEY,
    hash_id               BIGINT NOT NULL REFERENCES instruction_audit_hashes(id) ON DELETE CASCADE,
    source_type           VARCHAR(24) NOT NULL,
    field_name            VARCHAR(24) NOT NULL DEFAULT '',
    event_id              BIGINT REFERENCES instruction_audit_events(id) ON DELETE SET NULL,
    ai_review_id          BIGINT REFERENCES instruction_audit_ai_reviews(id) ON DELETE SET NULL,
    reviewer_model        VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version        VARCHAR(64) NOT NULL DEFAULT '',
    confidence            DOUBLE PRECISION,
    review_reason         VARCHAR(1000) NOT NULL DEFAULT '',
    created_by            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_hash_source_type CHECK (
        source_type IN ('manual', 'ai_review', 'import')
    ),
    CONSTRAINT chk_instruction_audit_hash_source_field CHECK (
        field_name IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_hash_source_confidence CHECK (
        confidence IS NULL OR confidence BETWEEN 0 AND 1
    )
);

INSERT INTO instruction_audit_hash_sources (hash_id, source_type, field_name, created_by, created_at)
SELECT h.id, 'import', h.observed_source, h.created_by, h.created_at
FROM instruction_audit_hashes h
WHERE NOT EXISTS (
    SELECT 1 FROM instruction_audit_hash_sources s WHERE s.hash_id = h.id
);

CREATE TABLE IF NOT EXISTS instruction_audit_sensitive_access_logs (
    id                    BIGSERIAL PRIMARY KEY,
    resource_type         VARCHAR(32) NOT NULL,
    resource_id           BIGINT NOT NULL,
    actor_id              BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action                VARCHAR(24) NOT NULL,
    request_id            VARCHAR(128) NOT NULL DEFAULT '',
    client_ip             VARCHAR(64) NOT NULL DEFAULT '',
    user_agent            VARCHAR(512) NOT NULL DEFAULT '',
    succeeded             BOOLEAN NOT NULL DEFAULT TRUE,
    error_code            VARCHAR(64) NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_sensitive_resource CHECK (
        resource_type IN ('event_evidence', 'hash_raw', 'translation', 'ai_hash')
    ),
    CONSTRAINT chk_instruction_audit_sensitive_action CHECK (
        action IN ('reveal', 'copy', 'translate', 'promote', 'disable', 'revoke')
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_translation_jobs (
    id                    BIGSERIAL PRIMARY KEY,
    resource_type         VARCHAR(24) NOT NULL,
    resource_id           BIGINT NOT NULL,
    field_name            VARCHAR(24) NOT NULL,
    target_language       VARCHAR(32) NOT NULL DEFAULT 'zh-CN',
    provider              VARCHAR(24) NOT NULL,
    status                VARCHAR(24) NOT NULL DEFAULT 'pending',
    requested_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    error_code            VARCHAR(64) NOT NULL DEFAULT '',
    chunk_count           INT NOT NULL DEFAULT 0,
    completed_chunks      INT NOT NULL DEFAULT 0,
    expires_at            TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_translation_resource CHECK (
        resource_type IN ('event', 'hash')
        AND field_name IN ('instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_translation_provider CHECK (
        provider IN ('internal', 'external')
    ),
    CONSTRAINT chk_instruction_audit_translation_status CHECK (
        status IN ('pending', 'processing', 'succeeded', 'partial', 'failed', 'expired')
    ),
    CONSTRAINT chk_instruction_audit_translation_progress CHECK (
        chunk_count >= 0 AND completed_chunks >= 0 AND completed_chunks <= chunk_count
    )
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_hashes_raw_expiry
    ON instruction_audit_hash_raw_contents(raw_expires_at)
    WHERE raw_content_status = 'stored';
CREATE INDEX IF NOT EXISTS idx_instruction_audit_hash_sources_hash_created
    ON instruction_audit_hash_sources(hash_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_ai_reviews_user_created
    ON instruction_audit_ai_reviews(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_ai_reviews_event_created
    ON instruction_audit_ai_reviews(event_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_ai_reviews_group_client_created
    ON instruction_audit_ai_reviews(group_id, client_type, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_sensitive_access_resource
    ON instruction_audit_sensitive_access_logs(resource_type, resource_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_translation_jobs_expiry
    ON instruction_audit_translation_jobs(expires_at, id);
