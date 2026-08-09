-- Instruction Audit V2 intentionally starts with an empty policy set. Legacy
-- tables remain untouched for rollback, but the legacy runtime is disabled.
INSERT INTO settings (key, value, updated_at)
VALUES ('instruction_audit_enabled', 'false', NOW())
ON CONFLICT (key) DO UPDATE SET value = 'false', updated_at = NOW();

ALTER TABLE security_notification_outbox
    DROP CONSTRAINT IF EXISTS chk_security_notification_source;
ALTER TABLE security_notification_outbox
    ADD CONSTRAINT chk_security_notification_source CHECK (
        source_type IN ('instruction_audit', 'instruction_audit_v2', 'cyber_policy')
    );

CREATE TABLE IF NOT EXISTS instruction_audit_v2_config (
    id                      SMALLINT PRIMARY KEY DEFAULT 1,
    mode                    VARCHAR(16) NOT NULL DEFAULT 'off',
    review_criteria         TEXT NOT NULL DEFAULT '',
    confidence_threshold    DOUBLE PRECISION NOT NULL DEFAULT 0.95,
    ai_input_max_chars      INT NOT NULL DEFAULT 64000,
    ai_global_concurrency   INT NOT NULL DEFAULT 64,
    ai_queue_wait_ms        INT NOT NULL DEFAULT 2000,
    ai_total_timeout_ms     INT NOT NULL DEFAULT 30000,
    ai_cache_ttl_seconds    INT NOT NULL DEFAULT 600,
    event_retention_days    INT NOT NULL DEFAULT 30,
    evidence_retention_days INT NOT NULL DEFAULT 7,
    candidate_retention_days INT NOT NULL DEFAULT 30,
    raw_full_max_bytes      INT NOT NULL DEFAULT 4194304,
    config_version          BIGINT NOT NULL DEFAULT 1,
    updated_by              BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_config_id CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_v2_mode CHECK (mode IN ('off', 'observe', 'enforce')),
    CONSTRAINT chk_instruction_audit_v2_confidence CHECK (confidence_threshold BETWEEN 0.5 AND 1),
    CONSTRAINT chk_instruction_audit_v2_ai_input CHECK (ai_input_max_chars BETWEEN 1000 AND 1000000),
    CONSTRAINT chk_instruction_audit_v2_ai_global CHECK (ai_global_concurrency BETWEEN 1 AND 512),
    CONSTRAINT chk_instruction_audit_v2_ai_queue CHECK (ai_queue_wait_ms BETWEEN 0 AND 30000),
    CONSTRAINT chk_instruction_audit_v2_ai_timeout CHECK (ai_total_timeout_ms BETWEEN 1000 AND 30000),
    CONSTRAINT chk_instruction_audit_v2_ai_cache CHECK (ai_cache_ttl_seconds BETWEEN 0 AND 86400),
    CONSTRAINT chk_instruction_audit_v2_event_retention CHECK (event_retention_days BETWEEN 1 AND 3650),
    CONSTRAINT chk_instruction_audit_v2_evidence_retention CHECK (evidence_retention_days BETWEEN 1 AND 365),
    CONSTRAINT chk_instruction_audit_v2_candidate_retention CHECK (candidate_retention_days BETWEEN 1 AND 365),
    CONSTRAINT chk_instruction_audit_v2_raw_limit CHECK (raw_full_max_bytes BETWEEN 65536 AND 16777216),
    CONSTRAINT chk_instruction_audit_v2_config_version CHECK (config_version > 0)
);

INSERT INTO instruction_audit_v2_config (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_v2_ai_nodes (
    id                  BIGSERIAL PRIMARY KEY,
    name                VARCHAR(120) NOT NULL,
    base_url            VARCHAR(2048) NOT NULL,
    model               VARCHAR(255) NOT NULL,
    api_key_ciphertext  TEXT NOT NULL DEFAULT '',
    priority            INT NOT NULL DEFAULT 100,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    timeout_ms          INT NOT NULL DEFAULT 15000,
    max_concurrency     INT NOT NULL DEFAULT 16,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_node_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_instruction_audit_v2_node_base CHECK (btrim(base_url) <> ''),
    CONSTRAINT chk_instruction_audit_v2_node_model CHECK (btrim(model) <> ''),
    CONSTRAINT chk_instruction_audit_v2_node_priority CHECK (priority BETWEEN 0 AND 100000),
    CONSTRAINT chk_instruction_audit_v2_node_timeout CHECK (timeout_ms BETWEEN 100 AND 30000),
    CONSTRAINT chk_instruction_audit_v2_node_concurrency CHECK (max_concurrency BETWEEN 1 AND 256)
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_nodes_runtime
    ON instruction_audit_v2_ai_nodes(enabled, priority, id);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_client_profiles (
    id                  BIGSERIAL PRIMARY KEY,
    profile_key         VARCHAR(64) NOT NULL UNIQUE,
    name                VARCHAR(120) NOT NULL,
    description         VARCHAR(500) NOT NULL DEFAULT '',
    matchers            JSONB NOT NULL DEFAULT '[]'::jsonb,
    priority            INT NOT NULL DEFAULT 100,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    built_in            BOOLEAN NOT NULL DEFAULT FALSE,
    immutable_internal  BOOLEAN NOT NULL DEFAULT FALSE,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_profile_key CHECK (profile_key ~ '^[a-z][a-z0-9_]{1,63}$'),
    CONSTRAINT chk_instruction_audit_v2_profile_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_instruction_audit_v2_profile_matchers CHECK (jsonb_typeof(matchers) = 'array'),
    CONSTRAINT chk_instruction_audit_v2_profile_priority CHECK (priority BETWEEN 0 AND 100000),
    CONSTRAINT chk_instruction_audit_v2_internal_profile CHECK (
        NOT immutable_internal OR (profile_key = 'modelport_internal' AND built_in)
    )
);

INSERT INTO instruction_audit_v2_client_profiles
    (profile_key, name, description, matchers, priority, enabled, built_in, immutable_internal)
VALUES
    ('codex_vscode', 'Codex VS Code', 'Codex VS Code and Copilot integrations',
     '[{"type":"prefix","value":"codex_vscode/","case_sensitive":false},{"type":"prefix","value":"codex_vscode_copilot/","case_sensitive":false}]'::jsonb,
     10, TRUE, TRUE, FALSE),
    ('codex_cli', 'Codex CLI', 'Codex command-line and terminal clients',
     '[{"type":"prefix","value":"codex_cli_rs/","case_sensitive":false},{"type":"prefix","value":"codex-tui/","case_sensitive":false}]'::jsonb,
     20, TRUE, TRUE, FALSE),
    ('codex_desktop', 'Codex Desktop', 'Codex desktop clients',
     '[{"type":"prefix","value":"Codex Desktop/","case_sensitive":false},{"type":"prefix","value":"codex_chatgpt_desktop/","case_sensitive":false}]'::jsonb,
     30, TRUE, TRUE, FALSE),
    ('opencode', 'OpenCode', 'OpenCode clients',
     '[{"type":"prefix","value":"opencode/","case_sensitive":false}]'::jsonb,
     40, TRUE, TRUE, FALSE),
    ('modelport_internal', 'ModelPort Internal', 'Trusted internal calls only; never inferred from User-Agent',
     '[]'::jsonb, 0, TRUE, TRUE, TRUE),
    ('other', 'Other', 'A valid User-Agent that did not match another enabled profile',
     '[]'::jsonb, 100000, TRUE, TRUE, FALSE),
    ('unknown', 'Unknown', 'Missing or invalid User-Agent',
     '[]'::jsonb, 100000, TRUE, TRUE, FALSE)
ON CONFLICT (profile_key) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_profiles_runtime
    ON instruction_audit_v2_client_profiles(enabled, priority, id);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_scopes (
    id                  BIGSERIAL PRIMARY KEY,
    group_id            BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    client_profile_id   BIGINT REFERENCES instruction_audit_v2_client_profiles(id) ON DELETE RESTRICT,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_v2_scope_profile
    ON instruction_audit_v2_scopes(group_id, client_profile_id)
    WHERE client_profile_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_v2_scope_all_clients
    ON instruction_audit_v2_scopes(group_id)
    WHERE client_profile_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_scopes_runtime
    ON instruction_audit_v2_scopes(group_id, enabled, client_profile_id);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_user_allowlist (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note                VARCHAR(500) NOT NULL DEFAULT '',
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id)
);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_hashes (
    id                  BIGSERIAL PRIMARY KEY,
    sha256              CHAR(64) NOT NULL UNIQUE,
    name                VARCHAR(160) NOT NULL DEFAULT '',
    note                VARCHAR(1000) NOT NULL DEFAULT '',
    status              VARCHAR(16) NOT NULL DEFAULT 'candidate',
    source              VARCHAR(16) NOT NULL,
    observed_field      VARCHAR(16) NOT NULL DEFAULT '',
    hash_algorithm      VARCHAR(16) NOT NULL DEFAULT 'sha256',
    normalization_version VARCHAR(32) NOT NULL DEFAULT 'identity_utf8_v1',
    content_bytes       BIGINT NOT NULL DEFAULT 0,
    raw_storage         VARCHAR(24) NOT NULL DEFAULT 'unavailable',
    raw_ciphertext      BYTEA,
    stored_bytes        INT NOT NULL DEFAULT 0,
    ai_sampled          BOOLEAN NOT NULL DEFAULT FALSE,
    source_event_id     BIGINT,
    reviewer_node_id    BIGINT REFERENCES instruction_audit_v2_ai_nodes(id) ON DELETE SET NULL,
    reviewer_model      VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version      VARCHAR(96) NOT NULL DEFAULT '',
    confidence          DOUBLE PRECISION,
    review_reason       VARCHAR(1000) NOT NULL DEFAULT '',
    review_category     VARCHAR(120) NOT NULL DEFAULT '',
    candidate_expires_at TIMESTAMPTZ,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_hash_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_hash_status CHECK (status IN ('candidate', 'active', 'disabled', 'revoked')),
    CONSTRAINT chk_instruction_audit_v2_hash_source CHECK (source IN ('manual', 'ai_review', 'import')),
    CONSTRAINT chk_instruction_audit_v2_hash_field CHECK (observed_field IN ('', 'instructions', 'input1')),
    CONSTRAINT chk_instruction_audit_v2_hash_identity CHECK (
        hash_algorithm = 'sha256' AND normalization_version = 'identity_utf8_v1'
    ),
    CONSTRAINT chk_instruction_audit_v2_hash_lengths CHECK (
        content_bytes >= 0 AND stored_bytes >= 0
    ),
    CONSTRAINT chk_instruction_audit_v2_hash_raw CHECK (
        (raw_storage IN ('full', 'sample') AND raw_ciphertext IS NOT NULL AND stored_bytes > 0)
        OR (raw_storage = 'unavailable' AND raw_ciphertext IS NULL AND stored_bytes = 0)
    ),
    CONSTRAINT chk_instruction_audit_v2_hash_confidence CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1),
    CONSTRAINT chk_instruction_audit_v2_hash_candidate_expiry CHECK (
        status <> 'candidate' OR candidate_expires_at IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_hashes_status_created
    ON instruction_audit_v2_hashes(status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_hashes_candidate_expiry
    ON instruction_audit_v2_hashes(candidate_expires_at, id)
    WHERE status = 'candidate';

CREATE TABLE IF NOT EXISTS instruction_audit_v2_hash_scopes (
    hash_id             BIGINT NOT NULL REFERENCES instruction_audit_v2_hashes(id) ON DELETE CASCADE,
    scope_id            BIGINT NOT NULL REFERENCES instruction_audit_v2_scopes(id) ON DELETE CASCADE,
    status              VARCHAR(16) NOT NULL DEFAULT 'candidate',
    source              VARCHAR(16) NOT NULL DEFAULT 'manual',
    candidate_expires_at TIMESTAMPTZ,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (hash_id, scope_id),
    CONSTRAINT chk_instruction_audit_v2_hash_scope_status CHECK (
        status IN ('candidate', 'active', 'disabled', 'revoked')
    ),
    CONSTRAINT chk_instruction_audit_v2_hash_scope_source CHECK (
        source IN ('manual', 'ai_review', 'import')
    ),
    CONSTRAINT chk_instruction_audit_v2_hash_scope_expiry CHECK (
        status <> 'candidate' OR candidate_expires_at IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_hash_scopes_scope
    ON instruction_audit_v2_hash_scopes(scope_id, status, hash_id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_hash_scopes_candidate_expiry
    ON instruction_audit_v2_hash_scopes(candidate_expires_at, hash_id, scope_id)
    WHERE status = 'candidate';

CREATE TABLE IF NOT EXISTS instruction_audit_v2_events (
    id                    BIGSERIAL PRIMARY KEY,
    request_id            VARCHAR(128) NOT NULL DEFAULT '',
    user_id               BIGINT REFERENCES users(id) ON DELETE SET NULL,
    user_email_snapshot   VARCHAR(255) NOT NULL DEFAULT '',
    api_key_id            BIGINT,
    api_key_name_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    group_id              BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    group_name_snapshot   VARCHAR(255) NOT NULL DEFAULT '',
    scope_id              BIGINT REFERENCES instruction_audit_v2_scopes(id) ON DELETE SET NULL,
    client_profile_id     BIGINT REFERENCES instruction_audit_v2_client_profiles(id) ON DELETE SET NULL,
    client_key_snapshot   VARCHAR(64) NOT NULL DEFAULT 'unknown',
    client_name_snapshot  VARCHAR(120) NOT NULL DEFAULT 'Unknown',
    client_user_agent     VARCHAR(512) NOT NULL DEFAULT '',
    model                 VARCHAR(255) NOT NULL DEFAULT '',
    endpoint              VARCHAR(255) NOT NULL DEFAULT '',
    stage                 VARCHAR(32) NOT NULL DEFAULT '',
    mode                  VARCHAR(16) NOT NULL,
    decision              VARCHAR(16) NOT NULL,
    outcome               VARCHAR(32) NOT NULL,
    reason                VARCHAR(64) NOT NULL,
    instructions_state    VARCHAR(16) NOT NULL DEFAULT 'not_checked',
    instructions_sha256   CHAR(64),
    instructions_bytes    BIGINT NOT NULL DEFAULT 0,
    instructions_partial  BOOLEAN NOT NULL DEFAULT FALSE,
    input1_state          VARCHAR(16) NOT NULL DEFAULT 'not_checked',
    input1_sha256         CHAR(64),
    input1_bytes          BIGINT NOT NULL DEFAULT 0,
    input1_partial        BOOLEAN NOT NULL DEFAULT FALSE,
    matched_hash_id       BIGINT REFERENCES instruction_audit_v2_hashes(id) ON DELETE SET NULL,
    ai_result             VARCHAR(16) NOT NULL DEFAULT 'not_run',
    ai_reviewed_field     VARCHAR(16) NOT NULL DEFAULT '',
    ai_sampled            BOOLEAN NOT NULL DEFAULT FALSE,
    audit_latency_ms      INT NOT NULL DEFAULT 0,
    ai_latency_ms         INT NOT NULL DEFAULT 0,
    body_bytes            BIGINT NOT NULL DEFAULT 0,
    config_version        BIGINT NOT NULL,
    evidence_status       VARCHAR(24) NOT NULL DEFAULT 'not_stored',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_event_mode CHECK (mode IN ('observe', 'enforce')),
    CONSTRAINT chk_instruction_audit_v2_event_decision CHECK (decision IN ('allow', 'block')),
    CONSTRAINT chk_instruction_audit_v2_event_outcome CHECK (
        outcome IN ('hash_pass', 'ai_pass', 'blocked', 'empty_pass', 'user_allowlist_pass', 'observe_allow')
    ),
    CONSTRAINT chk_instruction_audit_v2_event_field_state CHECK (
        instructions_state IN ('not_checked', 'missing', 'empty', 'valid', 'invalid')
        AND input1_state IN ('not_checked', 'missing', 'empty', 'valid', 'invalid')
    ),
    CONSTRAINT chk_instruction_audit_v2_event_digest CHECK (
        (instructions_sha256 IS NULL OR instructions_sha256 ~ '^[0-9a-f]{64}$')
        AND (input1_sha256 IS NULL OR input1_sha256 ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT chk_instruction_audit_v2_event_lengths CHECK (
        instructions_bytes >= 0 AND input1_bytes >= 0 AND body_bytes >= 0
        AND audit_latency_ms >= 0 AND ai_latency_ms >= 0
    ),
    CONSTRAINT chk_instruction_audit_v2_event_ai CHECK (
        ai_result IN ('not_run', 'pass', 'reject', 'uncertain', 'error', 'queue_full')
        AND ai_reviewed_field IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_event_evidence CHECK (
        evidence_status IN ('not_stored', 'stored', 'partial', 'encryption_unavailable')
    )
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_instruction_audit_v2_hash_source_event'
          AND conrelid = 'instruction_audit_v2_hashes'::regclass
    ) THEN
        ALTER TABLE instruction_audit_v2_hashes
            ADD CONSTRAINT fk_instruction_audit_v2_hash_source_event
            FOREIGN KEY (source_event_id) REFERENCES instruction_audit_v2_events(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_events_created
    ON instruction_audit_v2_events(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_events_group_created
    ON instruction_audit_v2_events(group_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_events_user_created
    ON instruction_audit_v2_events(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_events_outcome_created
    ON instruction_audit_v2_events(outcome, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_events_client_created
    ON instruction_audit_v2_events(client_key_snapshot, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_events_request_id
    ON instruction_audit_v2_events(request_id)
    WHERE request_id <> '';

CREATE TABLE IF NOT EXISTS instruction_audit_v2_event_evidence (
    id                  BIGSERIAL PRIMARY KEY,
    event_id            BIGINT NOT NULL REFERENCES instruction_audit_v2_events(id) ON DELETE CASCADE,
    field_name          VARCHAR(16) NOT NULL,
    sha256              CHAR(64) NOT NULL,
    storage_kind        VARCHAR(16) NOT NULL,
    ciphertext          BYTEA NOT NULL,
    content_bytes       BIGINT NOT NULL,
    stored_bytes        INT NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, field_name),
    CONSTRAINT chk_instruction_audit_v2_evidence_field CHECK (field_name IN ('instructions', 'input1')),
    CONSTRAINT chk_instruction_audit_v2_evidence_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_evidence_kind CHECK (storage_kind IN ('full', 'sample')),
    CONSTRAINT chk_instruction_audit_v2_evidence_lengths CHECK (content_bytes > 0 AND stored_bytes > 0)
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_evidence_expiry
    ON instruction_audit_v2_event_evidence(expires_at, id);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_ai_reviews (
    id                  BIGSERIAL PRIMARY KEY,
    event_id            BIGINT NOT NULL REFERENCES instruction_audit_v2_events(id) ON DELETE CASCADE,
    node_id             BIGINT REFERENCES instruction_audit_v2_ai_nodes(id) ON DELETE SET NULL,
    node_name_snapshot  VARCHAR(120) NOT NULL DEFAULT '',
    reviewer_model      VARCHAR(255) NOT NULL DEFAULT '',
    field_name          VARCHAR(16) NOT NULL,
    sha256              CHAR(64) NOT NULL,
    result              VARCHAR(16) NOT NULL,
    confidence          DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason              VARCHAR(1000) NOT NULL DEFAULT '',
    category            VARCHAR(120) NOT NULL DEFAULT '',
    prompt_version      VARCHAR(96) NOT NULL,
    sampled             BOOLEAN NOT NULL DEFAULT FALSE,
    cached              BOOLEAN NOT NULL DEFAULT FALSE,
    latency_ms          INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_review_field CHECK (field_name IN ('instructions', 'input1')),
    CONSTRAINT chk_instruction_audit_v2_review_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_review_result CHECK (result IN ('pass', 'reject', 'uncertain', 'error')),
    CONSTRAINT chk_instruction_audit_v2_review_confidence CHECK (confidence BETWEEN 0 AND 1),
    CONSTRAINT chk_instruction_audit_v2_review_latency CHECK (latency_ms >= 0)
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_reviews_event
    ON instruction_audit_v2_ai_reviews(event_id, id);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_raw_access_logs (
    id                  BIGSERIAL PRIMARY KEY,
    resource_type       VARCHAR(16) NOT NULL,
    resource_id         BIGINT NOT NULL,
    field_name          VARCHAR(16) NOT NULL DEFAULT '',
    action              VARCHAR(16) NOT NULL,
    actor_id            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    request_id          VARCHAR(128) NOT NULL DEFAULT '',
    client_ip           VARCHAR(64) NOT NULL DEFAULT '',
    user_agent          VARCHAR(512) NOT NULL DEFAULT '',
    succeeded           BOOLEAN NOT NULL DEFAULT TRUE,
    error_code          VARCHAR(64) NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_access_resource CHECK (resource_type IN ('event', 'hash')),
    CONSTRAINT chk_instruction_audit_v2_access_field CHECK (field_name IN ('', 'instructions', 'input1')),
    CONSTRAINT chk_instruction_audit_v2_access_action CHECK (action IN ('reveal', 'copy'))
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_access_resource
    ON instruction_audit_v2_raw_access_logs(resource_type, resource_id, created_at DESC, id DESC);
