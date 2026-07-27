\set ON_ERROR_STOP on
\set QUIET 1

BEGIN;

CREATE SCHEMA modelport_cutover_guard;

CREATE TABLE modelport_cutover_guard.metadata (
    key text PRIMARY KEY,
    value text NOT NULL
);

INSERT INTO modelport_cutover_guard.metadata (key, value) VALUES
    ('purpose', 'Reversible outbound fence for the first ModelPort production startup'),
    ('created_at_utc', to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'));

CREATE TABLE modelport_cutover_guard.setting_state (
    key varchar PRIMARY KEY,
    value text NOT NULL,
    updated_at timestamptz NOT NULL
);

INSERT INTO modelport_cutover_guard.setting_state (key, value, updated_at)
SELECT key, value, updated_at
FROM public.settings
WHERE key IN (
    'account_quota_notify_enabled',
    'backup_schedule',
    'balance_low_notify_enabled',
    'channel_monitor_enabled',
    'content_moderation_config',
    'ollama_cloud_usage_settings',
    'ops_email_notification_config',
    'payment_enabled',
    'risk_control_enabled',
    'smtp_host',
    'subscription_expiry_notify_enabled',
    'upstream_billing_probe_settings'
);

CREATE TABLE modelport_cutover_guard.channel_monitor_state (
    monitor_id bigint PRIMARY KEY,
    enabled boolean NOT NULL,
    updated_at timestamptz NOT NULL
);

INSERT INTO modelport_cutover_guard.channel_monitor_state (monitor_id, enabled, updated_at)
SELECT id, enabled, updated_at
FROM public.channel_monitors;

CREATE TABLE modelport_cutover_guard.scheduled_test_plan_state (
    plan_id bigint PRIMARY KEY,
    enabled boolean NOT NULL,
    updated_at timestamptz NOT NULL
);

INSERT INTO modelport_cutover_guard.scheduled_test_plan_state (plan_id, enabled, updated_at)
SELECT id, enabled, updated_at
FROM public.scheduled_test_plans;

UPDATE public.channel_monitors
SET enabled = false
WHERE enabled;

UPDATE public.scheduled_test_plans
SET enabled = false
WHERE enabled;

UPDATE public.settings
SET value = 'false', updated_at = now()
WHERE key IN (
    'account_quota_notify_enabled',
    'balance_low_notify_enabled',
    'channel_monitor_enabled',
    'payment_enabled',
    'risk_control_enabled',
    'subscription_expiry_notify_enabled'
);

UPDATE public.settings
SET value = '', updated_at = now()
WHERE key = 'smtp_host';

UPDATE public.settings
SET value = jsonb_set(value::jsonb, '{enabled}', 'false'::jsonb, true)::text,
    updated_at = now()
WHERE key IN (
    'backup_schedule',
    'content_moderation_config',
    'ollama_cloud_usage_settings',
    'upstream_billing_probe_settings'
);

UPDATE public.settings
SET value = jsonb_set(
        jsonb_set(value::jsonb, '{alert,enabled}', 'false'::jsonb, true),
        '{report,enabled}', 'false'::jsonb, true
    )::text,
    updated_at = now()
WHERE key = 'ops_email_notification_config';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.channel_monitors WHERE enabled) THEN
        RAISE EXCEPTION 'a channel monitor remains enabled';
    END IF;
    IF EXISTS (SELECT 1 FROM public.scheduled_test_plans WHERE enabled) THEN
        RAISE EXCEPTION 'a scheduled account test remains enabled';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM public.settings
        WHERE key IN (
            'account_quota_notify_enabled',
            'balance_low_notify_enabled',
            'channel_monitor_enabled',
            'payment_enabled',
            'risk_control_enabled',
            'subscription_expiry_notify_enabled'
        )
          AND value <> 'false'
    ) THEN
        RAISE EXCEPTION 'a boolean outbound setting remains enabled';
    END IF;
    IF EXISTS (SELECT 1 FROM public.settings WHERE key = 'smtp_host' AND value <> '') THEN
        RAISE EXCEPTION 'SMTP remains configured';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM public.settings
        WHERE key IN (
            'backup_schedule',
            'content_moderation_config',
            'ollama_cloud_usage_settings',
            'upstream_billing_probe_settings'
        )
          AND COALESCE((value::jsonb->>'enabled')::boolean, false)
    ) THEN
        RAISE EXCEPTION 'a JSON outbound setting remains enabled';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM public.settings
        WHERE key = 'ops_email_notification_config'
          AND (
              COALESCE((value::jsonb#>>'{alert,enabled}')::boolean, false)
              OR COALESCE((value::jsonb#>>'{report,enabled}')::boolean, false)
          )
    ) THEN
        RAISE EXCEPTION 'an ops email mode remains enabled';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM public.payment_orders
        WHERE status = 'PENDING'
    ) THEN
        RAISE EXCEPTION 'pending payment orders must be resolved before cutover';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM public.batch_image_jobs
        WHERE status IN ('queued', 'submitted', 'running', 'processing')
    ) THEN
        RAISE EXCEPTION 'runnable batch image jobs must be resolved before cutover';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM public.prompt_audit_jobs
        WHERE status IN ('queued', 'pending', 'retrying', 'processing')
    ) THEN
        RAISE EXCEPTION 'runnable prompt audit jobs must be resolved before cutover';
    END IF;
END
$$;

COMMIT;

\pset tuples_only on
\pset format unaligned
\pset fieldsep '\t'
SELECT 'cutover_outbound_fence', 'PASS';
