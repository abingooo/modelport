CREATE TABLE IF NOT EXISTS instruction_audit_group_bindings (
    id                BIGSERIAL PRIMARY KEY,
    group_id          BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    rule_set_id       BIGINT NOT NULL REFERENCES instruction_audit_rule_sets(id) ON DELETE CASCADE,
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    created_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_instruction_audit_group_binding UNIQUE (group_id, rule_set_id)
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_group_bindings_group
    ON instruction_audit_group_bindings(group_id) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_instruction_audit_group_bindings_rule_set
    ON instruction_audit_group_bindings(rule_set_id);

UPDATE settings
SET value = 'false', updated_at = NOW()
WHERE key = 'instruction_audit_enabled'
  AND NOT EXISTS (SELECT 1 FROM instruction_audit_group_bindings);

ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL;
ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS group_name_snapshot VARCHAR(100) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_group_created
    ON instruction_audit_events(group_id, created_at DESC, id DESC);
