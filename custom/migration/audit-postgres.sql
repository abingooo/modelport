\set ON_ERROR_STOP on
\set QUIET 1
\pset tuples_only on
\pset format unaligned
\pset fieldsep '\t'

SET client_min_messages = warning;
SET timezone = 'UTC';

SELECT 'category', 'name', 'value_1', 'value_2', 'value_3';

SELECT 'core', 'users', count(*)::text,
       count(*) FILTER (WHERE deleted_at IS NULL)::text,
       count(*) FILTER (WHERE password_hash <> '')::text
FROM public.users;
SELECT 'core', 'user_balance', COALESCE(sum(balance), 0)::text,
       COALESCE(sum(frozen_balance), 0)::text,
       COALESCE(sum(total_recharged), 0)::text
FROM public.users;
SELECT 'core', 'api_keys', count(*)::text,
       count(*) FILTER (WHERE deleted_at IS NULL)::text,
       COALESCE(sum(quota_used), 0)::text
FROM public.api_keys;
SELECT 'core', 'accounts', count(*)::text,
       count(*) FILTER (WHERE deleted_at IS NULL)::text,
       count(*) FILTER (WHERE credentials IS NOT NULL)::text
FROM public.accounts;
SELECT 'core', 'subscriptions', count(*)::text,
       count(*) FILTER (WHERE deleted_at IS NULL)::text,
       count(*) FILTER (WHERE deleted_at IS NULL AND status = 'active')::text
FROM public.user_subscriptions;
SELECT 'core', 'usage', count(*)::text,
       COALESCE(sum(total_cost), 0)::text,
       COALESCE(sum(actual_cost), 0)::text
FROM public.usage_logs;
SELECT 'core', 'redeem_codes', count(*)::text,
       count(*) FILTER (WHERE status = 'unused')::text,
       COALESCE(sum(value), 0)::text
FROM public.redeem_codes;
SELECT 'core', 'payment_orders', count(*)::text,
       count(*) FILTER (WHERE status = 'PENDING')::text,
       COALESCE(sum(pay_amount), 0)::text
FROM public.payment_orders;
SELECT 'core', 'auth_identities', count(*)::text,
       count(*)::text,
       count(*) FILTER (WHERE verified_at IS NOT NULL)::text
FROM public.auth_identities;

WITH normalized_tables AS (
    SELECT n.nspname AS schema_name,
           c.relname AS table_name,
           CASE
               WHEN n.nspname = 'public' AND c.relname = 'groups'
                   THEN '(to_jsonb(row_data) - ''is_free'')'
               WHEN n.nspname = 'public' AND c.relname = 'channel_model_pricing'
                   THEN '(to_jsonb(row_data) - ''user_visible'')'
               WHEN n.nspname = 'public' AND c.relname = 'usage_logs'
                   THEN '(to_jsonb(row_data) - ''billing_model'')'
               ELSE 'to_jsonb(row_data)'
           END AS row_expression,
           CASE
               WHEN n.nspname = 'public' AND c.relname = 'schema_migrations' THEN
                   ' WHERE filename NOT IN (' ||
                   '''187_add_deepseek_platform.sql'',' ||
                   '''188_add_openai_compatible_providers.sql'',' ||
                   '''189_add_usage_log_billing_model.sql'',' ||
                   '''190_create_model_catalog_metadata.sql'',' ||
                   '''191_create_lottery_system.sql'',' ||
                   '''191_passkey_credentials.sql'',' ||
                   '''192_add_free_group_billing.sql'',' ||
                   '''193_add_channel_pricing_user_visibility.sql'',' ||
                   '''194_remove_image_site_setting.sql'',' ||
                   '''195_affiliate_reward_review_program.sql'')'
               WHEN n.nspname = 'public' AND c.relname = 'settings'
                   THEN ' WHERE key NOT IN (''image_site_url'', ''affiliate_reward_program_config'', ''passkey_enabled'', ''model_plaza_enabled'', ''model_plaza_require_auth'', ''model_plaza_description'')'
               ELSE ''
           END AS row_filter
    FROM pg_class AS c
    JOIN pg_namespace AS n ON n.oid = c.relnamespace
    WHERE c.relkind IN ('r', 'p')
      AND n.nspname NOT IN ('pg_catalog', 'information_schema')
      AND NOT (
          n.nspname = 'public'
          AND c.relname IN (
              'model_catalog_metadata',
              'lottery_campaigns',
              'lottery_prizes',
              'lottery_entries',
              'lottery_draw_runs',
              'lottery_events',
              'passkey_user_handles',
              'passkey_credentials'
          )
      )
)
SELECT format(
    'SELECT %L, %L, count(*)::text, ' ||
    'COALESCE(sum(hashtextextended((%s)::text, 0)::numeric), 0)::text, ' ||
    'COALESCE(sum(hashtextextended((%s)::text, 1)::numeric), 0)::text ' ||
    'FROM %I.%I AS row_data%s;',
    'table_data',
    format('%I.%I', schema_name, table_name),
    row_expression,
    row_expression,
    schema_name,
    table_name,
    row_filter
)
FROM normalized_tables
ORDER BY schema_name, table_name
\gexec

SELECT format(
    'SELECT %L, %L, last_value::text, is_called::text, '''' ' ||
    'FROM %I.%I;',
    'sequence',
    format('%I.%I', sequence_schema, sequence_name),
    sequence_schema,
    sequence_name
)
FROM information_schema.sequences
WHERE sequence_schema NOT IN ('pg_catalog', 'information_schema')
  AND NOT (
      sequence_schema = 'public'
          AND sequence_name IN (
              'settings_id_seq',
              'model_catalog_metadata_id_seq',
              'lottery_campaigns_id_seq',
          'lottery_prizes_id_seq',
          'lottery_entries_id_seq',
          'lottery_draw_runs_id_seq',
          'lottery_events_id_seq',
          'passkey_credentials_id_seq'
      )
  )
ORDER BY sequence_schema, sequence_name
\gexec

WITH column_state AS (
    SELECT format(
        '%I.%I.%I|%s|%s|%s|%s',
        table_schema,
        table_name,
        column_name,
        data_type,
        udt_name,
        is_nullable,
        COALESCE(column_default, '')
    ) AS state
    FROM information_schema.columns
    WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
      AND NOT (
          table_schema = 'public'
          AND table_name IN (
              'model_catalog_metadata',
              'lottery_campaigns',
              'lottery_prizes',
              'lottery_entries',
              'lottery_draw_runs',
              'lottery_events',
              'passkey_user_handles',
              'passkey_credentials'
          )
      )
      AND NOT (
          table_schema = 'public'
          AND (
              (table_name = 'groups' AND column_name = 'is_free')
              OR (table_name = 'channel_model_pricing' AND column_name = 'user_visible')
              OR (table_name = 'usage_logs' AND column_name = 'billing_model')
          )
      )
)
SELECT 'schema_object', 'columns', count(*)::text,
       COALESCE(sum(hashtextextended(state, 0)::numeric), 0)::text,
       COALESCE(sum(hashtextextended(state, 1)::numeric), 0)::text
FROM column_state;

WITH constraint_state AS (
    SELECT format(
        '%I.%I|%s|%s|%s|%s',
        n.nspname,
        c.relname,
        con.conname,
        con.contype,
        con.convalidated,
        pg_get_constraintdef(con.oid, true)
    ) AS state
    FROM pg_constraint AS con
    JOIN pg_class AS c ON c.oid = con.conrelid
    JOIN pg_namespace AS n ON n.oid = c.relnamespace
    WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
      AND con.conname NOT IN (
          'user_platform_quotas_platform_check',
          'composite_model_routes_target_platform_check',
          'groups_is_free_not_null',
          'channel_model_pricing_user_visible_not_null'
      )
      AND NOT (
          n.nspname = 'public'
          AND c.relname IN (
              'model_catalog_metadata',
              'lottery_campaigns',
              'lottery_prizes',
              'lottery_entries',
              'lottery_draw_runs',
              'lottery_events',
              'passkey_user_handles',
              'passkey_credentials'
          )
      )
)
SELECT 'schema_object', 'constraints', count(*)::text,
       COALESCE(sum(hashtextextended(state, 0)::numeric), 0)::text,
       COALESCE(sum(hashtextextended(state, 1)::numeric), 0)::text
FROM constraint_state;

WITH index_state AS (
    SELECT format(
        '%I.%I|%s',
        n.nspname,
        c.relname,
        pg_get_indexdef(i.indexrelid)
    ) AS state
    FROM pg_index AS i
    JOIN pg_class AS c ON c.oid = i.indrelid
    JOIN pg_class AS index_class ON index_class.oid = i.indexrelid
    JOIN pg_namespace AS n ON n.oid = c.relnamespace
    WHERE n.nspname NOT IN ('pg_catalog', 'pg_toast', 'information_schema')
      AND index_class.relname NOT IN (
          'idx_referral_reward_reviews_reward_user',
          'idx_referral_reward_reviews_inviter_invitee',
          'idx_referral_balance_grants_user'
      )
      AND NOT (
          n.nspname = 'public'
          AND c.relname IN (
              'model_catalog_metadata',
              'lottery_campaigns',
              'lottery_prizes',
              'lottery_entries',
              'lottery_draw_runs',
              'lottery_events',
              'passkey_user_handles',
              'passkey_credentials'
          )
      )
)
SELECT 'schema_object', 'indexes', count(*)::text,
       COALESCE(sum(hashtextextended(state, 0)::numeric), 0)::text,
       COALESCE(sum(hashtextextended(state, 1)::numeric), 0)::text
FROM index_state;

WITH trigger_state AS (
    SELECT format(
        '%I.%I|%s|%s|%s',
        n.nspname,
        c.relname,
        t.tgname,
        t.tgenabled,
        pg_get_triggerdef(t.oid, true)
    ) AS state
    FROM pg_trigger AS t
    JOIN pg_class AS c ON c.oid = t.tgrelid
    JOIN pg_namespace AS n ON n.oid = c.relnamespace
    WHERE NOT t.tgisinternal
      AND n.nspname NOT IN ('pg_catalog', 'information_schema')
      AND t.tgname NOT IN (
          'trg_referral_first_recharge_insert',
          'trg_referral_first_recharge_update',
          'trg_referral_registration_rewards_insert',
          'trg_referral_registration_rewards_update',
          'trg_reward_reviews_notify_changed',
          'trg_referral_refresh_admin_registration_ip_risk_flags'
      )
)
SELECT 'schema_object', 'triggers', count(*)::text,
       COALESCE(sum(hashtextextended(state, 0)::numeric), 0)::text,
       COALESCE(sum(hashtextextended(state, 1)::numeric), 0)::text
FROM trigger_state;

WITH function_state AS (
    SELECT format(
        '%I.%I(%s)|%s',
        n.nspname,
        p.proname,
        pg_get_function_identity_arguments(p.oid),
        pg_get_functiondef(p.oid)
    ) AS state
    FROM pg_proc AS p
    JOIN pg_namespace AS n ON n.oid = p.pronamespace
    WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
      AND p.prokind IN ('f', 'p')
)
SELECT 'schema_object', 'functions', count(*)::text,
       COALESCE(sum(hashtextextended(state, 0)::numeric), 0)::text,
       COALESCE(sum(hashtextextended(state, 1)::numeric), 0)::text
FROM function_state;

WITH view_state AS (
    SELECT format(
        '%I.%I|%s',
        n.nspname,
        c.relname,
        pg_get_viewdef(c.oid, true)
    ) AS state
    FROM pg_class AS c
    JOIN pg_namespace AS n ON n.oid = c.relnamespace
    WHERE c.relkind IN ('v', 'm')
      AND n.nspname NOT IN ('pg_catalog', 'information_schema')
)
SELECT 'schema_object', 'views', count(*)::text,
       COALESCE(sum(hashtextextended(state, 0)::numeric), 0)::text,
       COALESCE(sum(hashtextextended(state, 1)::numeric), 0)::text
FROM view_state;

WITH invalid_state AS (
    SELECT format('%I.%I|%s', n.nspname, c.relname, con.conname) AS state
    FROM pg_constraint AS con
    JOIN pg_class AS c ON c.oid = con.conrelid
    JOIN pg_namespace AS n ON n.oid = c.relnamespace
    WHERE NOT con.convalidated
      AND n.nspname NOT IN ('pg_catalog', 'information_schema')
)
SELECT 'schema_object', 'unvalidated_constraints', count(*)::text,
       COALESCE(sum(hashtextextended(state, 0)::numeric), 0)::text,
       COALESCE(sum(hashtextextended(state, 1)::numeric), 0)::text
FROM invalid_state;
