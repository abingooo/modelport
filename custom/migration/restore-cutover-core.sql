\set ON_ERROR_STOP on
\set QUIET 1

BEGIN;

UPDATE public.settings AS setting
SET value = original.value,
    updated_at = original.updated_at
FROM modelport_cutover_guard.setting_state AS original
WHERE setting.key = original.key
  AND original.key IN (
      'account_quota_notify_enabled',
      'balance_low_notify_enabled',
      'payment_enabled',
      'smtp_host',
      'subscription_expiry_notify_enabled'
  );

INSERT INTO modelport_cutover_guard.metadata (key, value) VALUES
    ('core_settings_restored_at_utc', to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'))
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM modelport_cutover_guard.setting_state AS original
        LEFT JOIN public.settings AS setting USING (key)
        WHERE original.key IN (
            'account_quota_notify_enabled',
            'balance_low_notify_enabled',
            'payment_enabled',
            'smtp_host',
            'subscription_expiry_notify_enabled'
        )
          AND (setting.key IS NULL OR setting.value IS DISTINCT FROM original.value)
    ) THEN
        RAISE EXCEPTION 'a core setting was not restored exactly';
    END IF;
END
$$;

COMMIT;

\pset tuples_only on
\pset format unaligned
\pset fieldsep '\t'
SELECT 'cutover_core_settings_restore', 'PASS';
