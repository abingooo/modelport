ALTER TABLE instruction_audit_outcome_hourly
    ADD COLUMN IF NOT EXISTS shard_no BIGINT NOT NULL DEFAULT 0;

UPDATE instruction_audit_outcome_hourly
SET shard_no = -1
WHERE shard_no = 0 AND cardinality(event_times) > 0;

DO $$
DECLARE
    primary_key_definition TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid)
    INTO primary_key_definition
    FROM pg_constraint
    WHERE conrelid = 'instruction_audit_outcome_hourly'::regclass
      AND contype = 'p';

    IF primary_key_definition IS NOT NULL
       AND position('shard_no' IN lower(primary_key_definition)) = 0 THEN
        ALTER TABLE instruction_audit_outcome_hourly
            DROP CONSTRAINT instruction_audit_outcome_hourly_pkey;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'instruction_audit_outcome_hourly'::regclass
          AND contype = 'p'
    ) THEN
        ALTER TABLE instruction_audit_outcome_hourly
            ADD CONSTRAINT instruction_audit_outcome_hourly_pkey PRIMARY KEY (
                bucket_at, user_id, group_id, model, client_type,
                final_outcome, final_reason, shard_no
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_outcome_hourly_shard'
          AND conrelid = 'instruction_audit_outcome_hourly'::regclass
    ) THEN
        ALTER TABLE instruction_audit_outcome_hourly
            ADD CONSTRAINT chk_instruction_audit_outcome_hourly_shard CHECK (shard_no >= -1);
    END IF;
END $$;
