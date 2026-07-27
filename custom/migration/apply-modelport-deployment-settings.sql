\set ON_ERROR_STOP on
\set QUIET 1
SET client_min_messages = warning;

BEGIN;

LOCK TABLE public.settings IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF to_regnamespace('modelport_cutover_guard') IS NULL THEN
        RAISE EXCEPTION 'modelport_cutover_guard is missing; run enter-cutover-fence.sql first';
    END IF;
END
$$;

CREATE TEMP TABLE modelport_deployment_input (
    public_url text PRIMARY KEY
) ON COMMIT DROP;

INSERT INTO modelport_deployment_input (public_url)
VALUES (regexp_replace(trim(:'modelport_public_url'), '/+$', ''));

DO $$
DECLARE
    configured_url text;
    configured_port integer;
BEGIN
    SELECT public_url INTO configured_url FROM modelport_deployment_input;
    IF configured_url !~ '^https://[A-Za-z0-9.-]+(:[0-9]{1,5})?$'
       OR configured_url LIKE '%..%'
       OR length(configured_url) > 255 THEN
        RAISE EXCEPTION 'modelport_public_url must be an HTTPS origin without a path: %', configured_url;
    END IF;
    IF configured_url ~ ':[0-9]+$' THEN
        configured_port := substring(configured_url FROM ':([0-9]+)$')::integer;
        IF configured_port < 1 OR configured_port > 65535 THEN
            RAISE EXCEPTION 'modelport_public_url has an invalid port: %', configured_url;
        END IF;
    END IF;
END
$$;

CREATE TEMP TABLE modelport_target_settings (
    key varchar(100) PRIMARY KEY,
    value text NOT NULL
) ON COMMIT DROP;

INSERT INTO modelport_target_settings (key, value)
SELECT 'api_base_url', public_url FROM modelport_deployment_input
UNION ALL
SELECT 'balance_low_notify_recharge_url', public_url FROM modelport_deployment_input
UNION ALL
VALUES
    ('custom_menu_items', '[]'),
    ('site_logo', ''),
    ('site_name', 'ModelPort'),
    ('site_subtitle', 'One port, All Models.');

CREATE TABLE IF NOT EXISTS modelport_cutover_guard.deployment_setting_state (
    key varchar(100) PRIMARY KEY,
    existed boolean NOT NULL,
    id bigint,
    value text,
    updated_at timestamptz,
    CONSTRAINT deployment_setting_state_original_row_check CHECK (
        (existed AND id IS NOT NULL AND value IS NOT NULL AND updated_at IS NOT NULL)
        OR (NOT existed AND id IS NULL AND value IS NULL AND updated_at IS NULL)
    )
);

CREATE TABLE IF NOT EXISTS modelport_cutover_guard.deployment_sequence_state (
    sequence_name text PRIMARY KEY,
    last_value bigint NOT NULL,
    is_called boolean NOT NULL
);

CREATE TABLE IF NOT EXISTS modelport_cutover_guard.deployment_sequence_applied_state (
    sequence_name text PRIMARY KEY,
    last_value bigint NOT NULL,
    is_called boolean NOT NULL
);

INSERT INTO modelport_cutover_guard.deployment_sequence_state
    (sequence_name, last_value, is_called)
SELECT 'public.settings_id_seq', last_value, is_called
FROM public.settings_id_seq
ON CONFLICT (sequence_name) DO NOTHING;

INSERT INTO modelport_cutover_guard.deployment_setting_state
    (key, existed, id, value, updated_at)
SELECT target.key,
       current.key IS NOT NULL,
       current.id,
       current.value,
       current.updated_at
FROM modelport_target_settings AS target
LEFT JOIN public.settings AS current USING (key)
ON CONFLICT (key) DO NOTHING;

UPDATE public.settings AS current
SET value = target.value,
    updated_at = now()
FROM modelport_target_settings AS target
WHERE current.key = target.key
  AND current.value IS DISTINCT FROM target.value;

INSERT INTO public.settings (key, value, updated_at)
SELECT target.key, target.value, now()
FROM modelport_target_settings AS target
WHERE NOT EXISTS (
    SELECT 1
    FROM public.settings AS current
    WHERE current.key = target.key
);

INSERT INTO modelport_cutover_guard.deployment_sequence_applied_state
    (sequence_name, last_value, is_called)
SELECT 'public.settings_id_seq', last_value, is_called
FROM public.settings_id_seq
ON CONFLICT (sequence_name) DO NOTHING;

DO $$
BEGIN
    IF (SELECT count(*) FROM modelport_cutover_guard.deployment_setting_state) <> 6 THEN
        RAISE EXCEPTION 'deployment setting backup is incomplete';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM modelport_cutover_guard.deployment_sequence_applied_state
        WHERE sequence_name = 'public.settings_id_seq'
    ) THEN
        RAISE EXCEPTION 'applied settings sequence state is missing';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM modelport_target_settings AS target
        LEFT JOIN public.settings AS current USING (key)
        WHERE current.value IS DISTINCT FROM target.value
    ) THEN
        RAISE EXCEPTION 'a ModelPort deployment setting was not applied';
    END IF;
    IF (
        SELECT value::jsonb
        FROM public.settings
        WHERE key = 'custom_menu_items'
    ) <> '[]'::jsonb THEN
        RAISE EXCEPTION 'legacy custom menu items remain active';
    END IF;
END
$$;

COMMIT;

\pset tuples_only on
\pset format unaligned
\pset fieldsep '\t'
SELECT 'modelport_deployment_settings', 'PASS';
SELECT key, value
FROM public.settings
WHERE key IN (
    'api_base_url',
    'balance_low_notify_recharge_url',
    'custom_menu_items',
    'site_logo',
    'site_name',
    'site_subtitle'
)
ORDER BY key;
