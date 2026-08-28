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

CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_full_draw_due
    ON lottery_campaigns (full_draw_reached_at, id)
    WHERE mode = 'scheduled' AND status = 'active' AND full_draw_reached_at IS NOT NULL;
