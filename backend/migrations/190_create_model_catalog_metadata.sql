CREATE TABLE IF NOT EXISTS model_catalog_metadata (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(32) NOT NULL,
    model_name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    context_window BIGINT NOT NULL DEFAULT 0,
    interface_formats JSONB NOT NULL DEFAULT '[]'::jsonb,
    scenarios JSONB NOT NULL DEFAULT '[]'::jsonb,
    example_overrides JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_recommended BOOLEAN NOT NULL DEFAULT FALSE,
    is_visible BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_model_catalog_metadata_context_window CHECK (context_window >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_model_catalog_metadata_platform_model_ci
    ON model_catalog_metadata (platform, LOWER(model_name));

CREATE INDEX IF NOT EXISTS idx_model_catalog_metadata_listing
    ON model_catalog_metadata (is_visible, is_recommended DESC, sort_order, platform, model_name);

COMMENT ON TABLE model_catalog_metadata IS 'ModelPort model catalog presentation metadata; pricing remains sourced from channels';
