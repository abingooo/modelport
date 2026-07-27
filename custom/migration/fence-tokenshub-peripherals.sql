\set ON_ERROR_STOP on
\set QUIET 1

DO $$
DECLARE
    trigger_record record;
BEGIN
    FOR trigger_record IN
        SELECT *
        FROM (VALUES
            ('public', 'payment_orders', 'trg_referral_first_recharge_insert'),
            ('public', 'payment_orders', 'trg_referral_first_recharge_update'),
            ('public', 'user_affiliates', 'trg_referral_registration_rewards_insert'),
            ('public', 'user_affiliates', 'trg_referral_registration_rewards_update'),
            ('referral', 'reward_reviews', 'trg_reward_reviews_notify_changed'),
            ('referral', 'user_registration_ip_proxy', 'trg_referral_refresh_admin_registration_ip_risk_flags')
        ) AS expected(schema_name, table_name, trigger_name)
    LOOP
        IF EXISTS (
            SELECT 1
            FROM pg_trigger AS trigger
            JOIN pg_class AS relation ON relation.oid = trigger.tgrelid
            JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace
            WHERE namespace.nspname = trigger_record.schema_name
              AND relation.relname = trigger_record.table_name
              AND trigger.tgname = trigger_record.trigger_name
              AND NOT trigger.tgisinternal
        ) THEN
            EXECUTE format(
                'ALTER TABLE %I.%I DISABLE TRIGGER %I',
                trigger_record.schema_name,
                trigger_record.table_name,
                trigger_record.trigger_name
            );
        END IF;
    END LOOP;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_trigger AS trigger
        WHERE trigger.tgname IN (
            'trg_referral_first_recharge_insert',
            'trg_referral_first_recharge_update',
            'trg_referral_registration_rewards_insert',
            'trg_referral_registration_rewards_update',
            'trg_reward_reviews_notify_changed',
            'trg_referral_refresh_admin_registration_ip_risk_flags'
        )
          AND trigger.tgenabled <> 'D'
    ) THEN
        RAISE EXCEPTION 'a deferred TokensHub trigger remains enabled';
    END IF;

    IF (
        SELECT count(*)
        FROM pg_trigger AS trigger
        WHERE trigger.tgname IN (
            'accounts_enforce_openai_long_context_billing_extra',
            'accounts_propagate_openai_long_context_billing_extra',
            'trg_api_keys_auth_cache_invalidation',
            'trg_groups_auth_cache_invalidation',
            'trg_user_allowed_groups_auth_cache_invalidation',
            'trg_users_auth_cache_invalidation',
            'trg_subscription_plan_sales_limit'
        )
          AND NOT trigger.tgisinternal
          AND trigger.tgenabled <> 'D'
    ) <> 7 THEN
        RAISE EXCEPTION 'the seven expected ModelPort core triggers are not all enabled';
    END IF;
END
$$;

\pset tuples_only on
\pset format unaligned
\pset fieldsep '\t'
SELECT 'tokenshub_peripheral_fence', 'PASS';
