CREATE TABLE IF NOT EXISTS instruction_audit_operational_counters (
    id                      SMALLINT PRIMARY KEY DEFAULT 1,
    persist_failure_count   BIGINT NOT NULL DEFAULT 0,
    statistics_loss_count   BIGINT NOT NULL DEFAULT 0,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_operational_counter_id CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_operational_counter_values CHECK (
        persist_failure_count >= 0 AND statistics_loss_count >= 0
    )
);

INSERT INTO instruction_audit_operational_counters (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

UPDATE instruction_audit_hash_raw_contents
SET raw_content_status = 'raw_content_unavailable',
    ciphertext = NULL,
    encryption_key_version = '',
    updated_at = NOW()
WHERE raw_content_status = 'expired';

ALTER TABLE instruction_audit_hash_raw_contents
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_hash_raw_status;
ALTER TABLE instruction_audit_hash_raw_contents
    ADD CONSTRAINT chk_instruction_audit_hash_raw_status CHECK (
        raw_content_status IN (
            'stored', 'raw_content_unavailable', 'encryption_unavailable'
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
    );
