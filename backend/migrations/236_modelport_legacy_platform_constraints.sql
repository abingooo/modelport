-- Preserve legacy ModelPort identifiers without pretending that the removed
-- OpenAI-compatible gateway integrations still exist. Channel-monitor probes
-- retain their old provider IDs and have runtime adapters. Executable account,
-- group, and composite-route configuration for removed gateways must be
-- resolved explicitly by an operator before this migration can continue.

DO $$
DECLARE
    blocked_values TEXT;
BEGIN
    SELECT string_agg(format('%s=%L', source, value), ', ' ORDER BY source, value)
      INTO blocked_values
      FROM (
        SELECT 'accounts.platform' AS source, platform::text AS value
         FROM accounts
         WHERE status = 'active'
           AND deleted_at IS NULL
           AND platform IN ('qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo')
        UNION
        SELECT 'groups.platform', platform::text
          FROM groups
         WHERE status = 'active'
           AND deleted_at IS NULL
           AND platform IN ('qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo')
        UNION
        SELECT 'composite_model_routes.target_platform', target_platform::text
          FROM composite_model_routes
         WHERE deleted_at IS NULL
           AND enabled
           AND target_platform IN ('qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo')
        UNION
        SELECT 'user_platform_quotas.platform', platform::text
          FROM user_platform_quotas
         WHERE deleted_at IS NULL
           AND platform IN ('qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo')
           AND (
               daily_limit_usd IS NOT NULL
               OR weekly_limit_usd IS NOT NULL
               OR monthly_limit_usd IS NOT NULL
               OR daily_usage_usd <> 0
               OR weekly_usage_usd <> 0
               OR monthly_usage_usd <> 0
           )
      ) AS blocked;

    IF blocked_values IS NOT NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = 'check_violation',
            MESSAGE = 'legacy ModelPort provider configuration blocks migration',
            DETAIL = blocked_values,
            HINT = 'Preserve the rows and explicitly restore or reconfigure each provider before retrying; this migration will not rename, delete, or disable data.';
    END IF;
END $$;

-- Quota and composite-route rows may contain disabled or storage-only legacy
-- values. Keeping the union constraint preserves those rows; the preflight
-- above prevents live configuration from being treated as v0.1.183 support.

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;
ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN (
        'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
        'kimi', 'zhipu', 'deepseek',
        'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
    ));

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;
ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN (
        'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
        'kimi', 'zhipu', 'deepseek',
        'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
    ));

ALTER TABLE channel_monitors
    DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN (
        'openai', 'anthropic', 'gemini', 'grok',
        'antigravity', 'kimi', 'zhipu', 'deepseek',
        'qwen', 'glm', 'doubao', 'minimax', 'mimo'
    ));

ALTER TABLE channel_monitor_request_templates
    DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN (
        'openai', 'anthropic', 'gemini', 'grok',
        'antigravity', 'kimi', 'zhipu', 'deepseek',
        'qwen', 'glm', 'doubao', 'minimax', 'mimo'
    ));
