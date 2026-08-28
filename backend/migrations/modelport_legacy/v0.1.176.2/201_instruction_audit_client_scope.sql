ALTER TABLE instruction_audit_group_bindings
    ADD COLUMN IF NOT EXISTS client_types TEXT[] NOT NULL DEFAULT ARRAY['all']::TEXT[];

UPDATE instruction_audit_group_bindings
SET client_types = ARRAY['all']::TEXT[]
WHERE client_types IS NULL OR cardinality(client_types) = 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_binding_client_types'
    ) THEN
        ALTER TABLE instruction_audit_group_bindings
            ADD CONSTRAINT chk_instruction_audit_binding_client_types CHECK (
                cardinality(client_types) BETWEEN 1 AND 7
                AND client_types <@ ARRAY[
                    'all', 'codex_vscode', 'codex_cli', 'codex_desktop',
                    'opencode', 'modelport_internal', 'other', 'unknown'
                ]::TEXT[]
                AND (NOT ('all' = ANY(client_types)) OR client_types = ARRAY['all']::TEXT[])
            );
    END IF;
END $$;

ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS client_type VARCHAR(32) NOT NULL DEFAULT 'unknown';
ALTER TABLE instruction_audit_events
    ADD COLUMN IF NOT EXISTS client_user_agent VARCHAR(512) NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_event_client_type'
    ) THEN
        ALTER TABLE instruction_audit_events
            ADD CONSTRAINT chk_instruction_audit_event_client_type CHECK (
                client_type IN (
                    'codex_vscode', 'codex_cli', 'codex_desktop', 'opencode',
                    'modelport_internal', 'other', 'unknown'
                )
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_bindings_client_types
    ON instruction_audit_group_bindings USING GIN (client_types);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_events_client_created
    ON instruction_audit_events(client_type, created_at DESC, id DESC);
