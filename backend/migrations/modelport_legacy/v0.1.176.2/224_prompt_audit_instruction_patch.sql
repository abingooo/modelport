-- Prompt Audit becomes an Instruction Audit V2 patch for non-Responses
-- requests. Existing prompt endpoints use the retired model contract and are
-- disabled until an administrator explicitly reconfigures and probes them.

ALTER TABLE instruction_audit_v2_client_profiles
    ADD COLUMN IF NOT EXISTS prompt_audit_enabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE instruction_audit_v2_client_profiles
SET prompt_audit_enabled = FALSE
WHERE profile_key = 'modelport_internal'
  AND prompt_audit_enabled;

ALTER TABLE instruction_audit_v2_client_profiles
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_v2_internal_prompt_audit;
ALTER TABLE instruction_audit_v2_client_profiles
    ADD CONSTRAINT chk_instruction_audit_v2_internal_prompt_audit CHECK (
        profile_key <> 'modelport_internal' OR NOT prompt_audit_enabled
    );

ALTER TABLE prompt_audit_jobs
    ADD COLUMN IF NOT EXISTS audit_source VARCHAR(64) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS instruction_config_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS client_profile_key VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_profile_name VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trigger_reason VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS model_contract_version INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS effective_response_mode VARCHAR(16) NOT NULL DEFAULT '';

ALTER TABLE prompt_audit_events
    ADD COLUMN IF NOT EXISTS audit_source VARCHAR(64) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS instruction_config_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS client_profile_key VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_profile_name VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trigger_reason VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS model_contract_version INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS effective_response_mode VARCHAR(16) NOT NULL DEFAULT '';

ALTER TABLE prompt_audit_events
    ALTER COLUMN scanner_version TYPE VARCHAR(255);

UPDATE prompt_audit_jobs
SET status = 'failed',
    processed_at = NOW(),
    processing_started_at = NULL,
    updated_at = NOW(),
    last_error_code = 'model_contract_retired',
    last_error_message = 'legacy prompt audit model contract retired'
WHERE model_contract_version <> 2
  AND status IN ('staging', 'queued', 'retry', 'processing');

ALTER TABLE prompt_audit_jobs
    DROP CONSTRAINT IF EXISTS chk_prompt_audit_jobs_route_metadata;
ALTER TABLE prompt_audit_jobs
    ADD CONSTRAINT chk_prompt_audit_jobs_route_metadata CHECK (
        instruction_config_version >= 0
        AND model_contract_version >= 1
        AND effective_response_mode IN ('', 'json_schema', 'json_object', 'text_json')
    );

ALTER TABLE prompt_audit_events
    DROP CONSTRAINT IF EXISTS chk_prompt_audit_events_route_metadata;
ALTER TABLE prompt_audit_events
    ADD CONSTRAINT chk_prompt_audit_events_route_metadata CHECK (
        instruction_config_version >= 0
        AND model_contract_version >= 1
        AND effective_response_mode IN ('', 'json_schema', 'json_object', 'text_json')
    );

DO $$
DECLARE
    current_config JSONB;
    migrated_endpoints JSONB;
    current_contract INT;
    next_version BIGINT;
BEGIN
    BEGIN
        SELECT value::jsonb
        INTO current_config
        FROM settings
        WHERE key = 'prompt_audit_config'
        FOR UPDATE;
    EXCEPTION WHEN OTHERS THEN
        -- The application parser treats malformed configuration as unavailable.
        -- Do not overwrite an unreadable value because it may be recoverable by
        -- an administrator and can contain encrypted endpoint credentials.
        RETURN;
    END;

    IF current_config IS NULL OR jsonb_typeof(current_config) <> 'object' THEN
        RETURN;
    END IF;

    current_contract := CASE
        WHEN COALESCE(current_config->>'model_contract_version', '') ~ '^[0-9]+$'
            AND length(current_config->>'model_contract_version') <= 9
            THEN (current_config->>'model_contract_version')::INT
        ELSE 0
    END;
    IF current_contract >= 2 THEN
        RETURN;
    END IF;

    SELECT COALESCE(jsonb_agg(
        CASE
            WHEN jsonb_typeof(endpoint.value) = 'object' THEN endpoint.value || jsonb_build_object(
                'enabled', FALSE,
                'requires_reconfigure', TRUE,
                'response_mode', 'auto',
                'max_output_tokens', 256
            )
            ELSE endpoint.value
        END
        ORDER BY endpoint.ordinality
    ), '[]'::jsonb)
    INTO migrated_endpoints
    FROM jsonb_array_elements(
        CASE
            WHEN jsonb_typeof(current_config->'endpoints') = 'array'
                THEN current_config->'endpoints'
            ELSE '[]'::jsonb
        END
    ) WITH ORDINALITY AS endpoint(value, ordinality);

    next_version := CASE
        WHEN COALESCE(current_config->>'config_version', '') ~ '^[0-9]+$'
            AND length(current_config->>'config_version') <= 18
            THEN GREATEST((current_config->>'config_version')::BIGINT, 1) + 1
        ELSE 2
    END;

    current_config := current_config || jsonb_build_object(
        'model_contract_version', 2,
        'enabled', FALSE,
        'blocking_enabled', FALSE,
        'requires_reconfigure', TRUE,
        'endpoints', migrated_endpoints,
        'config_version', next_version,
        'change_summary', 'legacy_model_contract_disabled'
    );

    UPDATE settings
    SET value = current_config::text,
        updated_at = NOW()
    WHERE key = 'prompt_audit_config';
END $$;
