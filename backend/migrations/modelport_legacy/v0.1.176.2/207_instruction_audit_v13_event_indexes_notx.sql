CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_instruction_audit_events_outcome_created
    ON instruction_audit_events(final_outcome, created_at DESC, id DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_instruction_audit_events_final_reason_created
    ON instruction_audit_events(final_reason, created_at DESC, id DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_instruction_audit_events_group_outcome_created
    ON instruction_audit_events(group_id, final_outcome, created_at DESC, id DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_instruction_audit_events_pass_cleanup
    ON instruction_audit_events(id, created_at)
    WHERE final_outcome IN ('hash_pass', 'exception_pass');
