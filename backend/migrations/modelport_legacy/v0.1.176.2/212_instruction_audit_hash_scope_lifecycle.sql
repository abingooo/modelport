ALTER TABLE instruction_audit_rule_set_hashes
    ADD COLUMN IF NOT EXISTS source_type VARCHAR(24) NOT NULL DEFAULT 'manual';
ALTER TABLE instruction_audit_rule_set_hashes
    ADD COLUMN IF NOT EXISTS valid_until TIMESTAMPTZ;
ALTER TABLE instruction_audit_rule_set_hashes
    ADD COLUMN IF NOT EXISTS status VARCHAR(24) NOT NULL DEFAULT 'active';
ALTER TABLE instruction_audit_rule_set_hashes
    ADD COLUMN IF NOT EXISTS updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE instruction_audit_rule_set_hashes
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Before this migration, AI expiry lived on the hash identity. Reconstruct the
-- exact system-managed grant from its AI review, unless an administrator had
-- already promoted the legacy hash globally.
WITH legacy_ai_scopes AS (
    SELECT
        rsh.rule_set_id,
        rsh.hash_id,
        COALESCE(
            h.valid_until,
            MAX(ar.created_at) + INTERVAL '24 hours'
        ) AS inferred_valid_until,
        EXISTS (
            SELECT 1
            FROM instruction_audit_sensitive_access_logs sal
            WHERE sal.resource_type = 'ai_hash'
              AND sal.resource_id = rsh.hash_id
              AND sal.action = 'promote'
              AND sal.succeeded = TRUE
        ) AS globally_promoted
    FROM instruction_audit_rule_set_hashes rsh
    JOIN instruction_audit_rule_sets rs ON rs.id = rsh.rule_set_id
    JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
    JOIN instruction_audit_ai_reviews ar
      ON ar.automatic_hash_id = rsh.hash_id
     AND rs.system_key = 'ai:' || ar.group_id::TEXT || ':' || LOWER(ar.client_type)
    WHERE rs.system_managed = TRUE
      AND rsh.updated_by IS NULL
    GROUP BY rsh.rule_set_id, rsh.hash_id, h.valid_until
)
UPDATE instruction_audit_rule_set_hashes rsh
SET source_type = CASE WHEN legacy.globally_promoted THEN 'manual' ELSE 'ai_review' END,
    valid_until = CASE WHEN legacy.globally_promoted THEN NULL ELSE legacy.inferred_valid_until END
FROM legacy_ai_scopes legacy
WHERE rsh.rule_set_id = legacy.rule_set_id
  AND rsh.hash_id = legacy.hash_id
  AND rsh.updated_by IS NULL;

UPDATE instruction_audit_rule_set_hashes rsh
SET source_type = 'manual', valid_until = NULL
FROM instruction_audit_rule_sets rs
WHERE rsh.rule_set_id = rs.id
  AND (rs.system_managed = FALSE OR NOT EXISTS (
      SELECT 1
      FROM instruction_audit_ai_reviews ar
      WHERE ar.automatic_hash_id = rsh.hash_id
        AND rs.system_key = 'ai:' || ar.group_id::TEXT || ':' || LOWER(ar.client_type)
  ))
  AND rsh.updated_by IS NULL;

-- A pure AI hash now has no global expiry. Its independently manageable exact
-- scope owns the temporary lifetime. Manual/import identities keep their
-- existing global validity unchanged.
UPDATE instruction_audit_hashes h
SET valid_until = NULL, updated_at = NOW()
WHERE h.valid_until IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM instruction_audit_hash_sources hs
      WHERE hs.hash_id = h.id AND hs.source_type = 'ai_review'
  )
  AND NOT EXISTS (
      SELECT 1 FROM instruction_audit_hash_sources hs
      WHERE hs.hash_id = h.id AND hs.source_type IN ('manual', 'import')
  );

ALTER TABLE instruction_audit_rule_set_hashes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_rule_set_hash_source;
ALTER TABLE instruction_audit_rule_set_hashes
    ADD CONSTRAINT chk_instruction_audit_rule_set_hash_source CHECK (
        source_type IN ('manual', 'ai_review')
        AND (source_type <> 'ai_review' OR valid_until IS NOT NULL)
    );

ALTER TABLE instruction_audit_rule_set_hashes
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_rule_set_hash_status;
ALTER TABLE instruction_audit_rule_set_hashes
    ADD CONSTRAINT chk_instruction_audit_rule_set_hash_status CHECK (
        status IN ('active', 'disabled', 'revoked')
    );

ALTER TABLE instruction_audit_sensitive_access_logs
    ADD COLUMN IF NOT EXISTS scope_rule_set_id BIGINT REFERENCES instruction_audit_rule_sets(id) ON DELETE SET NULL;
ALTER TABLE instruction_audit_sensitive_access_logs
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_sensitive_resource;
ALTER TABLE instruction_audit_sensitive_access_logs
    ADD CONSTRAINT chk_instruction_audit_sensitive_resource CHECK (
        resource_type IN ('event_evidence', 'hash_raw', 'translation', 'ai_hash', 'ai_scope')
    );

CREATE INDEX IF NOT EXISTS idx_instruction_audit_rule_set_hashes_validity
    ON instruction_audit_rule_set_hashes(rule_set_id, status, valid_until);
CREATE INDEX IF NOT EXISTS idx_instruction_audit_sensitive_scope_actions
    ON instruction_audit_sensitive_access_logs(resource_id, scope_rule_set_id, created_at DESC)
    WHERE resource_type = 'ai_scope';
