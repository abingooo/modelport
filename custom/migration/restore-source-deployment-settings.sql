\set ON_ERROR_STOP on
\set QUIET 1
SET client_min_messages = warning;

BEGIN;

LOCK TABLE public.settings IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF to_regclass('modelport_cutover_guard.deployment_setting_state') IS NULL
       OR to_regclass('modelport_cutover_guard.deployment_sequence_state') IS NULL
       OR to_regclass('modelport_cutover_guard.deployment_sequence_applied_state') IS NULL THEN
        RAISE EXCEPTION 'ModelPort deployment setting backup is missing';
    END IF;
    IF (SELECT count(*) FROM modelport_cutover_guard.deployment_setting_state) <> 6 THEN
        RAISE EXCEPTION 'ModelPort deployment setting backup is incomplete';
    END IF;
END
$$;

DO $$
DECLARE
    current_last_value bigint;
    current_is_called boolean;
    applied_last_value bigint;
    applied_is_called boolean;
BEGIN
    SELECT last_value, is_called
    INTO current_last_value, current_is_called
    FROM public.settings_id_seq;
    SELECT last_value, is_called
    INTO applied_last_value, applied_is_called
    FROM modelport_cutover_guard.deployment_sequence_applied_state
    WHERE sequence_name = 'public.settings_id_seq';
    IF NOT FOUND
       OR current_last_value IS DISTINCT FROM applied_last_value
       OR current_is_called IS DISTINCT FROM applied_is_called THEN
        RAISE EXCEPTION 'settings sequence changed after ModelPort deployment settings were applied';
    END IF;
END
$$;

DELETE FROM public.settings AS current
USING modelport_cutover_guard.deployment_setting_state AS original
WHERE current.key = original.key;

INSERT INTO public.settings (id, key, value, updated_at)
SELECT id, key, value, updated_at
FROM modelport_cutover_guard.deployment_setting_state
WHERE existed;

DO $$
DECLARE
    original_last_value bigint;
    original_is_called boolean;
BEGIN
    SELECT last_value, is_called
    INTO original_last_value, original_is_called
    FROM modelport_cutover_guard.deployment_sequence_state
    WHERE sequence_name = 'public.settings_id_seq';
    IF NOT FOUND THEN
        RAISE EXCEPTION 'settings sequence backup is missing';
    END IF;
    PERFORM setval('public.settings_id_seq', original_last_value, original_is_called);

    IF EXISTS (
        SELECT 1
        FROM modelport_cutover_guard.deployment_setting_state AS original
        LEFT JOIN public.settings AS current USING (key)
        WHERE original.existed
          AND (
              current.id IS DISTINCT FROM original.id
              OR current.value IS DISTINCT FROM original.value
              OR current.updated_at IS DISTINCT FROM original.updated_at
          )
    ) THEN
        RAISE EXCEPTION 'an original deployment setting was not restored';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM modelport_cutover_guard.deployment_setting_state AS original
        JOIN public.settings AS current USING (key)
        WHERE NOT original.existed
    ) THEN
        RAISE EXCEPTION 'a deployment setting created during cutover still exists';
    END IF;
END
$$;

COMMIT;

\pset tuples_only on
\pset format unaligned
\pset fieldsep '\t'
SELECT 'source_deployment_settings_restore', 'PASS';
