ALTER TABLE instruction_audit_rule_sets
    ADD COLUMN IF NOT EXISTS allow_empty_fields BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS instruction_audit_rule_set_users (
    rule_set_id       BIGINT NOT NULL REFERENCES instruction_audit_rule_sets(id) ON DELETE CASCADE,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (rule_set_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_instruction_audit_rule_set_users_user
    ON instruction_audit_rule_set_users(user_id, rule_set_id);
