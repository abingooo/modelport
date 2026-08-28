-- ModelPort bridge for both clean Sub2API databases and databases upgraded
-- from custom-v0.1.176.2. The legacy release already has groups.is_free, so
-- every statement must remain idempotent.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS is_free BOOLEAN;

UPDATE groups
SET is_free = FALSE
WHERE is_free IS NULL;

ALTER TABLE groups
    ALTER COLUMN is_free SET DEFAULT FALSE;

ALTER TABLE groups
    ALTER COLUMN is_free SET NOT NULL;

COMMENT ON COLUMN groups.is_free IS
    'Explicit free-billing group: preserve raw usage cost while charging the user zero';

-- Keep the durable API-key auth-cache invalidation backstop in sync with the
-- new policy field. The admin service already invalidates locally; this
-- trigger also covers direct SQL edits and instances that did not receive the
-- application-level invalidation event.
CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_group_id BIGINT;
BEGIN
    target_group_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.is_exclusive IS NOT DISTINCT FROM NEW.is_exclusive
       AND OLD.allow_image_generation IS NOT DISTINCT FROM NEW.allow_image_generation
       AND OLD.platform IS NOT DISTINCT FROM NEW.platform
       AND OLD.subscription_type IS NOT DISTINCT FROM NEW.subscription_type
       AND OLD.rate_multiplier IS NOT DISTINCT FROM NEW.rate_multiplier
       AND OLD.is_free IS NOT DISTINCT FROM NEW.is_free
       AND OLD.peak_rate_enabled IS NOT DISTINCT FROM NEW.peak_rate_enabled
       AND OLD.peak_start IS NOT DISTINCT FROM NEW.peak_start
       AND OLD.peak_end IS NOT DISTINCT FROM NEW.peak_end
       AND OLD.peak_rate_multiplier IS NOT DISTINCT FROM NEW.peak_rate_multiplier
       AND OLD.profit_control_enabled IS NOT DISTINCT FROM NEW.profit_control_enabled
       AND OLD.profit_min_margin IS NOT DISTINCT FROM NEW.profit_min_margin
       AND OLD.profit_safety_buffer IS NOT DISTINCT FROM NEW.profit_safety_buffer
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN
        RETURN NEW;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.group_id = target_group_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

-- Batch image jobs settle asynchronously. Snapshot the free-billing decision
-- made at submission so a later group edit cannot change money semantics for
-- an in-flight job. Old jobs default to their historical paid behavior.
ALTER TABLE IF EXISTS batch_image_jobs
    ADD COLUMN IF NOT EXISTS is_free_billing BOOLEAN;

UPDATE batch_image_jobs
SET is_free_billing = FALSE
WHERE is_free_billing IS NULL;

ALTER TABLE IF EXISTS batch_image_jobs
    ALTER COLUMN is_free_billing SET DEFAULT FALSE;

ALTER TABLE IF EXISTS batch_image_jobs
    ALTER COLUMN is_free_billing SET NOT NULL;

COMMENT ON COLUMN batch_image_jobs.is_free_billing IS
    'Submission-time snapshot of groups.is_free for deterministic settlement';
