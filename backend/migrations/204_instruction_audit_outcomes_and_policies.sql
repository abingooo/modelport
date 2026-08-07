ALTER TABLE instruction_audit_events
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_event_decision;

ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS body_bytes BIGINT;
ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS initial_reason VARCHAR(64);
ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS final_reason VARCHAR(64);
ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS final_outcome VARCHAR(32);
ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS policy_action VARCHAR(32);
ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS ai_latency_ms INT;

UPDATE instruction_audit_events
SET initial_reason = COALESCE(initial_reason, reason),
    final_reason = COALESCE(final_reason, reason),
    final_outcome = COALESCE(final_outcome, decision, 'blocked'),
    policy_action = COALESCE(policy_action, 'block')
WHERE initial_reason IS NULL
   OR final_reason IS NULL
   OR final_outcome IS NULL
   OR policy_action IS NULL;

CREATE OR REPLACE FUNCTION instruction_audit_event_v13_compat()
RETURNS TRIGGER AS $$
BEGIN
    NEW.initial_reason := COALESCE(NEW.initial_reason, NEW.reason, 'hash_mismatch');
    NEW.final_reason := COALESCE(NEW.final_reason, NEW.reason, NEW.initial_reason);
    NEW.final_outcome := COALESCE(NEW.final_outcome, NEW.decision, 'blocked');
    NEW.policy_action := COALESCE(
        NEW.policy_action,
        CASE NEW.final_outcome
            WHEN 'policy_allow' THEN 'allow_and_record'
            WHEN 'hash_pass' THEN 'hash_match'
            WHEN 'exception_pass' THEN 'exception'
            WHEN 'ai_pass' THEN 'ai_review'
            ELSE 'block'
        END
    );
    NEW.decision := NEW.final_outcome;
    NEW.reason := NEW.final_reason;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_instruction_audit_event_v13_compat ON instruction_audit_events;
CREATE TRIGGER trg_instruction_audit_event_v13_compat
    BEFORE INSERT OR UPDATE OF decision, reason, initial_reason, final_reason, final_outcome, policy_action
    ON instruction_audit_events
    FOR EACH ROW EXECUTE FUNCTION instruction_audit_event_v13_compat();

ALTER TABLE instruction_audit_events
    ALTER COLUMN initial_reason SET NOT NULL;
ALTER TABLE instruction_audit_events
    ALTER COLUMN final_reason SET NOT NULL;
ALTER TABLE instruction_audit_events
    ALTER COLUMN final_outcome SET NOT NULL;
ALTER TABLE instruction_audit_events
    ALTER COLUMN policy_action SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_event_decision'
          AND conrelid = 'instruction_audit_events'::regclass
    ) THEN
        ALTER TABLE instruction_audit_events
            ADD CONSTRAINT chk_instruction_audit_event_decision CHECK (
                decision IN ('blocked', 'policy_allow', 'ai_pass', 'hash_pass', 'exception_pass')
            );
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_event_final_outcome'
          AND conrelid = 'instruction_audit_events'::regclass
    ) THEN
        ALTER TABLE instruction_audit_events
            ADD CONSTRAINT chk_instruction_audit_event_final_outcome CHECK (
                final_outcome IN ('blocked', 'policy_allow', 'ai_pass', 'hash_pass', 'exception_pass')
                AND decision = final_outcome
            );
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_event_policy_action'
          AND conrelid = 'instruction_audit_events'::regclass
    ) THEN
        ALTER TABLE instruction_audit_events
            ADD CONSTRAINT chk_instruction_audit_event_policy_action CHECK (
                policy_action IN ('block', 'allow_and_record', 'hash_match', 'exception', 'ai_review')
            );
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_event_measurements'
          AND conrelid = 'instruction_audit_events'::regclass
    ) THEN
        ALTER TABLE instruction_audit_events
            ADD CONSTRAINT chk_instruction_audit_event_measurements CHECK (
                (body_bytes IS NULL OR body_bytes >= 0)
                AND (ai_latency_ms IS NULL OR ai_latency_ms >= 0)
            );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS instruction_audit_reason_policies (
    reason              VARCHAR(64) PRIMARY KEY,
    action              VARCHAR(32) NOT NULL DEFAULT 'block',
    ai_review_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    alert_enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    allow_until         TIMESTAMPTZ,
    config_version      BIGINT NOT NULL DEFAULT 1,
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
