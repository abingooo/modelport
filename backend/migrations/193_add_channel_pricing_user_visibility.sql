ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS user_visible BOOLEAN NOT NULL DEFAULT TRUE;

COMMENT ON COLUMN channel_model_pricing.user_visible IS '是否在用户侧模型定价页面展示该定价条目';
