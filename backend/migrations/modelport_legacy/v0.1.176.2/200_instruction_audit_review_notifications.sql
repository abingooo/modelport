INSERT INTO settings (key, value, updated_at)
VALUES ('instruction_audit_evidence_retention_days', '30', NOW())
ON CONFLICT (key) DO NOTHING;

ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS evidence_status VARCHAR(32) NOT NULL DEFAULT 'legacy_unavailable';
ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS evidence_expires_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_event_evidence_status'
    ) THEN
        ALTER TABLE instruction_audit_events
            ADD CONSTRAINT chk_instruction_audit_event_evidence_status CHECK (
                evidence_status IN (
                    'stored', 'not_available', 'encryption_unavailable',
                    'expired', 'legacy_unavailable'
                )
            );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS instruction_audit_evidence (
    event_id          BIGINT NOT NULL REFERENCES instruction_audit_events(id) ON DELETE CASCADE,
    source            VARCHAR(24) NOT NULL,
    digest            CHAR(64) NOT NULL,
    ciphertext        BYTEA NOT NULL,
    key_version       VARCHAR(64) NOT NULL,
    plaintext_bytes   INT NOT NULL,
    expires_at        TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, source),
    CONSTRAINT chk_instruction_audit_evidence_source CHECK (source IN ('instructions', 'input1')),
    CONSTRAINT chk_instruction_audit_evidence_digest CHECK (digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_instruction_audit_evidence_bytes CHECK (plaintext_bytes > 0),
    CONSTRAINT chk_instruction_audit_evidence_ciphertext CHECK (octet_length(ciphertext) > 0)
);

CREATE TABLE IF NOT EXISTS instruction_audit_evidence_access_logs (
    id                BIGSERIAL PRIMARY KEY,
    event_id          BIGINT NOT NULL REFERENCES instruction_audit_events(id) ON DELETE CASCADE,
    actor_id          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action            VARCHAR(16) NOT NULL,
    source            VARCHAR(48) NOT NULL,
    request_id        VARCHAR(128) NOT NULL DEFAULT '',
    client_ip         VARCHAR(64) NOT NULL DEFAULT '',
    user_agent        VARCHAR(512) NOT NULL DEFAULT '',
    succeeded         BOOLEAN NOT NULL DEFAULT TRUE,
    error_code        VARCHAR(64) NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_evidence_access_action CHECK (action IN ('reveal', 'copy')),
    CONSTRAINT chk_instruction_audit_evidence_access_source CHECK (btrim(source) <> '')
);

CREATE TABLE IF NOT EXISTS security_notification_outbox (
    id                     BIGSERIAL PRIMARY KEY,
    source_type            VARCHAR(32) NOT NULL,
    source_id              BIGINT NOT NULL,
    audience               VARCHAR(16) NOT NULL,
    user_id                BIGINT REFERENCES users(id) ON DELETE SET NULL,
    recipients             TEXT[] NOT NULL DEFAULT '{}',
    sent_recipient_hashes  TEXT[] NOT NULL DEFAULT '{}',
    template_event         VARCHAR(96) NOT NULL,
    variables              JSONB NOT NULL DEFAULT '{}'::jsonb,
    dedup_key              VARCHAR(64),
    status                 VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempts               INT NOT NULL DEFAULT 0,
    max_attempts           INT NOT NULL DEFAULT 8,
    available_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at             TIMESTAMPTZ,
    last_error             VARCHAR(512) NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_security_notification_source_audience UNIQUE (source_type, source_id, audience),
    CONSTRAINT chk_security_notification_source CHECK (source_type IN ('instruction_audit', 'cyber_policy')),
    CONSTRAINT chk_security_notification_audience CHECK (audience IN ('user', 'ops')),
    CONSTRAINT chk_security_notification_status CHECK (
        status IN ('pending', 'processing', 'retry', 'sent', 'failed', 'suppressed', 'no_recipient')
    ),
    CONSTRAINT chk_security_notification_attempts CHECK (attempts >= 0 AND max_attempts > 0),
    CONSTRAINT chk_security_notification_variables CHECK (jsonb_typeof(variables) = 'object')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_security_notification_dedup_active
    ON security_notification_outbox(dedup_key)
    WHERE dedup_key IS NOT NULL AND status <> 'suppressed';
CREATE INDEX IF NOT EXISTS idx_security_notification_available
    ON security_notification_outbox(status, available_at, id);
CREATE INDEX IF NOT EXISTS idx_security_notification_claimed
    ON security_notification_outbox(claimed_at) WHERE status = 'processing';
CREATE INDEX IF NOT EXISTS idx_security_notification_source
    ON security_notification_outbox(source_type, source_id, audience);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_evidence_expiry
    ON instruction_audit_evidence(expires_at);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_evidence_access_event
    ON instruction_audit_evidence_access_logs(event_id, created_at DESC, id DESC);
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

-- Preserve the old operations-delivery state without re-sending historical notifications.
INSERT INTO security_notification_outbox (
    source_type, source_id, audience, recipients, template_event, variables,
    dedup_key, status, attempts, max_attempts, available_at, claimed_at,
    last_error, created_at, updated_at
)
SELECT
    'instruction_audit', o.event_id, 'ops', '{}'::TEXT[],
    'instruction_audit.ops_notice', '{}'::JSONB, NULL,
    CASE
        WHEN o.status = 'sent' THEN 'sent'
        WHEN o.status = 'failed' THEN 'failed'
        ELSE 'suppressed'
    END,
    o.attempts, o.max_attempts, o.available_at, NULL,
    o.last_error, o.created_at, o.updated_at
FROM instruction_audit_notification_outbox o
ON CONFLICT (source_type, source_id, audience) DO NOTHING;

INSERT INTO security_notification_outbox (
    source_type, source_id, audience, recipients, template_event, variables,
    status, max_attempts, created_at, updated_at
)
SELECT
    'instruction_audit', e.id, 'user', '{}'::TEXT[],
    'instruction_audit.user_notice', '{}'::JSONB,
    'suppressed', 8, e.created_at, NOW()
FROM instruction_audit_events e
ON CONFLICT (source_type, source_id, audience) DO NOTHING;
