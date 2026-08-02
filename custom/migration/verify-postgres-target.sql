\set ON_ERROR_STOP on
\set QUIET 1
SET client_min_messages = warning;

DO $$
DECLARE
    expected_migrations text[] := ARRAY[
        '187_add_deepseek_platform.sql',
        '188_add_openai_compatible_providers.sql',
        '189_add_usage_log_billing_model.sql',
        '190_create_model_catalog_metadata.sql',
        '191_create_lottery_system.sql',
        '191_passkey_credentials.sql',
        '192_add_free_group_billing.sql',
        '193_add_channel_pricing_user_visibility.sql',
        '194_remove_image_site_setting.sql',
        '195_affiliate_reward_review_program.sql',
        '196_affiliate_default_inviter.sql'
    ];
    expected_tables text[] := ARRAY[
        'model_catalog_metadata',
        'lottery_campaigns',
        'lottery_prizes',
        'lottery_entries',
        'lottery_draw_runs',
        'lottery_events',
        'passkey_user_handles',
        'passkey_credentials'
    ];
    actual_count bigint;
    object_name text;
BEGIN
    SELECT count(*) INTO actual_count FROM public.schema_migrations;
    IF actual_count <> 247 THEN
        RAISE EXCEPTION 'expected 247 schema migrations, found %', actual_count;
    END IF;

    FOREACH object_name IN ARRAY expected_migrations LOOP
        IF NOT EXISTS (
            SELECT 1 FROM public.schema_migrations WHERE filename = object_name
        ) THEN
            RAISE EXCEPTION 'required migration is missing: %', object_name;
        END IF;
    END LOOP;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'groups'
          AND column_name = 'is_free' AND is_nullable = 'NO'
    ) THEN
        RAISE EXCEPTION 'groups.is_free is missing or nullable';
    END IF;
    IF EXISTS (SELECT 1 FROM public.groups WHERE is_free IS NULL) THEN
        RAISE EXCEPTION 'groups.is_free contains NULL';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'channel_model_pricing'
          AND column_name = 'user_visible' AND is_nullable = 'NO'
    ) THEN
        RAISE EXCEPTION 'channel_model_pricing.user_visible is missing or nullable';
    END IF;
    IF EXISTS (SELECT 1 FROM public.channel_model_pricing WHERE user_visible IS NULL) THEN
        RAISE EXCEPTION 'channel_model_pricing.user_visible contains NULL';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'usage_logs'
          AND column_name = 'billing_model'
    ) THEN
        RAISE EXCEPTION 'usage_logs.billing_model is missing';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM public.usage_logs AS usage
        CROSS JOIN public.schema_migrations AS migration
        WHERE migration.filename = '189_add_usage_log_billing_model.sql'
          AND usage.billing_model IS NOT NULL
          AND usage.created_at < migration.applied_at
    ) THEN
        RAISE EXCEPTION 'pre-migration usage_logs.billing_model values were unexpectedly populated';
    END IF;

    FOREACH object_name IN ARRAY expected_tables LOOP
        IF to_regclass('public.' || object_name) IS NULL THEN
            RAISE EXCEPTION 'required target table is missing: %', object_name;
        END IF;
        EXECUTE format('SELECT count(*) FROM public.%I', object_name) INTO actual_count;
        IF actual_count <> 0 THEN
            RAISE EXCEPTION 'new target table % is not empty: % rows', object_name, actual_count;
        END IF;
    END LOOP;

    IF EXISTS (SELECT 1 FROM public.settings WHERE key = 'image_site_url') THEN
        RAISE EXCEPTION 'removed image_site_url setting is still present';
    END IF;

    IF to_regnamespace('lottery') IS NULL OR to_regnamespace('referral') IS NULL THEN
        RAISE EXCEPTION 'TokensHub custom schemas were not preserved';
    END IF;

    IF to_regclass('referral.reward_reviews') IS NULL
       OR to_regclass('referral.balance_grants') IS NULL
       OR to_regclass('referral.user_registration_ip_proxy') IS NULL THEN
        RAISE EXCEPTION 'referral history tables were not preserved';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM public.settings
        WHERE key = 'affiliate_reward_program_config'
          AND COALESCE((value::jsonb->>'enabled')::boolean, false)
          AND COALESCE((value::jsonb->>'version')::integer, 0) = 1
          AND value::jsonb->>'legacy_approval_cutoff' = '2026-07-05T22:00:00Z'
          AND COALESCE((value::jsonb->'registration'->>'default_inviter_enabled')::boolean, false)
          AND COALESCE((value::jsonb->'registration'->>'default_inviter_user_id')::bigint, 0) = 1
    ) THEN
        RAISE EXCEPTION 'affiliate reward program was not adopted from referral history';
    END IF;
    IF (
        SELECT count(*)
        FROM pg_trigger
        WHERE tgname IN (
            'trg_referral_first_recharge_insert',
            'trg_referral_first_recharge_update',
            'trg_referral_registration_rewards_insert',
            'trg_referral_registration_rewards_update',
            'trg_reward_reviews_notify_changed',
            'trg_referral_refresh_admin_registration_ip_risk_flags'
        )
          AND NOT tgisinternal
    ) <> 6 THEN
        RAISE EXCEPTION 'legacy referral trigger definitions were not preserved';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_trigger
        WHERE tgname IN (
            'trg_referral_first_recharge_insert',
            'trg_referral_first_recharge_update',
            'trg_referral_registration_rewards_insert',
            'trg_referral_registration_rewards_update',
            'trg_reward_reviews_notify_changed',
            'trg_referral_refresh_admin_registration_ip_risk_flags'
        )
          AND NOT tgisinternal
          AND tgenabled <> 'D'
    ) THEN
        RAISE EXCEPTION 'a legacy referral trigger remains enabled';
    END IF;

    IF (
        SELECT count(*)
        FROM pg_constraint AS con
        JOIN pg_class AS c ON c.oid = con.conrelid
        JOIN pg_namespace AS n ON n.oid = c.relnamespace
        WHERE NOT con.convalidated
          AND n.nspname NOT IN ('pg_catalog', 'information_schema')
    ) <> 2 THEN
        RAISE EXCEPTION 'unexpected count of unvalidated constraints';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_constraint AS con
        JOIN pg_class AS c ON c.oid = con.conrelid
        JOIN pg_namespace AS n ON n.oid = c.relnamespace
        WHERE NOT con.convalidated
          AND format('%I.%I.%I', n.nspname, c.relname, con.conname) NOT IN (
              'public.usage_logs.usage_logs_image_billing_size_check',
              'public.usage_logs.usage_logs_image_size_source_check'
          )
    ) THEN
        RAISE EXCEPTION 'an unexpected unvalidated constraint exists';
    END IF;
END
$$;

\pset tuples_only on
\pset format unaligned
\pset fieldsep '\t'
SELECT 'target_verification', 'PASS';
SELECT 'schema_migrations', count(*) FROM public.schema_migrations;
SELECT 'users', count(*) FROM public.users;
SELECT 'api_keys', count(*) FROM public.api_keys;
SELECT 'usage_logs', count(*) FROM public.usage_logs;
SELECT 'referral_reward_reviews', count(*) FROM referral.reward_reviews;
SELECT 'referral_balance_grants', count(*) FROM referral.balance_grants;
