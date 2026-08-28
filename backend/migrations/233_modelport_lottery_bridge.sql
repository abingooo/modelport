-- ModelPort lottery bridge for both clean Sub2API v0.1.183 databases and
-- databases upgraded from custom-v0.1.176.2. The legacy release used
-- migrations 191 and 202; those files are archived and must not be replayed.

CREATE TABLE IF NOT EXISTS lottery_campaigns (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    mode VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    draw_at TIMESTAMPTZ,
    full_draw_participant_limit INTEGER,
    full_draw_reached_at TIMESTAMPTZ,
    per_user_limit INTEGER NOT NULL DEFAULT 1,
    minimum_balance NUMERIC(20,8) NOT NULL DEFAULT 0,
    required_subscription_group_ids BIGINT[] NOT NULL DEFAULT '{}',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_lottery_campaign_mode CHECK (mode IN ('instant', 'scheduled')),
    CONSTRAINT chk_lottery_campaign_status CHECK (status IN ('draft', 'active', 'paused', 'completed')),
    CONSTRAINT chk_lottery_campaign_window CHECK (ends_at > starts_at),
    CONSTRAINT chk_lottery_campaign_limit CHECK (per_user_limit BETWEEN 1 AND 1000),
    CONSTRAINT chk_lottery_campaign_minimum_balance CHECK (minimum_balance >= 0),
    CONSTRAINT chk_lottery_campaign_draw CHECK (
        (mode = 'instant' AND draw_at IS NULL) OR
        (mode = 'scheduled' AND draw_at IS NOT NULL AND draw_at >= ends_at)
    ),
    CONSTRAINT chk_lottery_campaign_full_draw_limit CHECK (
        full_draw_participant_limit IS NULL OR (
            mode = 'scheduled' AND full_draw_participant_limit BETWEEN 1 AND 1000000
        )
    ),
    CONSTRAINT chk_lottery_campaign_full_draw_reached CHECK (
        full_draw_reached_at IS NULL OR full_draw_participant_limit IS NOT NULL
    )
);

ALTER TABLE lottery_campaigns
    ADD COLUMN IF NOT EXISTS full_draw_participant_limit INTEGER,
    ADD COLUMN IF NOT EXISTS full_draw_reached_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_lottery_campaign_full_draw_limit'
    ) THEN
        ALTER TABLE lottery_campaigns
            ADD CONSTRAINT chk_lottery_campaign_full_draw_limit CHECK (
                full_draw_participant_limit IS NULL OR (
                    mode = 'scheduled' AND full_draw_participant_limit BETWEEN 1 AND 1000000
                )
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_lottery_campaign_full_draw_reached'
    ) THEN
        ALTER TABLE lottery_campaigns
            ADD CONSTRAINT chk_lottery_campaign_full_draw_reached CHECK (
                full_draw_reached_at IS NULL OR full_draw_participant_limit IS NOT NULL
            );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS lottery_prizes (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE,
    name VARCHAR(160) NOT NULL,
    prize_type VARCHAR(32) NOT NULL,
    balance_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    subscription_group_id BIGINT REFERENCES groups(id) ON DELETE RESTRICT,
    subscription_validity_days INTEGER NOT NULL DEFAULT 0,
    probability_bps INTEGER NOT NULL,
    inventory INTEGER NOT NULL,
    awarded_count INTEGER NOT NULL DEFAULT 0,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_lottery_prize_type CHECK (prize_type IN ('balance', 'subscription_code')),
    CONSTRAINT chk_lottery_prize_probability CHECK (probability_bps BETWEEN 1 AND 10000),
    CONSTRAINT chk_lottery_prize_inventory CHECK (inventory > 0 AND awarded_count BETWEEN 0 AND inventory),
    CONSTRAINT chk_lottery_prize_payload CHECK (
        (prize_type = 'balance' AND balance_amount > 0 AND subscription_group_id IS NULL AND subscription_validity_days = 0) OR
        (prize_type = 'subscription_code' AND balance_amount = 0 AND subscription_group_id IS NOT NULL AND subscription_validity_days > 0)
    )
);

CREATE TABLE IF NOT EXISTS lottery_entries (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES lottery_campaigns(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(128) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    prize_id BIGINT REFERENCES lottery_prizes(id) ON DELETE RESTRICT,
    prize_name VARCHAR(160) NOT NULL DEFAULT '',
    prize_type VARCHAR(32) NOT NULL DEFAULT '',
    balance_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    subscription_group_id BIGINT REFERENCES groups(id) ON DELETE RESTRICT,
    subscription_validity_days INTEGER NOT NULL DEFAULT 0,
    reward_redeem_code_id BIGINT REFERENCES redeem_codes(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT uq_lottery_entry_idempotency UNIQUE (campaign_id, user_id, idempotency_key),
    CONSTRAINT chk_lottery_entry_status CHECK (status IN ('pending', 'won', 'not_won')),
    CONSTRAINT chk_lottery_entry_reward CHECK (
        (status IN ('pending', 'not_won') AND prize_id IS NULL AND prize_type = '' AND balance_amount = 0 AND subscription_group_id IS NULL AND subscription_validity_days = 0 AND reward_redeem_code_id IS NULL) OR
        (status = 'won' AND prize_id IS NOT NULL AND (
            (prize_type = 'balance' AND balance_amount > 0 AND subscription_group_id IS NULL AND subscription_validity_days = 0 AND reward_redeem_code_id IS NULL) OR
            (prize_type = 'subscription_code' AND balance_amount = 0 AND subscription_group_id IS NOT NULL AND subscription_validity_days > 0 AND reward_redeem_code_id IS NOT NULL)
        ))
    )
);

CREATE TABLE IF NOT EXISTS lottery_draw_runs (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL UNIQUE REFERENCES lottery_campaigns(id) ON DELETE RESTRICT,
    participant_count INTEGER NOT NULL DEFAULT 0,
    winner_count INTEGER NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    triggered_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT chk_lottery_draw_counts CHECK (
        participant_count >= 0 AND winner_count >= 0 AND winner_count <= participant_count
    )
);

CREATE TABLE IF NOT EXISTS lottery_events (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES lottery_campaigns(id) ON DELETE RESTRICT,
    entry_id BIGINT REFERENCES lottery_entries(id) ON DELETE RESTRICT,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    event_type VARCHAR(48) NOT NULL,
    balance_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    redeem_code_id BIGINT REFERENCES redeem_codes(id) ON DELETE RESTRICT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_lottery_event_type CHECK (event_type IN (
        'participated', 'entry_won', 'entry_not_won', 'balance_credited',
        'subscription_code_issued', 'scheduled_draw_completed'
    )),
    CONSTRAINT chk_lottery_event_balance CHECK (balance_amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_public
    ON lottery_campaigns (status, starts_at, ends_at, id DESC);
CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_due
    ON lottery_campaigns (draw_at, id) WHERE mode = 'scheduled' AND status = 'active';
CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_full_draw_due
    ON lottery_campaigns (full_draw_reached_at, id)
    WHERE mode = 'scheduled' AND status = 'active' AND full_draw_reached_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_lottery_prizes_campaign
    ON lottery_prizes (campaign_id, is_enabled, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_lottery_entries_user
    ON lottery_entries (user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_lottery_entries_campaign_status
    ON lottery_entries (campaign_id, status, id);
CREATE INDEX IF NOT EXISTS idx_lottery_events_campaign
    ON lottery_events (campaign_id, created_at DESC, id DESC);

COMMENT ON TABLE lottery_events IS
    'Append-only transactional audit trail for lottery participation and reward fulfillment';
