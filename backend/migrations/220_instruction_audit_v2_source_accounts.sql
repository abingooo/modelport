ALTER TABLE instruction_audit_v2_hashes
    ADD COLUMN IF NOT EXISTS source_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE instruction_audit_v2_risk_hashes
    ADD COLUMN IF NOT EXISTS source_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE instruction_audit_v2_review_jobs
    ADD COLUMN IF NOT EXISTS source_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '';

UPDATE instruction_audit_v2_hashes hash
SET source_user_id = event.user_id,
    source_user_email_snapshot = event.user_email_snapshot
FROM instruction_audit_v2_events event
WHERE hash.source_event_id = event.id
  AND hash.source_user_id IS NULL
  AND hash.source_user_email_snapshot = '';

UPDATE instruction_audit_v2_risk_hashes risk
SET source_user_id = event.user_id,
    source_user_email_snapshot = event.user_email_snapshot
FROM instruction_audit_v2_events event
WHERE risk.source_event_id = event.id
  AND risk.source_user_id IS NULL
  AND risk.source_user_email_snapshot = '';

UPDATE instruction_audit_v2_review_jobs job
SET source_user_id = event.user_id,
    source_user_email_snapshot = event.user_email_snapshot
FROM instruction_audit_v2_events event
WHERE job.source_event_id = event.id
  AND job.source_user_id IS NULL
  AND job.source_user_email_snapshot = '';
