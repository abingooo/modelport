-- ModelPort Instruction Audit compatibility bridge.
--
-- This migration declares the final runtime schema directly for a clean
-- Sub2API v0.1.183 database. Databases upgraded through custom-v0.1.176.2
-- already contain these objects, so every compatibility operation is
-- additive and idempotent. Existing audit rows, hashes, ciphertext, key
-- versions, metadata, and relationships are intentionally left untouched.

INSERT INTO settings (key, value)
VALUES ('instruction_audit_enabled', 'false')
ON CONFLICT (key) DO NOTHING;

INSERT INTO settings (key, value)
VALUES ('instruction_audit_evidence_retention_days', '30')
ON CONFLICT (key) DO NOTHING;

-- Legacy Instruction Audit runtime, retained because the current backend
-- still exposes its administration, translation, and statistics APIs.
CREATE TABLE IF NOT EXISTS instruction_audit_state (
    id             SMALLINT PRIMARY KEY DEFAULT 1,
    config_version BIGINT NOT NULL DEFAULT 1,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_state_singleton CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_state_version CHECK (config_version >= 1)
);

INSERT INTO instruction_audit_state (id, config_version)
VALUES (1, 1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_hashes (
    id              BIGSERIAL PRIMARY KEY,
    digest          CHAR(64) NOT NULL UNIQUE,
    name            VARCHAR(160) NOT NULL,
    note            TEXT NOT NULL DEFAULT '',
    observed_source VARCHAR(32) NOT NULL DEFAULT '',
    client_name     VARCHAR(120) NOT NULL DEFAULT '',
    client_version  VARCHAR(120) NOT NULL DEFAULT '',
    status          VARCHAR(24) NOT NULL DEFAULT 'candidate',
    valid_from      TIMESTAMPTZ,
    valid_until     TIMESTAMPTZ,
    created_by      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_hash_digest CHECK (digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_hash_source CHECK (
        observed_source IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_hash_status CHECK (
        status IN ('candidate', 'active', 'disabled', 'expired', 'revoked')
    ),
    CONSTRAINT chk_instruction_audit_hash_validity CHECK (
        valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_rule_sets (
    id                 BIGSERIAL PRIMARY KEY,
    name               VARCHAR(160) NOT NULL UNIQUE,
    description        TEXT NOT NULL DEFAULT '',
    enabled            BOOLEAN NOT NULL DEFAULT TRUE,
    version            BIGINT NOT NULL DEFAULT 1,
    allow_empty_fields BOOLEAN NOT NULL DEFAULT FALSE,
    system_managed     BOOLEAN NOT NULL DEFAULT FALSE,
    system_key         VARCHAR(160) NOT NULL DEFAULT '',
    created_by         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_rule_set_version CHECK (version >= 1)
);

CREATE TABLE IF NOT EXISTS instruction_audit_rule_set_hashes (
    rule_set_id BIGINT NOT NULL REFERENCES instruction_audit_rule_sets(id) ON DELETE CASCADE,
    hash_id     BIGINT NOT NULL REFERENCES instruction_audit_hashes(id) ON DELETE CASCADE,
    source_type VARCHAR(24) NOT NULL DEFAULT 'manual',
    valid_until TIMESTAMPTZ,
    status      VARCHAR(24) NOT NULL DEFAULT 'active',
    created_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (rule_set_id, hash_id),
    CONSTRAINT chk_instruction_audit_rule_set_hash_source CHECK (
        source_type IN ('manual', 'ai_review')
        AND (source_type <> 'ai_review' OR valid_until IS NOT NULL)
    ),
    CONSTRAINT chk_instruction_audit_rule_set_hash_status CHECK (
        status IN ('active', 'disabled', 'revoked')
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_bindings (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    model       VARCHAR(255) NOT NULL,
    rule_set_id BIGINT NOT NULL REFERENCES instruction_audit_rule_sets(id) ON DELETE CASCADE,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_instruction_audit_binding UNIQUE (user_id, model, rule_set_id),
    CONSTRAINT chk_instruction_audit_binding_model CHECK (
        btrim(model) <> '' AND model = btrim(model)
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_group_bindings (
    id           BIGSERIAL PRIMARY KEY,
    group_id     BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    rule_set_id  BIGINT NOT NULL REFERENCES instruction_audit_rule_sets(id) ON DELETE CASCADE,
    client_types TEXT[] NOT NULL DEFAULT ARRAY['all']::TEXT[],
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_instruction_audit_group_binding UNIQUE (group_id, rule_set_id),
    CONSTRAINT chk_instruction_audit_binding_client_types CHECK (
        cardinality(client_types) BETWEEN 1 AND 7
        AND client_types <@ ARRAY[
            'all', 'codex_vscode', 'codex_cli', 'codex_desktop',
            'opencode', 'modelport_internal', 'other', 'unknown'
        ]::TEXT[]
        AND (NOT ('all' = ANY(client_types)) OR client_types = ARRAY['all']::TEXT[])
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_rule_set_users (
    rule_set_id BIGINT NOT NULL REFERENCES instruction_audit_rule_sets(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (rule_set_id, user_id)
);

CREATE TABLE IF NOT EXISTS instruction_audit_events (
    id                    BIGSERIAL PRIMARY KEY,
    request_id            VARCHAR(128) NOT NULL DEFAULT '',
    user_id               BIGINT REFERENCES users(id) ON DELETE SET NULL,
    user_email_snapshot   VARCHAR(320) NOT NULL DEFAULT '',
    api_key_id            BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    group_id              BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    group_name_snapshot   VARCHAR(100) NOT NULL DEFAULT '',
    client_type           VARCHAR(32) NOT NULL DEFAULT 'unknown',
    client_user_agent     VARCHAR(512) NOT NULL DEFAULT '',
    model                 VARCHAR(255) NOT NULL DEFAULT '',
    endpoint              VARCHAR(255) NOT NULL DEFAULT '',
    stage                 VARCHAR(32) NOT NULL DEFAULT 'http',
    instructions_present  BOOLEAN NOT NULL DEFAULT FALSE,
    instructions_sha256   VARCHAR(64) NOT NULL DEFAULT '',
    instructions_result   VARCHAR(24) NOT NULL DEFAULT 'missing',
    input1_present        BOOLEAN NOT NULL DEFAULT FALSE,
    input1_sha256         VARCHAR(64) NOT NULL DEFAULT '',
    input1_result         VARCHAR(24) NOT NULL DEFAULT 'missing',
    decision              VARCHAR(24) NOT NULL DEFAULT 'blocked',
    reason                VARCHAR(64) NOT NULL DEFAULT 'hash_mismatch',
    initial_reason        VARCHAR(64) NOT NULL DEFAULT 'hash_mismatch',
    final_reason          VARCHAR(64) NOT NULL DEFAULT 'hash_mismatch',
    final_outcome         VARCHAR(32) NOT NULL DEFAULT 'blocked',
    policy_action         VARCHAR(32) NOT NULL DEFAULT 'block',
    rule_set_ids          JSONB NOT NULL DEFAULT '[]'::jsonb,
    config_version        BIGINT NOT NULL DEFAULT 1,
    body_bytes            BIGINT,
    latency_ms            INT NOT NULL DEFAULT 0,
    ai_latency_ms         INT,
    evidence_status       VARCHAR(32) NOT NULL DEFAULT 'legacy_unavailable',
    evidence_expires_at   TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_event_instructions_digest CHECK (
        instructions_sha256 = '' OR instructions_sha256 ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_instruction_audit_event_input1_digest CHECK (
        input1_sha256 = '' OR input1_sha256 ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_instruction_audit_event_field_result CHECK (
        instructions_result IN ('missing', 'invalid', 'mismatch', 'match', 'not_checked')
        AND input1_result IN ('missing', 'invalid', 'mismatch', 'match', 'not_checked')
    ),
    CONSTRAINT chk_instruction_audit_event_decision CHECK (
        decision IN ('blocked', 'policy_allow', 'ai_pass', 'hash_pass', 'exception_pass')
    ),
    CONSTRAINT chk_instruction_audit_event_final_outcome CHECK (
        final_outcome IN ('blocked', 'policy_allow', 'ai_pass', 'hash_pass', 'exception_pass')
        AND decision = final_outcome
    ),
    CONSTRAINT chk_instruction_audit_event_policy_action CHECK (
        policy_action IN ('block', 'allow_and_record', 'hash_match', 'exception', 'ai_review')
    ),
    CONSTRAINT chk_instruction_audit_event_rule_sets CHECK (jsonb_typeof(rule_set_ids) = 'array'),
    CONSTRAINT chk_instruction_audit_event_values CHECK (config_version >= 1 AND latency_ms >= 0),
    CONSTRAINT chk_instruction_audit_event_measurements CHECK (
        (body_bytes IS NULL OR body_bytes >= 0)
        AND (ai_latency_ms IS NULL OR ai_latency_ms >= 0)
    ),
    CONSTRAINT chk_instruction_audit_event_evidence_status CHECK (
        evidence_status IN (
            'stored', 'not_available', 'encryption_unavailable',
            'expired', 'legacy_unavailable'
        )
    ),
    CONSTRAINT chk_instruction_audit_event_client_type CHECK (
        client_type IN (
            'codex_vscode', 'codex_cli', 'codex_desktop', 'opencode',
            'modelport_internal', 'other', 'unknown'
        )
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_notification_outbox (
    id                 BIGSERIAL PRIMARY KEY,
    event_id           BIGINT NOT NULL REFERENCES instruction_audit_events(id) ON DELETE CASCADE,
    dedup_key          VARCHAR(64) NOT NULL UNIQUE,
    status             VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempts           INT NOT NULL DEFAULT 0,
    max_attempts       INT NOT NULL DEFAULT 8,
    available_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at         TIMESTAMPTZ,
    sent_recipient_ids BIGINT[] NOT NULL DEFAULT '{}',
    last_error         VARCHAR(512) NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_outbox_status CHECK (
        status IN ('pending', 'processing', 'retry', 'sent', 'failed')
    ),
    CONSTRAINT chk_instruction_audit_outbox_attempts CHECK (
        attempts >= 0 AND max_attempts > 0
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_evidence (
    event_id        BIGINT NOT NULL REFERENCES instruction_audit_events(id) ON DELETE CASCADE,
    source          VARCHAR(24) NOT NULL,
    digest          CHAR(64) NOT NULL,
    ciphertext      BYTEA NOT NULL,
    key_version     VARCHAR(64) NOT NULL,
    plaintext_bytes INT NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, source),
    CONSTRAINT chk_instruction_audit_evidence_source CHECK (
        source IN ('instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_evidence_digest CHECK (digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_evidence_bytes CHECK (plaintext_bytes > 0),
    CONSTRAINT chk_instruction_audit_evidence_ciphertext CHECK (octet_length(ciphertext) > 0)
);

CREATE TABLE IF NOT EXISTS instruction_audit_sensitive_access_grants (
    id                     BIGSERIAL PRIMARY KEY,
    subject_user_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    subject_email_snapshot VARCHAR(255) NOT NULL,
    granted_by             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    grant_source           VARCHAR(32) NOT NULL,
    grant_reason           VARCHAR(255) NOT NULL DEFAULT '',
    granted_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_by             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    revoke_source          VARCHAR(32),
    revoke_reason          VARCHAR(255) NOT NULL DEFAULT '',
    revoked_at             TIMESTAMPTZ,
    CONSTRAINT chk_instruction_audit_sensitive_grant_source CHECK (
        grant_source IN ('setup_bootstrap', 'migration_bootstrap', 'manual', 'emergency_cli')
    ),
    CONSTRAINT chk_instruction_audit_sensitive_revoke_source CHECK (
        revoke_source IS NULL OR revoke_source IN ('manual', 'admin_state_change', 'emergency_cli')
    ),
    CONSTRAINT chk_instruction_audit_sensitive_revoke_state CHECK (
        (revoked_at IS NULL AND revoke_source IS NULL)
        OR (revoked_at IS NOT NULL AND revoke_source IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_evidence_access_logs (
    id                   BIGSERIAL PRIMARY KEY,
    event_id             BIGINT NOT NULL REFERENCES instruction_audit_events(id) ON DELETE CASCADE,
    actor_id             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action               VARCHAR(16) NOT NULL,
    source               VARCHAR(48) NOT NULL,
    request_id           VARCHAR(128) NOT NULL DEFAULT '',
    client_ip            VARCHAR(64) NOT NULL DEFAULT '',
    user_agent           VARCHAR(512) NOT NULL DEFAULT '',
    succeeded            BOOLEAN NOT NULL DEFAULT TRUE,
    error_code           VARCHAR(64) NOT NULL DEFAULT '',
    grant_id             BIGINT REFERENCES instruction_audit_sensitive_access_grants(id) ON DELETE SET NULL,
    auth_method          VARCHAR(24) NOT NULL DEFAULT 'legacy',
    authorization_result VARCHAR(24) NOT NULL DEFAULT 'legacy',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_evidence_access_action CHECK (action IN ('reveal', 'copy')),
    CONSTRAINT chk_instruction_audit_evidence_access_source CHECK (btrim(source) <> '')
);

CREATE TABLE IF NOT EXISTS security_notification_outbox (
    id                    BIGSERIAL PRIMARY KEY,
    source_type           VARCHAR(32) NOT NULL,
    source_id             BIGINT NOT NULL,
    audience              VARCHAR(16) NOT NULL,
    user_id               BIGINT REFERENCES users(id) ON DELETE SET NULL,
    recipients            TEXT[] NOT NULL DEFAULT '{}',
    sent_recipient_hashes TEXT[] NOT NULL DEFAULT '{}',
    template_event        VARCHAR(96) NOT NULL,
    variables             JSONB NOT NULL DEFAULT '{}'::jsonb,
    dedup_key             VARCHAR(64),
    status                VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempts              INT NOT NULL DEFAULT 0,
    max_attempts          INT NOT NULL DEFAULT 8,
    available_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at            TIMESTAMPTZ,
    last_error            VARCHAR(512) NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_security_notification_source_audience UNIQUE (source_type, source_id, audience),
    CONSTRAINT chk_security_notification_source CHECK (
        source_type IN ('instruction_audit', 'instruction_audit_v2', 'cyber_policy')
    ),
    CONSTRAINT chk_security_notification_audience CHECK (audience IN ('user', 'ops')),
    CONSTRAINT chk_security_notification_status CHECK (
        status IN (
            'pending', 'processing', 'retry', 'sent', 'failed',
            'suppressed', 'no_recipient', 'enqueue_failed'
        )
    ),
    CONSTRAINT chk_security_notification_attempts CHECK (
        attempts >= 0 AND max_attempts > 0
    ),
    CONSTRAINT chk_security_notification_variables CHECK (jsonb_typeof(variables) = 'object')
);

CREATE TABLE IF NOT EXISTS instruction_audit_reason_policies (
    reason            VARCHAR(64) PRIMARY KEY,
    action            VARCHAR(32) NOT NULL DEFAULT 'block',
    ai_review_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    alert_enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    allow_until       TIMESTAMPTZ,
    config_version    BIGINT NOT NULL DEFAULT 1,
    updated_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_reason_policy_reason CHECK (
        reason IN (
            'hash_mismatch', 'fields_missing', 'field_invalid', 'invalid_json',
            'request_too_large', 'structure_too_complex', 'parse_timeout',
            'config_unavailable', 'group_not_allowed', 'client_not_allowed',
            'ai_rejected', 'ai_uncertain', 'ai_error'
        )
    ),
    CONSTRAINT chk_instruction_audit_reason_policy_action CHECK (
        action IN ('block', 'allow_and_record')
    ),
    CONSTRAINT chk_instruction_audit_reason_policy_version CHECK (config_version >= 1),
    CONSTRAINT chk_instruction_audit_reason_policy_fail_closed CHECK (
        reason NOT IN ('config_unavailable', 'ai_error') OR action = 'block'
    ),
    CONSTRAINT chk_instruction_audit_reason_policy_no_recursion CHECK (
        reason NOT IN ('ai_rejected', 'ai_uncertain', 'ai_error') OR ai_review_enabled = FALSE
    ),
    CONSTRAINT chk_instruction_audit_reason_policy_temporary_oversize CHECK (
        reason <> 'request_too_large' OR action = 'block' OR allow_until IS NOT NULL
    )
);

INSERT INTO instruction_audit_reason_policies
    (reason, action, ai_review_enabled, alert_enabled, config_version)
VALUES
    ('hash_mismatch', 'block', FALSE, TRUE, 1),
    ('fields_missing', 'block', FALSE, TRUE, 1),
    ('field_invalid', 'block', FALSE, TRUE, 1),
    ('invalid_json', 'block', FALSE, TRUE, 1),
    ('request_too_large', 'block', FALSE, TRUE, 1),
    ('structure_too_complex', 'block', FALSE, TRUE, 1),
    ('parse_timeout', 'block', FALSE, TRUE, 1),
    ('config_unavailable', 'block', FALSE, TRUE, 1),
    ('group_not_allowed', 'block', FALSE, TRUE, 1),
    ('client_not_allowed', 'block', FALSE, TRUE, 1),
    ('ai_rejected', 'block', FALSE, TRUE, 1),
    ('ai_uncertain', 'block', FALSE, TRUE, 1),
    ('ai_error', 'block', FALSE, TRUE, 1)
ON CONFLICT (reason) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_runtime_config (
    id                             SMALLINT PRIMARY KEY DEFAULT 1,
    max_body_bytes                 BIGINT NOT NULL DEFAULT 67108864,
    parse_timeout_ms               INT NOT NULL DEFAULT 500,
    max_inflight_body_bytes        BIGINT NOT NULL DEFAULT 268435456,
    pass_event_retention_days      INT NOT NULL DEFAULT 7,
    aggregate_retention_days       INT NOT NULL DEFAULT 365,
    raw_content_retention_days     INT NOT NULL DEFAULT 30,
    ai_enabled                     BOOLEAN NOT NULL DEFAULT FALSE,
    ai_base_url                    TEXT NOT NULL DEFAULT '',
    ai_model                       VARCHAR(255) NOT NULL DEFAULT '',
    ai_token_ciphertext            TEXT NOT NULL DEFAULT '',
    ai_timeout_ms                  INT NOT NULL DEFAULT 5000,
    ai_max_concurrency             INT NOT NULL DEFAULT 8,
    ai_min_confidence              DOUBLE PRECISION NOT NULL DEFAULT 0.95,
    ai_per_user_rpm                INT NOT NULL DEFAULT 2,
    ai_per_user_daily_limit        INT NOT NULL DEFAULT 10,
    ai_global_daily_limit          INT NOT NULL DEFAULT 100,
    ai_prompt_version              VARCHAR(64) NOT NULL DEFAULT 'instruction-review-v1',
    translation_enabled            BOOLEAN NOT NULL DEFAULT FALSE,
    external_translation_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    translation_base_url           TEXT NOT NULL DEFAULT '',
    translation_model              VARCHAR(255) NOT NULL DEFAULT '',
    translation_token_ciphertext   TEXT NOT NULL DEFAULT '',
    translation_timeout_ms         INT NOT NULL DEFAULT 15000,
    translation_max_concurrency    INT NOT NULL DEFAULT 2,
    translation_chunk_bytes        INT NOT NULL DEFAULT 12000,
    translation_max_bytes          INT NOT NULL DEFAULT 1048576,
    translation_result_ttl_seconds INT NOT NULL DEFAULT 1800,
    updated_by                     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_runtime_singleton CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_runtime_body_limit CHECK (
        max_body_bytes BETWEEN 1048576 AND 134217728
    ),
    CONSTRAINT chk_instruction_audit_runtime_parse_timeout CHECK (
        parse_timeout_ms BETWEEN 50 AND 5000
    ),
    CONSTRAINT chk_instruction_audit_runtime_inflight_limit CHECK (
        max_inflight_body_bytes BETWEEN max_body_bytes * 3 AND 2147483648
    ),
    CONSTRAINT chk_instruction_audit_runtime_retention CHECK (
        pass_event_retention_days BETWEEN 1 AND 90
        AND raw_content_retention_days BETWEEN 1 AND 3650
        AND aggregate_retention_days BETWEEN 30 AND 3650
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
    hash_id                BIGINT PRIMARY KEY REFERENCES instruction_audit_hashes(id) ON DELETE CASCADE,
    ciphertext             BYTEA,
    raw_content_status     VARCHAR(32) NOT NULL DEFAULT 'raw_content_unavailable',
    content_bytes          INT NOT NULL DEFAULT 0,
    hash_algorithm         VARCHAR(24) NOT NULL DEFAULT 'sha256',
    normalization_version  VARCHAR(64) NOT NULL DEFAULT 'identity_utf8_v1',
    encryption_key_version VARCHAR(64) NOT NULL DEFAULT '',
    raw_expires_at         TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_hash_raw_status CHECK (
        raw_content_status IN ('stored', 'raw_content_unavailable', 'encryption_unavailable')
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

CREATE TABLE IF NOT EXISTS instruction_audit_ai_reviews (
    id                BIGSERIAL PRIMARY KEY,
    event_id          BIGINT REFERENCES instruction_audit_events(id) ON DELETE SET NULL,
    request_id        VARCHAR(128) NOT NULL DEFAULT '',
    user_id           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    group_id          BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    client_type       VARCHAR(32) NOT NULL DEFAULT 'unknown',
    model             VARCHAR(255) NOT NULL DEFAULT '',
    reviewed_source   VARCHAR(24) NOT NULL,
    reviewed_sha256   CHAR(64) NOT NULL,
    result            VARCHAR(24) NOT NULL,
    approved_source   VARCHAR(24),
    confidence        DOUBLE PRECISION NOT NULL DEFAULT 0,
    review_reason     VARCHAR(1000) NOT NULL DEFAULT '',
    reviewer_model    VARCHAR(255) NOT NULL,
    prompt_version    VARCHAR(64) NOT NULL,
    latency_ms        INT NOT NULL DEFAULT 0,
    automatic_hash_id BIGINT REFERENCES instruction_audit_hashes(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_ai_source CHECK (
        reviewed_source IN ('instructions', 'input1')
        AND (approved_source IS NULL OR approved_source IN ('instructions', 'input1'))
    ),
    CONSTRAINT chk_instruction_audit_ai_digest CHECK (reviewed_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_ai_result CHECK (
        result IN ('pass', 'reject', 'uncertain', 'error')
    ),
    CONSTRAINT chk_instruction_audit_ai_confidence CHECK (confidence BETWEEN 0 AND 1),
    CONSTRAINT chk_instruction_audit_ai_latency CHECK (latency_ms >= 0)
);

ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS ai_review_id BIGINT
        REFERENCES instruction_audit_ai_reviews(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS instruction_audit_hash_sources (
    id             BIGSERIAL PRIMARY KEY,
    hash_id        BIGINT NOT NULL REFERENCES instruction_audit_hashes(id) ON DELETE CASCADE,
    source_type    VARCHAR(24) NOT NULL,
    field_name     VARCHAR(24) NOT NULL DEFAULT '',
    event_id       BIGINT REFERENCES instruction_audit_events(id) ON DELETE SET NULL,
    ai_review_id   BIGINT REFERENCES instruction_audit_ai_reviews(id) ON DELETE SET NULL,
    reviewer_model VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    confidence     DOUBLE PRECISION,
    review_reason  VARCHAR(1000) NOT NULL DEFAULT '',
    created_by     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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

CREATE TABLE IF NOT EXISTS instruction_audit_sensitive_access_logs (
    id                   BIGSERIAL PRIMARY KEY,
    resource_type        VARCHAR(32) NOT NULL,
    resource_id          BIGINT NOT NULL,
    actor_id             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action               VARCHAR(24) NOT NULL,
    request_id           VARCHAR(128) NOT NULL DEFAULT '',
    client_ip            VARCHAR(64) NOT NULL DEFAULT '',
    user_agent           VARCHAR(512) NOT NULL DEFAULT '',
    succeeded            BOOLEAN NOT NULL DEFAULT TRUE,
    error_code           VARCHAR(64) NOT NULL DEFAULT '',
    scope_rule_set_id    BIGINT REFERENCES instruction_audit_rule_sets(id) ON DELETE SET NULL,
    grant_id             BIGINT REFERENCES instruction_audit_sensitive_access_grants(id) ON DELETE SET NULL,
    auth_method          VARCHAR(24) NOT NULL DEFAULT 'legacy',
    authorization_result VARCHAR(24) NOT NULL DEFAULT 'legacy',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_sensitive_resource CHECK (
        resource_type IN ('event_evidence', 'hash_raw', 'translation', 'ai_hash', 'ai_scope')
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
    authorized_grant_id   BIGINT REFERENCES instruction_audit_sensitive_access_grants(id) ON DELETE SET NULL,
    error_code            VARCHAR(64) NOT NULL DEFAULT '',
    chunk_count           INT NOT NULL DEFAULT 0,
    completed_chunks      INT NOT NULL DEFAULT 0,
    attempts              INT NOT NULL DEFAULT 0,
    max_attempts          INT NOT NULL DEFAULT 3,
    claim_version         BIGINT NOT NULL DEFAULT 0,
    processing_started_at TIMESTAMPTZ,
    next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    result_bytes          INT NOT NULL DEFAULT 0,
    redaction_count       INT NOT NULL DEFAULT 0,
    provider_latency_ms   INT NOT NULL DEFAULT 0,
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
        status IN ('pending', 'processing', 'retry', 'succeeded', 'partial', 'failed', 'expired')
    ),
    CONSTRAINT chk_instruction_audit_translation_progress CHECK (
        chunk_count >= 0 AND completed_chunks >= 0 AND completed_chunks <= chunk_count
    ),
    CONSTRAINT chk_instruction_audit_translation_execution CHECK (
        attempts >= 0
        AND max_attempts BETWEEN 1 AND 10
        AND claim_version >= 0
        AND result_bytes >= 0
        AND redaction_count >= 0
        AND provider_latency_ms >= 0
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_outcome_hourly (
    bucket_at           TIMESTAMPTZ NOT NULL,
    user_id             BIGINT NOT NULL DEFAULT 0,
    group_id            BIGINT NOT NULL DEFAULT 0,
    model               VARCHAR(255) NOT NULL DEFAULT '',
    client_type         VARCHAR(32) NOT NULL DEFAULT 'unknown',
    final_outcome       VARCHAR(32) NOT NULL,
    final_reason        VARCHAR(64) NOT NULL DEFAULT '',
    shard_no            BIGINT NOT NULL DEFAULT 0,
    event_count         BIGINT NOT NULL DEFAULT 0,
    latency_total_ms    BIGINT NOT NULL DEFAULT 0,
    ai_latency_total_ms BIGINT NOT NULL DEFAULT 0,
    event_times         TIMESTAMPTZ[] NOT NULL DEFAULT ARRAY[]::TIMESTAMPTZ[],
    first_event_at      TIMESTAMPTZ NOT NULL,
    last_event_at       TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (
        bucket_at, user_id, group_id, model, client_type,
        final_outcome, final_reason, shard_no
    ),
    CONSTRAINT chk_instruction_audit_outcome_hourly_outcome CHECK (
        final_outcome IN ('blocked', 'policy_allow', 'ai_pass', 'hash_pass', 'exception_pass')
    ),
    CONSTRAINT chk_instruction_audit_outcome_hourly_values CHECK (
        event_count >= 0 AND latency_total_ms >= 0 AND ai_latency_total_ms >= 0
    ),
    CONSTRAINT chk_instruction_audit_outcome_hourly_shard CHECK (shard_no >= 0),
    CONSTRAINT chk_instruction_audit_outcome_hourly_event_times_bounded CHECK (
        cardinality(event_times) <= 4096
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_outcome_rollup_state (
    id                              SMALLINT PRIMARY KEY DEFAULT 1,
    last_event_id                   BIGINT NOT NULL DEFAULT 0,
    expired_aggregate_event_count   BIGINT NOT NULL DEFAULT 0,
    last_aggregate_pruned_at        TIMESTAMPTZ,
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_rollup_singleton CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_rollup_watermark CHECK (last_event_id >= 0),
    CONSTRAINT chk_instruction_audit_rollup_expired_count CHECK (
        expired_aggregate_event_count >= 0
    )
);

INSERT INTO instruction_audit_outcome_rollup_state (id, last_event_id)
VALUES (1, 0)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_operational_counters (
    id                    SMALLINT PRIMARY KEY DEFAULT 1,
    persist_failure_count BIGINT NOT NULL DEFAULT 0,
    statistics_loss_count BIGINT NOT NULL DEFAULT 0,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_operational_counter_id CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_operational_counter_values CHECK (
        persist_failure_count >= 0 AND statistics_loss_count >= 0
    )
);

INSERT INTO instruction_audit_operational_counters (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

-- Preserve the original one-holder bootstrap on databases that have users but
-- no sensitive-access history. Existing grants and revocations are untouched.
INSERT INTO instruction_audit_sensitive_access_grants (
    subject_user_id, subject_email_snapshot, granted_by, grant_source, grant_reason
)
SELECT
    u.id, u.email, NULL, 'migration_bootstrap',
    'Automatic bootstrap for the earliest active administrator'
FROM users u
WHERE u.role = 'admin'
  AND u.status = 'active'
  AND u.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM instruction_audit_sensitive_access_grants)
ORDER BY u.created_at ASC, u.id ASC
LIMIT 1
ON CONFLICT DO NOTHING;

-- Instruction Audit V2 final runtime schema.
CREATE TABLE IF NOT EXISTS instruction_audit_v2_config (
    id                           SMALLINT PRIMARY KEY DEFAULT 1,
    mode                         VARCHAR(16) NOT NULL DEFAULT 'off',
    review_criteria              TEXT NOT NULL DEFAULT '',
    confidence_threshold         DOUBLE PRECISION NOT NULL DEFAULT 0.95,
    ai_input_max_chars           INT NOT NULL DEFAULT 64000,
    ai_global_concurrency        INT NOT NULL DEFAULT 64,
    ai_queue_wait_ms             INT NOT NULL DEFAULT 2000,
    ai_total_timeout_ms          INT NOT NULL DEFAULT 30000,
    ai_cache_ttl_seconds         INT NOT NULL DEFAULT 0,
    event_retention_days         INT NOT NULL DEFAULT 30,
    evidence_retention_days      INT NOT NULL DEFAULT 7,
    candidate_retention_days     INT NOT NULL DEFAULT 30,
    raw_full_max_bytes           INT NOT NULL DEFAULT 4194304,
    allow_empty_fields           BOOLEAN NOT NULL DEFAULT TRUE,
    async_retry_schedule_seconds INT[] NOT NULL DEFAULT ARRAY[30, 120, 600, 3600, 21600],
    config_version               BIGINT NOT NULL DEFAULT 1,
    updated_by                   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
    CONSTRAINT chk_instruction_audit_v2_config_version CHECK (config_version > 0),
    CONSTRAINT chk_instruction_audit_v2_retry_schedule CHECK (
        cardinality(async_retry_schedule_seconds) BETWEEN 1 AND 12
        AND 0 < ALL(async_retry_schedule_seconds)
        AND 604800 >= ALL(async_retry_schedule_seconds)
    )
);

ALTER TABLE instruction_audit_v2_config
    ADD COLUMN IF NOT EXISTS allow_empty_fields BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS async_retry_schedule_seconds INT[] NOT NULL
        DEFAULT ARRAY[30, 120, 600, 3600, 21600];

INSERT INTO instruction_audit_v2_config (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_v2_ai_nodes (
    id                 BIGSERIAL PRIMARY KEY,
    name               VARCHAR(120) NOT NULL,
    base_url           VARCHAR(2048) NOT NULL,
    model              VARCHAR(255) NOT NULL,
    api_key_ciphertext TEXT NOT NULL DEFAULT '',
    priority           INT NOT NULL DEFAULT 100,
    slot               VARCHAR(16) NOT NULL DEFAULT '',
    response_mode      VARCHAR(16) NOT NULL DEFAULT 'auto',
    max_output_tokens  INT NOT NULL DEFAULT 1024,
    enabled            BOOLEAN NOT NULL DEFAULT TRUE,
    timeout_ms         INT NOT NULL DEFAULT 15000,
    max_concurrency    INT NOT NULL DEFAULT 16,
    created_by         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_node_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_instruction_audit_v2_node_base CHECK (btrim(base_url) <> ''),
    CONSTRAINT chk_instruction_audit_v2_node_model CHECK (btrim(model) <> ''),
    CONSTRAINT chk_instruction_audit_v2_node_priority CHECK (priority BETWEEN 0 AND 100000),
    CONSTRAINT chk_instruction_audit_v2_node_timeout CHECK (timeout_ms BETWEEN 100 AND 30000),
    CONSTRAINT chk_instruction_audit_v2_node_concurrency CHECK (max_concurrency BETWEEN 1 AND 256),
    CONSTRAINT chk_instruction_audit_v2_node_slot CHECK (
        slot IN ('', 'sync', 'async_1', 'async_2', 'async_3')
    ),
    CONSTRAINT chk_instruction_audit_v2_node_response_mode CHECK (
        response_mode IN ('auto', 'json_schema', 'json_object')
    ),
    CONSTRAINT chk_instruction_audit_v2_node_output_tokens CHECK (
        max_output_tokens BETWEEN 128 AND 8192
    )
);

ALTER TABLE instruction_audit_v2_ai_nodes
    ADD COLUMN IF NOT EXISTS slot VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS response_mode VARCHAR(16) NOT NULL DEFAULT 'auto',
    ADD COLUMN IF NOT EXISTS max_output_tokens INT NOT NULL DEFAULT 1024;

CREATE TABLE IF NOT EXISTS instruction_audit_v2_client_profiles (
    id                   BIGSERIAL PRIMARY KEY,
    profile_key          VARCHAR(64) NOT NULL UNIQUE,
    name                 VARCHAR(120) NOT NULL,
    description          VARCHAR(500) NOT NULL DEFAULT '',
    matchers             JSONB NOT NULL DEFAULT '[]'::jsonb,
    priority             INT NOT NULL DEFAULT 100,
    enabled              BOOLEAN NOT NULL DEFAULT TRUE,
    prompt_audit_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    built_in             BOOLEAN NOT NULL DEFAULT FALSE,
    immutable_internal   BOOLEAN NOT NULL DEFAULT FALSE,
    created_by           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_profile_key CHECK (
        profile_key ~ '^[a-z][a-z0-9_]{1,63}$'
    ),
    CONSTRAINT chk_instruction_audit_v2_profile_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_instruction_audit_v2_profile_matchers CHECK (jsonb_typeof(matchers) = 'array'),
    CONSTRAINT chk_instruction_audit_v2_profile_priority CHECK (priority BETWEEN 0 AND 100000),
    CONSTRAINT chk_instruction_audit_v2_internal_profile CHECK (
        NOT immutable_internal OR (profile_key = 'modelport_internal' AND built_in)
    ),
    CONSTRAINT chk_instruction_audit_v2_internal_prompt_audit CHECK (
        profile_key <> 'modelport_internal' OR NOT prompt_audit_enabled
    )
);

ALTER TABLE instruction_audit_v2_client_profiles
    ADD COLUMN IF NOT EXISTS prompt_audit_enabled BOOLEAN NOT NULL DEFAULT FALSE;

INSERT INTO instruction_audit_v2_client_profiles
    (profile_key, name, description, matchers, priority, enabled,
     prompt_audit_enabled, built_in, immutable_internal)
VALUES
    ('codex_vscode', 'Codex VS Code', 'Codex VS Code and Copilot integrations',
     '[{"type":"prefix","value":"codex_vscode/","case_sensitive":false},{"type":"prefix","value":"codex_vscode_copilot/","case_sensitive":false}]'::jsonb,
     10, TRUE, FALSE, TRUE, FALSE),
    ('codex_cli', 'Codex CLI', 'Codex command-line and terminal clients',
     '[{"type":"prefix","value":"codex_cli_rs/","case_sensitive":false},{"type":"prefix","value":"codex-tui/","case_sensitive":false}]'::jsonb,
     20, TRUE, FALSE, TRUE, FALSE),
    ('codex_desktop', 'Codex Desktop', 'Codex desktop clients',
     '[{"type":"prefix","value":"Codex Desktop/","case_sensitive":false},{"type":"prefix","value":"codex_chatgpt_desktop/","case_sensitive":false}]'::jsonb,
     30, TRUE, FALSE, TRUE, FALSE),
    ('opencode', 'OpenCode', 'OpenCode clients',
     '[{"type":"prefix","value":"opencode/","case_sensitive":false}]'::jsonb,
     40, TRUE, FALSE, TRUE, FALSE),
    ('modelport_internal', 'ModelPort Internal',
     'Trusted internal calls only; never inferred from User-Agent',
     '[]'::jsonb, 0, TRUE, FALSE, TRUE, TRUE),
    ('other', 'Other', 'A valid User-Agent that did not match another enabled profile',
     '[]'::jsonb, 100000, TRUE, FALSE, TRUE, FALSE),
    ('unknown', 'Unknown', 'Missing or invalid User-Agent',
     '[]'::jsonb, 100000, TRUE, FALSE, TRUE, FALSE)
ON CONFLICT (profile_key) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_v2_scopes (
    id                BIGSERIAL PRIMARY KEY,
    group_id          BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    client_profile_id BIGINT REFERENCES instruction_audit_v2_client_profiles(id) ON DELETE RESTRICT,
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    created_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_user_allowlist (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note       VARCHAR(500) NOT NULL DEFAULT '',
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id)
);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_content_vault (
    id                     BIGSERIAL PRIMARY KEY,
    sha256                 CHAR(64) NOT NULL UNIQUE,
    raw_ciphertext         BYTEA NOT NULL,
    content_bytes          BIGINT NOT NULL,
    stored_bytes           INT NOT NULL,
    observed_field         VARCHAR(16) NOT NULL DEFAULT '',
    encryption_key_version VARCHAR(64) NOT NULL DEFAULT 'instruction-evidence-v1',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_vault_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_vault_field CHECK (
        observed_field IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_vault_lengths CHECK (
        content_bytes > 0 AND stored_bytes > 0 AND octet_length(raw_ciphertext) > 0
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_hashes (
    id                         BIGSERIAL PRIMARY KEY,
    sha256                     CHAR(64) NOT NULL UNIQUE,
    name                       VARCHAR(160) NOT NULL DEFAULT '',
    note                       VARCHAR(1000) NOT NULL DEFAULT '',
    status                     VARCHAR(16) NOT NULL DEFAULT 'active',
    source                     VARCHAR(16) NOT NULL,
    observed_field             VARCHAR(16) NOT NULL DEFAULT '',
    hash_algorithm             VARCHAR(16) NOT NULL DEFAULT 'sha256',
    normalization_version      VARCHAR(32) NOT NULL DEFAULT 'identity_utf8_v1',
    content_bytes              BIGINT NOT NULL DEFAULT 0,
    raw_storage                VARCHAR(24) NOT NULL DEFAULT 'unavailable',
    raw_ciphertext             BYTEA,
    stored_bytes               INT NOT NULL DEFAULT 0,
    content_vault_id           BIGINT REFERENCES instruction_audit_v2_content_vault(id) ON DELETE SET NULL,
    global_trust               BOOLEAN NOT NULL DEFAULT FALSE,
    ai_sampled                 BOOLEAN NOT NULL DEFAULT FALSE,
    source_event_id            BIGINT,
    source_user_id             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    reviewer_node_id           BIGINT REFERENCES instruction_audit_v2_ai_nodes(id) ON DELETE SET NULL,
    reviewer_model             VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version             VARCHAR(96) NOT NULL DEFAULT '',
    confidence                 DOUBLE PRECISION,
    review_reason              VARCHAR(1000) NOT NULL DEFAULT '',
    review_category            VARCHAR(120) NOT NULL DEFAULT '',
    candidate_expires_at       TIMESTAMPTZ,
    created_by                 BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by                 BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_hash_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_hash_status CHECK (
        status IN ('active', 'disabled', 'revoked')
    ),
    CONSTRAINT chk_instruction_audit_v2_hash_source CHECK (
        source IN ('manual', 'ai_review', 'import')
    ),
    CONSTRAINT chk_instruction_audit_v2_hash_field CHECK (
        observed_field IN ('', 'instructions', 'input1')
    ),
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
    CONSTRAINT chk_instruction_audit_v2_hash_confidence CHECK (
        confidence IS NULL OR confidence BETWEEN 0 AND 1
    )
);

ALTER TABLE instruction_audit_v2_hashes
    ADD COLUMN IF NOT EXISTS global_trust BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS content_vault_id BIGINT
        REFERENCES instruction_audit_v2_content_vault(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS instruction_audit_v2_hash_scopes (
    hash_id              BIGINT NOT NULL REFERENCES instruction_audit_v2_hashes(id) ON DELETE CASCADE,
    scope_id             BIGINT NOT NULL REFERENCES instruction_audit_v2_scopes(id) ON DELETE CASCADE,
    status               VARCHAR(16) NOT NULL DEFAULT 'active',
    source               VARCHAR(16) NOT NULL DEFAULT 'manual',
    candidate_expires_at TIMESTAMPTZ,
    created_by           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (hash_id, scope_id),
    CONSTRAINT chk_instruction_audit_v2_hash_scope_status CHECK (
        status IN ('active', 'disabled', 'revoked')
    ),
    CONSTRAINT chk_instruction_audit_v2_hash_scope_source CHECK (
        source IN ('manual', 'ai_review', 'import')
    )
);

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
    selected_field        VARCHAR(16) NOT NULL DEFAULT '',
    selected_sha256       CHAR(64),
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
        outcome IN (
            'hash_pass', 'ai_pass', 'blocked', 'empty_pass',
            'user_allowlist_pass', 'observe_allow', 'risk_hash_blocked',
            'ai_review_pending'
        )
    ),
    CONSTRAINT chk_instruction_audit_v2_event_field_state CHECK (
        instructions_state IN ('not_checked', 'missing', 'empty', 'valid', 'invalid')
        AND input1_state IN ('not_checked', 'missing', 'empty', 'valid', 'invalid')
    ),
    CONSTRAINT chk_instruction_audit_v2_event_digest CHECK (
        (instructions_sha256 IS NULL OR instructions_sha256 ~ '^[0-9a-f]{64}$')
        AND (input1_sha256 IS NULL OR input1_sha256 ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT chk_instruction_audit_v2_event_selected_field CHECK (
        selected_field IN ('', 'instructions', 'input1')
        AND (selected_sha256 IS NULL OR selected_sha256 ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT chk_instruction_audit_v2_event_lengths CHECK (
        instructions_bytes >= 0 AND input1_bytes >= 0 AND body_bytes >= 0
        AND audit_latency_ms >= 0 AND ai_latency_ms >= 0
    ),
    CONSTRAINT chk_instruction_audit_v2_event_ai CHECK (
        ai_result IN (
            'not_run', 'pass', 'reject', 'uncertain', 'error', 'queue_full',
            'timeout', 'invalid'
        )
        AND ai_reviewed_field IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_event_evidence CHECK (
        evidence_status IN ('not_stored', 'stored', 'partial', 'encryption_unavailable')
    )
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'fk_instruction_audit_v2_hash_source_event'
          AND conrelid = 'instruction_audit_v2_hashes'::regclass
    ) THEN
        ALTER TABLE instruction_audit_v2_hashes
            ADD CONSTRAINT fk_instruction_audit_v2_hash_source_event
            FOREIGN KEY (source_event_id)
            REFERENCES instruction_audit_v2_events(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS instruction_audit_v2_event_evidence (
    id            BIGSERIAL PRIMARY KEY,
    event_id      BIGINT NOT NULL REFERENCES instruction_audit_v2_events(id) ON DELETE CASCADE,
    field_name    VARCHAR(16) NOT NULL,
    sha256        CHAR(64) NOT NULL,
    storage_kind  VARCHAR(16) NOT NULL,
    ciphertext    BYTEA NOT NULL,
    content_bytes BIGINT NOT NULL,
    stored_bytes  INT NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, field_name),
    CONSTRAINT chk_instruction_audit_v2_evidence_field CHECK (
        field_name IN ('instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_evidence_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_evidence_kind CHECK (storage_kind IN ('full', 'sample')),
    CONSTRAINT chk_instruction_audit_v2_evidence_lengths CHECK (
        content_bytes > 0 AND stored_bytes > 0
    )
);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_ai_reviews (
    id                 BIGSERIAL PRIMARY KEY,
    event_id           BIGINT NOT NULL REFERENCES instruction_audit_v2_events(id) ON DELETE CASCADE,
    node_id            BIGINT REFERENCES instruction_audit_v2_ai_nodes(id) ON DELETE SET NULL,
    node_name_snapshot VARCHAR(120) NOT NULL DEFAULT '',
    reviewer_model     VARCHAR(255) NOT NULL DEFAULT '',
    field_name         VARCHAR(16) NOT NULL,
    sha256             CHAR(64) NOT NULL,
    result             VARCHAR(16) NOT NULL,
    confidence         DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason             VARCHAR(1000) NOT NULL DEFAULT '',
    category           VARCHAR(120) NOT NULL DEFAULT '',
    prompt_version     VARCHAR(96) NOT NULL,
    sampled            BOOLEAN NOT NULL DEFAULT FALSE,
    cached             BOOLEAN NOT NULL DEFAULT FALSE,
    latency_ms         INT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_review_field CHECK (
        field_name IN ('instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_review_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_review_result CHECK (
        result IN ('pass', 'reject', 'uncertain', 'error', 'timeout', 'invalid')
    ),
    CONSTRAINT chk_instruction_audit_v2_review_confidence CHECK (confidence BETWEEN 0 AND 1),
    CONSTRAINT chk_instruction_audit_v2_review_latency CHECK (latency_ms >= 0)
);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_raw_access_logs (
    id            BIGSERIAL PRIMARY KEY,
    resource_type VARCHAR(16) NOT NULL,
    resource_id   BIGINT NOT NULL,
    field_name    VARCHAR(16) NOT NULL DEFAULT '',
    action        VARCHAR(16) NOT NULL,
    actor_id      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    request_id    VARCHAR(128) NOT NULL DEFAULT '',
    client_ip     VARCHAR(64) NOT NULL DEFAULT '',
    user_agent    VARCHAR(512) NOT NULL DEFAULT '',
    succeeded     BOOLEAN NOT NULL DEFAULT TRUE,
    error_code    VARCHAR(64) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_access_resource CHECK (
        resource_type IN ('event', 'hash', 'risk_hash', 'review_job')
    ),
    CONSTRAINT chk_instruction_audit_v2_access_field CHECK (
        field_name IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_access_action CHECK (action IN ('reveal', 'copy'))
);

CREATE TABLE IF NOT EXISTS instruction_audit_v2_risk_hashes (
    id                         BIGSERIAL PRIMARY KEY,
    sha256                     CHAR(64) NOT NULL UNIQUE,
    content_vault_id           BIGINT NOT NULL REFERENCES instruction_audit_v2_content_vault(id) ON DELETE RESTRICT,
    observed_field             VARCHAR(16) NOT NULL DEFAULT '',
    status                     VARCHAR(16) NOT NULL DEFAULT 'active',
    source                     VARCHAR(24) NOT NULL,
    source_event_id            BIGINT REFERENCES instruction_audit_v2_events(id) ON DELETE SET NULL,
    source_user_id             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    reviewer_node_id           BIGINT REFERENCES instruction_audit_v2_ai_nodes(id) ON DELETE SET NULL,
    reviewer_model             VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version             VARCHAR(96) NOT NULL DEFAULT '',
    confidence                 DOUBLE PRECISION,
    review_reason              VARCHAR(1000) NOT NULL DEFAULT '',
    review_category            VARCHAR(120) NOT NULL DEFAULT '',
    human_review_status        VARCHAR(24) NOT NULL DEFAULT 'pending',
    reviewed_by                BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at                TIMESTAMPTZ,
    created_by                 BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by                 BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_v2_risk_digest CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_v2_risk_field CHECK (
        observed_field IN ('', 'instructions', 'input1')
    ),
    CONSTRAINT chk_instruction_audit_v2_risk_status CHECK (status IN ('active', 'disabled')),
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

ALTER TABLE instruction_audit_v2_risk_hashes
    ADD COLUMN IF NOT EXISTS source_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS instruction_audit_v2_review_jobs (
    id                         BIGSERIAL PRIMARY KEY,
    sha256                     CHAR(64) NOT NULL UNIQUE,
    content_vault_id           BIGINT NOT NULL REFERENCES instruction_audit_v2_content_vault(id) ON DELETE RESTRICT,
    selected_field             VARCHAR(16) NOT NULL,
    source_event_id            BIGINT REFERENCES instruction_audit_v2_events(id) ON DELETE SET NULL,
    source_user_id             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    status                     VARCHAR(16) NOT NULL DEFAULT 'pending',
    final_result               VARCHAR(16) NOT NULL DEFAULT '',
    pass_votes                 SMALLINT NOT NULL DEFAULT 0,
    reject_votes               SMALLINT NOT NULL DEFAULT 0,
    retry_round                INT NOT NULL DEFAULT 0,
    next_attempt_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner                VARCHAR(128) NOT NULL DEFAULT '',
    lease_expires_at           TIMESTAMPTZ,
    prompt_version             VARCHAR(96) NOT NULL,
    review_criteria            TEXT NOT NULL DEFAULT '',
    config_version             BIGINT NOT NULL,
    observe_only               BOOLEAN NOT NULL DEFAULT FALSE,
    sampled                    BOOLEAN NOT NULL DEFAULT FALSE,
    sample_bytes               INT NOT NULL DEFAULT 0,
    content_bytes              BIGINT NOT NULL,
    last_error                 VARCHAR(500) NOT NULL DEFAULT '',
    completed_at               TIMESTAMPTZ,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
    ADD COLUMN IF NOT EXISTS observe_only BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS source_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS instruction_audit_v2_review_attempts (
    id                 BIGSERIAL PRIMARY KEY,
    job_id             BIGINT NOT NULL REFERENCES instruction_audit_v2_review_jobs(id) ON DELETE CASCADE,
    node_id            BIGINT REFERENCES instruction_audit_v2_ai_nodes(id) ON DELETE SET NULL,
    node_slot          VARCHAR(16) NOT NULL,
    node_name_snapshot VARCHAR(120) NOT NULL DEFAULT '',
    reviewer_model     VARCHAR(255) NOT NULL DEFAULT '',
    attempt_no         INT NOT NULL,
    result             VARCHAR(16) NOT NULL,
    confidence         DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason             VARCHAR(1000) NOT NULL DEFAULT '',
    category           VARCHAR(120) NOT NULL DEFAULT '',
    prompt_version     VARCHAR(96) NOT NULL,
    sampled            BOOLEAN NOT NULL DEFAULT FALSE,
    latency_ms         INT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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

ALTER TABLE instruction_audit_v2_events
    ADD COLUMN IF NOT EXISTS selected_field VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS selected_sha256 CHAR(64),
    ADD COLUMN IF NOT EXISTS review_job_id BIGINT
        REFERENCES instruction_audit_v2_review_jobs(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS content_moderation_cyber_evidence (
    log_id                  BIGINT PRIMARY KEY REFERENCES content_moderation_logs(id) ON DELETE CASCADE,
    request_body_ciphertext TEXT NOT NULL,
    request_body_sha256     CHAR(64) NOT NULL,
    request_body_bytes      BIGINT NOT NULL CHECK (request_body_bytes >= 0),
    encryption_version      VARCHAR(32) NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_content_moderation_cyber_evidence_sha256 CHECK (
        request_body_sha256 ~ '^[0-9a-f]{64}$'
    )
);

-- Prompt Audit metadata used by the Instruction Audit V2 compatibility path.
-- These are widening/additive schema changes only; queued jobs and settings are
-- not rewritten by the bridge.
ALTER TABLE prompt_audit_jobs
    ADD COLUMN IF NOT EXISTS audit_source VARCHAR(64) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS instruction_config_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS client_profile_key VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_profile_name VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trigger_reason VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS model_contract_version INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS effective_response_mode VARCHAR(16) NOT NULL DEFAULT '';

ALTER TABLE prompt_audit_events
    ADD COLUMN IF NOT EXISTS audit_source VARCHAR(64) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS instruction_config_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS client_profile_key VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_profile_name VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trigger_reason VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS model_contract_version INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS effective_response_mode VARCHAR(16) NOT NULL DEFAULT '';

ALTER TABLE prompt_audit_events
    ALTER COLUMN scanner_version TYPE VARCHAR(255);

-- Runtime indexes. All are transactional to keep migration recording atomic.
CREATE INDEX IF NOT EXISTS idx_instruction_audit_hashes_status_validity
    ON instruction_audit_hashes(status, valid_from, valid_until);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_rule_set_hashes_hash
    ON instruction_audit_rule_set_hashes(hash_id, rule_set_id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_rule_set_hashes_validity
    ON instruction_audit_rule_set_hashes(rule_set_id, status, valid_until);
CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_rule_sets_system_key
    ON instruction_audit_rule_sets(system_key)
    WHERE system_managed = TRUE AND system_key <> '';
CREATE INDEX IF NOT EXISTS idx_instruction_audit_bindings_user_model
    ON instruction_audit_bindings(user_id, model) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_instruction_audit_bindings_rule_set
    ON instruction_audit_bindings(rule_set_id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_group_bindings_group
    ON instruction_audit_group_bindings(group_id) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_instruction_audit_group_bindings_rule_set
    ON instruction_audit_group_bindings(rule_set_id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_bindings_client_types
    ON instruction_audit_group_bindings USING GIN (client_types);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_rule_set_users_user
    ON instruction_audit_rule_set_users(user_id, rule_set_id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_created
    ON instruction_audit_events(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_user_created
    ON instruction_audit_events(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_model_created
    ON instruction_audit_events(model, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_group_created
    ON instruction_audit_events(group_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_client_created
    ON instruction_audit_events(client_type, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_request_id
    ON instruction_audit_events(request_id) WHERE request_id <> '';
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_api_key_created
    ON instruction_audit_events(api_key_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_reason_created
    ON instruction_audit_events(reason, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_results_created
    ON instruction_audit_events(instructions_result, input1_result, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_email_created
    ON instruction_audit_events(LOWER(user_email_snapshot), created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_outcome_created
    ON instruction_audit_events(final_outcome, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_final_reason_created
    ON instruction_audit_events(final_reason, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_group_outcome_created
    ON instruction_audit_events(group_id, final_outcome, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_pass_cleanup
    ON instruction_audit_events(id, created_at)
    WHERE final_outcome IN ('hash_pass', 'exception_pass');
CREATE INDEX IF NOT EXISTS idx_instruction_audit_outbox_available
    ON instruction_audit_notification_outbox(status, available_at, id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_outbox_claimed
    ON instruction_audit_notification_outbox(claimed_at) WHERE status = 'processing';
CREATE INDEX IF NOT EXISTS idx_instruction_audit_evidence_expiry
    ON instruction_audit_evidence(expires_at);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_evidence_access_event
    ON instruction_audit_evidence_access_logs(event_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_security_notification_dedup_active
    ON security_notification_outbox(dedup_key)
    WHERE dedup_key IS NOT NULL AND status <> 'suppressed';
CREATE INDEX IF NOT EXISTS idx_security_notification_available
    ON security_notification_outbox(status, available_at, id);
CREATE INDEX IF NOT EXISTS idx_security_notification_claimed
    ON security_notification_outbox(claimed_at) WHERE status = 'processing';
CREATE INDEX IF NOT EXISTS idx_security_notification_source
    ON security_notification_outbox(source_type, source_id, audience);
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
CREATE INDEX IF NOT EXISTS idx_instruction_audit_sensitive_scope_actions
    ON instruction_audit_sensitive_access_logs(resource_id, scope_rule_set_id, created_at DESC)
    WHERE resource_type = 'ai_scope';
CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_sensitive_active_grant
    ON instruction_audit_sensitive_access_grants(subject_user_id)
    WHERE revoked_at IS NULL AND subject_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_instruction_audit_sensitive_grants_history
    ON instruction_audit_sensitive_access_grants(subject_user_id, granted_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_translation_jobs_expiry
    ON instruction_audit_translation_jobs(expires_at, id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_translation_claim
    ON instruction_audit_translation_jobs(next_attempt_at, id)
    WHERE status IN ('pending', 'retry');
CREATE INDEX IF NOT EXISTS idx_instruction_audit_translation_processing
    ON instruction_audit_translation_jobs(processing_started_at, id)
    WHERE status = 'processing';
CREATE INDEX IF NOT EXISTS idx_instruction_audit_translation_authorized_grant
    ON instruction_audit_translation_jobs(authorized_grant_id, status, id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_outcome_hourly_range
    ON instruction_audit_outcome_hourly(bucket_at DESC, final_outcome, group_id, client_type);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_nodes_runtime
    ON instruction_audit_v2_ai_nodes(enabled, priority, id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_v2_node_slot
    ON instruction_audit_v2_ai_nodes(slot) WHERE slot <> '';
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_profiles_runtime
    ON instruction_audit_v2_client_profiles(enabled, priority, id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_v2_scope_profile
    ON instruction_audit_v2_scopes(group_id, client_profile_id)
    WHERE client_profile_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_v2_scope_all_clients
    ON instruction_audit_v2_scopes(group_id)
    WHERE client_profile_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_scopes_runtime
    ON instruction_audit_v2_scopes(group_id, enabled, client_profile_id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_hashes_status_created
    ON instruction_audit_v2_hashes(status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_hashes_global_runtime
    ON instruction_audit_v2_hashes(global_trust, status, sha256);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_hash_scopes_scope
    ON instruction_audit_v2_hash_scopes(scope_id, status, hash_id);
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
    ON instruction_audit_v2_events(request_id) WHERE request_id <> '';
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_evidence_expiry
    ON instruction_audit_v2_event_evidence(expires_at, id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_reviews_event
    ON instruction_audit_v2_ai_reviews(event_id, id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_access_resource
    ON instruction_audit_v2_raw_access_logs(resource_type, resource_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_risk_runtime
    ON instruction_audit_v2_risk_hashes(status, sha256);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_risk_created
    ON instruction_audit_v2_risk_hashes(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_jobs_due
    ON instruction_audit_v2_review_jobs(next_attempt_at, id)
    WHERE status IN ('pending', 'retry', 'processing');
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_jobs_status_created
    ON instruction_audit_v2_review_jobs(status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_v2_attempts_job
    ON instruction_audit_v2_review_attempts(job_id, id);
