-- Source-managed invitation reward program.
--
-- Existing TokensHub referral tables are adopted in place. Historical rows are
-- intentionally left untouched; only missing schema objects are added.

CREATE SCHEMA IF NOT EXISTS referral;

CREATE TABLE IF NOT EXISTS referral.reward_reviews (
    id BIGSERIAL PRIMARY KEY,
    inviter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    invitee_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reward_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reward_type VARCHAR(64) NOT NULL,
    reward_amount DECIMAL(20,8) NOT NULL,
    payment_order_id BIGINT REFERENCES payment_orders(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    risk_flags JSONB NOT NULL DEFAULT '{}'::jsonb,
    reviewed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    review_note TEXT NOT NULL DEFAULT '',
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    feishu_notified_at TIMESTAMPTZ,
    feishu_chat_id TEXT,
    feishu_message_id TEXT,
    feishu_card_updated_at TIMESTAMPTZ,
    feishu_card_update_error TEXT,
    CONSTRAINT reward_reviews_status_check
        CHECK (status IN ('pending', 'approved', 'rejected', 'paid'))
);

ALTER TABLE referral.reward_reviews
    ADD COLUMN IF NOT EXISTS feishu_notified_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS feishu_chat_id TEXT,
    ADD COLUMN IF NOT EXISTS feishu_message_id TEXT,
    ADD COLUMN IF NOT EXISTS feishu_card_updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS feishu_card_update_error TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS ux_referral_reward_invitee_type_no_order
    ON referral.reward_reviews(invitee_user_id, reward_type)
    WHERE payment_order_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_referral_reward_order_type
    ON referral.reward_reviews(payment_order_id, reward_type)
    WHERE payment_order_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_referral_reward_reviews_status
    ON referral.reward_reviews(status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_referral_reward_reviews_reward_user
    ON referral.reward_reviews(reward_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_referral_reward_reviews_inviter_invitee
    ON referral.reward_reviews(inviter_user_id, invitee_user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS referral.balance_grants (
    id BIGSERIAL PRIMARY KEY,
    review_id BIGINT NOT NULL UNIQUE REFERENCES referral.reward_reviews(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount DECIMAL(20,8) NOT NULL,
    balance_before DECIMAL(20,8) NOT NULL,
    balance_after DECIMAL(20,8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_referral_balance_grants_user
    ON referral.balance_grants(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS referral.user_registration_ip_proxy (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    ip INET NOT NULL,
    source VARCHAR(32) NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_registration_ip_proxy_ip
    ON referral.user_registration_ip_proxy(ip);

CREATE INDEX IF NOT EXISTS idx_user_registration_ip_proxy_first_seen_at
    ON referral.user_registration_ip_proxy(first_seen_at);

-- Legacy deployments generated rewards from database triggers. ModelPort owns
-- this lifecycle in Go, so leave the trigger definitions available for a
-- rollback while preventing duplicate generation after this migration.
DO $$
DECLARE
    trigger_entry RECORD;
BEGIN
    FOR trigger_entry IN
        SELECT *
        FROM (VALUES
            ('public', 'payment_orders', 'trg_referral_first_recharge_insert'),
            ('public', 'payment_orders', 'trg_referral_first_recharge_update'),
            ('public', 'user_affiliates', 'trg_referral_registration_rewards_insert'),
            ('public', 'user_affiliates', 'trg_referral_registration_rewards_update'),
            ('referral', 'reward_reviews', 'trg_reward_reviews_notify_changed'),
            ('referral', 'user_registration_ip_proxy', 'trg_referral_refresh_admin_registration_ip_risk_flags')
        ) AS configured(schema_name, table_name, trigger_name)
    LOOP
        IF EXISTS (
            SELECT 1
            FROM pg_trigger t
            JOIN pg_class c ON c.oid = t.tgrelid
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = trigger_entry.schema_name
              AND c.relname = trigger_entry.table_name
              AND t.tgname = trigger_entry.trigger_name
              AND NOT t.tgisinternal
        ) THEN
            EXECUTE format(
                'ALTER TABLE %I.%I DISABLE TRIGGER %I',
                trigger_entry.schema_name,
                trigger_entry.table_name,
                trigger_entry.trigger_name
            );
        END IF;
    END LOOP;
END $$;

-- A legacy database with referral history adopts the production-compatible
-- rule set automatically. Fresh installations keep the source default (off)
-- until an administrator explicitly enables the program.
INSERT INTO settings(key, value, updated_at)
SELECT
    'affiliate_reward_program_config',
    jsonb_build_object(
        'version', 1,
        'enabled', true,
        'legacy_approval_cutoff', '2026-07-05T22:00:00Z',
        'registration', jsonb_build_object(
            'enabled', true,
            'inviter_bonus', 1,
            'invitee_trial_amount', 3,
            'invitee_trial_group_id', COALESCE((
                SELECT (risk_flags->>'group_id')::bigint
                FROM referral.reward_reviews
                WHERE reward_type = 'invite_register_invitee_pro_trial_card'
                  AND COALESCE(risk_flags->>'group_id', '') ~ '^[0-9]+$'
                ORDER BY created_at DESC, id DESC
                LIMIT 1
            ), 50),
            'invitee_trial_days', 3
        ),
        'first_recharge', jsonb_build_object(
            'enabled', true,
            'inviter_bonus', 2,
            'invitee_bonus_percent', 10
        )
    )::text,
    NOW()
WHERE EXISTS (SELECT 1 FROM referral.reward_reviews LIMIT 1)
ON CONFLICT (key) DO NOTHING;
