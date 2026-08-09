CREATE TABLE IF NOT EXISTS content_moderation_cyber_evidence (
    log_id                  BIGINT PRIMARY KEY REFERENCES content_moderation_logs(id) ON DELETE CASCADE,
    request_body_ciphertext TEXT NOT NULL,
    request_body_sha256     CHAR(64) NOT NULL,
    request_body_bytes      BIGINT NOT NULL CHECK (request_body_bytes >= 0),
    encryption_version      VARCHAR(32) NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_content_moderation_cyber_evidence_sha256
        CHECK (request_body_sha256 ~ '^[0-9a-f]{64}$')
);

