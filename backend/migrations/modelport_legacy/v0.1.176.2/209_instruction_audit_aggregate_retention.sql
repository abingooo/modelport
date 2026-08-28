ALTER TABLE instruction_audit_runtime_config
    ADD COLUMN IF NOT EXISTS aggregate_retention_days INT NOT NULL DEFAULT 365;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_runtime_aggregate_retention'
          AND conrelid = 'instruction_audit_runtime_config'::regclass
    ) THEN
        ALTER TABLE instruction_audit_runtime_config
            ADD CONSTRAINT chk_instruction_audit_runtime_aggregate_retention CHECK (
                aggregate_retention_days BETWEEN 30 AND 3650
            );
    END IF;
END $$;

ALTER TABLE instruction_audit_outcome_rollup_state
    ADD COLUMN IF NOT EXISTS expired_aggregate_event_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE instruction_audit_outcome_rollup_state
    ADD COLUMN IF NOT EXISTS last_aggregate_pruned_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_rollup_expired_count'
          AND conrelid = 'instruction_audit_outcome_rollup_state'::regclass
    ) THEN
        ALTER TABLE instruction_audit_outcome_rollup_state
            ADD CONSTRAINT chk_instruction_audit_rollup_expired_count CHECK (
                expired_aggregate_event_count >= 0
            );
    END IF;
END $$;
