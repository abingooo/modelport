INSERT INTO settings (key, value)
VALUES ('instruction_audit_enabled', 'false')
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_state (
    id               SMALLINT PRIMARY KEY DEFAULT 1,
    config_version   BIGINT NOT NULL DEFAULT 1,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_state_singleton CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_state_version CHECK (config_version >= 1)
);

INSERT INTO instruction_audit_state (id, config_version)
VALUES (1, 1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS instruction_audit_hashes (
    id                BIGSERIAL PRIMARY KEY,
    digest            CHAR(64) NOT NULL UNIQUE,
    name              VARCHAR(160) NOT NULL,
    note              TEXT NOT NULL DEFAULT '',
    observed_source   VARCHAR(32) NOT NULL DEFAULT '',
    client_name       VARCHAR(120) NOT NULL DEFAULT '',
    client_version    VARCHAR(120) NOT NULL DEFAULT '',
    status            VARCHAR(24) NOT NULL DEFAULT 'candidate',
    valid_from        TIMESTAMPTZ,
    valid_until       TIMESTAMPTZ,
    created_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_hash_digest CHECK (digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_hash_source CHECK (observed_source IN ('', 'instructions', 'input1')),
    CONSTRAINT chk_instruction_audit_hash_status CHECK (status IN ('candidate', 'active', 'disabled', 'expired')),
    CONSTRAINT chk_instruction_audit_hash_validity CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from)
);

CREATE TABLE IF NOT EXISTS instruction_audit_rule_sets (
    id                BIGSERIAL PRIMARY KEY,
    name              VARCHAR(160) NOT NULL UNIQUE,
    description       TEXT NOT NULL DEFAULT '',
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    version           BIGINT NOT NULL DEFAULT 1,
    created_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_rule_set_version CHECK (version >= 1)
);

CREATE TABLE IF NOT EXISTS instruction_audit_rule_set_hashes (
    rule_set_id       BIGINT NOT NULL REFERENCES instruction_audit_rule_sets(id) ON DELETE CASCADE,
    hash_id           BIGINT NOT NULL REFERENCES instruction_audit_hashes(id) ON DELETE CASCADE,
    created_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (rule_set_id, hash_id)
);

CREATE TABLE IF NOT EXISTS instruction_audit_bindings (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    model             VARCHAR(255) NOT NULL,
    rule_set_id       BIGINT NOT NULL REFERENCES instruction_audit_rule_sets(id) ON DELETE CASCADE,
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    created_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_instruction_audit_binding UNIQUE (user_id, model, rule_set_id),
    CONSTRAINT chk_instruction_audit_binding_model CHECK (btrim(model) <> '' AND model = btrim(model))
);

CREATE TABLE IF NOT EXISTS instruction_audit_events (
    id                       BIGSERIAL PRIMARY KEY,
    request_id               VARCHAR(128) NOT NULL DEFAULT '',
    user_id                  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    user_email_snapshot      VARCHAR(320) NOT NULL DEFAULT '',
    api_key_id               BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    model                    VARCHAR(255) NOT NULL DEFAULT '',
    endpoint                 VARCHAR(255) NOT NULL DEFAULT '',
    stage                    VARCHAR(32) NOT NULL DEFAULT 'http',
    instructions_present     BOOLEAN NOT NULL DEFAULT FALSE,
    instructions_sha256      VARCHAR(64) NOT NULL DEFAULT '',
    instructions_result      VARCHAR(24) NOT NULL DEFAULT 'missing',
    input1_present           BOOLEAN NOT NULL DEFAULT FALSE,
    input1_sha256            VARCHAR(64) NOT NULL DEFAULT '',
    input1_result            VARCHAR(24) NOT NULL DEFAULT 'missing',
    decision                 VARCHAR(24) NOT NULL DEFAULT 'blocked',
    reason                   VARCHAR(64) NOT NULL DEFAULT 'hash_mismatch',
    rule_set_ids             JSONB NOT NULL DEFAULT '[]'::jsonb,
    config_version           BIGINT NOT NULL DEFAULT 1,
    latency_ms               INT NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_event_instructions_digest CHECK (instructions_sha256 = '' OR instructions_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_event_input1_digest CHECK (input1_sha256 = '' OR input1_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_event_field_result CHECK (
        instructions_result IN ('missing', 'invalid', 'mismatch', 'match', 'not_checked') AND
        input1_result IN ('missing', 'invalid', 'mismatch', 'match', 'not_checked')
    ),
    CONSTRAINT chk_instruction_audit_event_decision CHECK (decision = 'blocked'),
    CONSTRAINT chk_instruction_audit_event_rule_sets CHECK (jsonb_typeof(rule_set_ids) = 'array'),
    CONSTRAINT chk_instruction_audit_event_values CHECK (config_version >= 1 AND latency_ms >= 0)
);

CREATE TABLE IF NOT EXISTS instruction_audit_notification_outbox (
    id                BIGSERIAL PRIMARY KEY,
    event_id          BIGINT NOT NULL REFERENCES instruction_audit_events(id) ON DELETE CASCADE,
    dedup_key         VARCHAR(64) NOT NULL UNIQUE,
    status            VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempts          INT NOT NULL DEFAULT 0,
    max_attempts      INT NOT NULL DEFAULT 8,
    available_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at        TIMESTAMPTZ,
    sent_recipient_ids BIGINT[] NOT NULL DEFAULT '{}',
    last_error        VARCHAR(512) NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_outbox_status CHECK (status IN ('pending', 'processing', 'retry', 'sent', 'failed')),
    CONSTRAINT chk_instruction_audit_outbox_attempts CHECK (attempts >= 0 AND max_attempts > 0)
);

ALTER TABLE instruction_audit_notification_outbox
    ADD COLUMN IF NOT EXISTS sent_recipient_ids BIGINT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_instruction_audit_hashes_status_validity
    ON instruction_audit_hashes(status, valid_from, valid_until);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_rule_set_hashes_hash
    ON instruction_audit_rule_set_hashes(hash_id, rule_set_id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_bindings_user_model
    ON instruction_audit_bindings(user_id, model) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_instruction_audit_bindings_rule_set
    ON instruction_audit_bindings(rule_set_id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_created
    ON instruction_audit_events(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_user_created
    ON instruction_audit_events(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_model_created
    ON instruction_audit_events(model, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_outbox_available
    ON instruction_audit_notification_outbox(status, available_at, id);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_outbox_claimed
    ON instruction_audit_notification_outbox(claimed_at) WHERE status = 'processing';
