CREATE TABLE IF NOT EXISTS instruction_audit_sensitive_access_grants (
    id                       BIGSERIAL PRIMARY KEY,
    subject_user_id          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    subject_email_snapshot   VARCHAR(255) NOT NULL,
    granted_by               BIGINT REFERENCES users(id) ON DELETE SET NULL,
    grant_source             VARCHAR(32) NOT NULL,
    grant_reason             VARCHAR(255) NOT NULL DEFAULT '',
    granted_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_by               BIGINT REFERENCES users(id) ON DELETE SET NULL,
    revoke_source            VARCHAR(32),
    revoke_reason            VARCHAR(255) NOT NULL DEFAULT '',
    revoked_at               TIMESTAMPTZ,
    CONSTRAINT chk_instruction_audit_sensitive_grant_source CHECK (
        grant_source IN ('setup_bootstrap', 'migration_bootstrap', 'manual', 'emergency_cli')
    ),
    CONSTRAINT chk_instruction_audit_sensitive_revoke_source CHECK (
        revoke_source IS NULL OR revoke_source IN ('manual', 'admin_state_change', 'emergency_cli')
    ),
    CONSTRAINT chk_instruction_audit_sensitive_revoke_state CHECK (
        (revoked_at IS NULL AND revoke_source IS NULL)
        OR (revoked_at IS NOT NULL AND revoke_source IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_instruction_audit_sensitive_active_grant
    ON instruction_audit_sensitive_access_grants(subject_user_id)
    WHERE revoked_at IS NULL AND subject_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_sensitive_grants_history
    ON instruction_audit_sensitive_access_grants(subject_user_id, granted_at DESC, id DESC);

ALTER TABLE instruction_audit_sensitive_access_logs
    ADD COLUMN IF NOT EXISTS grant_id BIGINT REFERENCES instruction_audit_sensitive_access_grants(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS auth_method VARCHAR(24) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS authorization_result VARCHAR(24) NOT NULL DEFAULT 'legacy';

ALTER TABLE instruction_audit_evidence_access_logs
    ADD COLUMN IF NOT EXISTS grant_id BIGINT REFERENCES instruction_audit_sensitive_access_grants(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS auth_method VARCHAR(24) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS authorization_result VARCHAR(24) NOT NULL DEFAULT 'legacy';

ALTER TABLE instruction_audit_translation_jobs
    ADD COLUMN IF NOT EXISTS authorized_grant_id BIGINT
        REFERENCES instruction_audit_sensitive_access_grants(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_translation_authorized_grant
    ON instruction_audit_translation_jobs(authorized_grant_id, status, id);

INSERT INTO instruction_audit_sensitive_access_grants (
    subject_user_id,
    subject_email_snapshot,
    granted_by,
    grant_source,
    grant_reason
)
SELECT
    u.id,
    u.email,
    NULL,
    'migration_bootstrap',
    'Automatic bootstrap for the earliest active administrator during the v0.1.170.13 upgrade'
FROM users u
WHERE u.role = 'admin'
  AND u.status = 'active'
  AND u.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM instruction_audit_sensitive_access_grants)
ORDER BY u.created_at ASC, u.id ASC
LIMIT 1
ON CONFLICT DO NOTHING;
