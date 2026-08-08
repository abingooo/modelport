UPDATE instruction_audit_runtime_config
SET max_inflight_body_bytes = GREATEST(max_inflight_body_bytes, max_body_bytes * 3),
    updated_at = NOW()
WHERE max_inflight_body_bytes < max_body_bytes * 3;

ALTER TABLE instruction_audit_runtime_config
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_runtime_inflight_limit;
ALTER TABLE instruction_audit_runtime_config
    ADD CONSTRAINT chk_instruction_audit_runtime_inflight_limit CHECK (
        max_inflight_body_bytes BETWEEN max_body_bytes * 3 AND 2147483648
    );
