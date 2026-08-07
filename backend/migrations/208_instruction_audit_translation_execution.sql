ALTER TABLE instruction_audit_translation_jobs
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_translation_status;

ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0;
ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS max_attempts INT NOT NULL DEFAULT 3;
ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS claim_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ;
ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS result_bytes INT NOT NULL DEFAULT 0;
ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS redaction_count INT NOT NULL DEFAULT 0;
ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS provider_latency_ms INT NOT NULL DEFAULT 0;

ALTER TABLE instruction_audit_translation_jobs
    ADD CONSTRAINT chk_instruction_audit_translation_status CHECK (
        status IN ('pending', 'processing', 'retry', 'succeeded', 'partial', 'failed', 'expired')
    );

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_translation_execution'
          AND conrelid = 'instruction_audit_translation_jobs'::regclass
    ) THEN
        ALTER TABLE instruction_audit_translation_jobs
            ADD CONSTRAINT chk_instruction_audit_translation_execution CHECK (
                attempts >= 0
                AND max_attempts BETWEEN 1 AND 10
                AND claim_version >= 0
                AND result_bytes >= 0
                AND redaction_count >= 0
                AND provider_latency_ms >= 0
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_translation_claim
    ON instruction_audit_translation_jobs(next_attempt_at, id)
    WHERE status IN ('pending', 'retry');

CREATE INDEX IF NOT EXISTS idx_instruction_audit_translation_processing
    ON instruction_audit_translation_jobs(processing_started_at, id)
    WHERE status = 'processing';
