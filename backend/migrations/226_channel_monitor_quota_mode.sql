-- Migration: 226_channel_monitor_quota_mode
-- 渠道监控配额模式：
--   1. provider 扩容到全部 8 平台（antigravity/kimi/zhipu/deepseek）
--      （antigravity 仅支持配额模式，无探活 adapter；国产 3 家复用 OpenAI 兼容探活）
--   2. check_mode：probe（默认，现状探活）/ quota（仅查关联账号用量，零 LLM 成本）
--      / quota_probe（探活 + 配额并存）
--   3. account_id 关联已有账号（配额模式的数据源，复用账号侧用量服务）；
--      账号删除时置空，监控保留并报「账号未关联」
--   4. channel_monitor_histories.quota 持久化归一化配额快照（JSONB）
--   5. 新增公开设置 channel_monitor_show_quota（默认关闭）：
--      控制用户端监控页是否展示配额/余额；管理端始终可见

DO $$
DECLARE
    monitor_constraint_def TEXT;
    template_constraint_def TEXT;
    monitor_provider_bound BOOLEAN;
    template_provider_bound BOOLEAN;
    monitor_provider_values TEXT[];
    template_provider_values TEXT[];
BEGIN
    SELECT pg_get_constraintdef(c.oid)
           , COALESCE(
               c.contype = 'c'
               AND c.conkey IS NOT NULL
               AND cardinality(c.conkey) = 1
               AND EXISTS (
                   SELECT 1
                     FROM pg_attribute a
                    WHERE a.attrelid = c.conrelid
                      AND a.attnum = c.conkey[1]
                      AND a.attname = 'provider'
                      AND NOT a.attisdropped
               ),
               FALSE
           )
      INTO monitor_constraint_def, monitor_provider_bound
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
      JOIN pg_namespace n ON n.oid = t.relnamespace
     WHERE t.relname = 'channel_monitors'
       AND n.nspname = 'public'
       AND c.conrelid = 'public.channel_monitors'::regclass
       AND c.conname = 'channel_monitors_provider_check';

    -- pg_get_constraintdef renders CHECK membership as either IN (...) or
    -- = ANY (ARRAY[...]). Extract and sort the literals so a partial list
    -- (for example one that merely contains kimi) cannot be mistaken for the
    -- complete upstream allow-list. The terminal 236 superset is accepted so
    -- this migration never narrows providers restored by the legacy bridge.
    SELECT COALESCE(
        ARRAY(
            SELECT match[1]
            FROM regexp_matches(monitor_constraint_def, $re$'([^']*)'$re$, 'g') AS match
            ORDER BY match[1]
        ),
        ARRAY[]::TEXT[]
    ) INTO monitor_provider_values;

    IF monitor_constraint_def IS NULL OR NOT COALESCE(monitor_provider_bound, FALSE) OR NOT (
        (
            cardinality(monitor_provider_values) = 8
            AND monitor_provider_values <@ ARRAY[
            'antigravity', 'anthropic', 'deepseek', 'gemini',
            'grok', 'kimi', 'openai', 'zhipu'
            ]::TEXT[]
            AND ARRAY[
                'antigravity', 'anthropic', 'deepseek', 'gemini',
                'grok', 'kimi', 'openai', 'zhipu'
            ]::TEXT[] <@ monitor_provider_values
        )
        OR (
            cardinality(monitor_provider_values) = 13
            AND monitor_provider_values <@ ARRAY[
            'antigravity', 'anthropic', 'deepseek', 'doubao',
            'gemini', 'glm', 'grok', 'kimi', 'mimo', 'minimax',
            'openai', 'qwen', 'zhipu'
            ]::TEXT[]
            AND ARRAY[
                'antigravity', 'anthropic', 'deepseek', 'doubao',
                'gemini', 'glm', 'grok', 'kimi', 'mimo', 'minimax',
                'openai', 'qwen', 'zhipu'
            ]::TEXT[] <@ monitor_provider_values
        )
    ) THEN
        ALTER TABLE channel_monitors
            DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                                'antigravity', 'kimi', 'zhipu', 'deepseek'));
    END IF;

    SELECT pg_get_constraintdef(c.oid)
           , COALESCE(
               c.contype = 'c'
               AND c.conkey IS NOT NULL
               AND cardinality(c.conkey) = 1
               AND EXISTS (
                   SELECT 1
                     FROM pg_attribute a
                    WHERE a.attrelid = c.conrelid
                      AND a.attnum = c.conkey[1]
                      AND a.attname = 'provider'
                      AND NOT a.attisdropped
               ),
               FALSE
           )
      INTO template_constraint_def, template_provider_bound
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
      JOIN pg_namespace n ON n.oid = t.relnamespace
     WHERE t.relname = 'channel_monitor_request_templates'
       AND n.nspname = 'public'
       AND c.conrelid = 'public.channel_monitor_request_templates'::regclass
       AND c.conname = 'channel_monitor_request_templates_provider_check';

    SELECT COALESCE(
        ARRAY(
            SELECT match[1]
            FROM regexp_matches(template_constraint_def, $re$'([^']*)'$re$, 'g') AS match
            ORDER BY match[1]
        ),
        ARRAY[]::TEXT[]
    ) INTO template_provider_values;

    IF template_constraint_def IS NULL OR NOT COALESCE(template_provider_bound, FALSE) OR NOT (
        (
            cardinality(template_provider_values) = 8
            AND template_provider_values <@ ARRAY[
            'antigravity', 'anthropic', 'deepseek', 'gemini',
            'grok', 'kimi', 'openai', 'zhipu'
            ]::TEXT[]
            AND ARRAY[
                'antigravity', 'anthropic', 'deepseek', 'gemini',
                'grok', 'kimi', 'openai', 'zhipu'
            ]::TEXT[] <@ template_provider_values
        )
        OR (
            cardinality(template_provider_values) = 13
            AND template_provider_values <@ ARRAY[
            'antigravity', 'anthropic', 'deepseek', 'doubao',
            'gemini', 'glm', 'grok', 'kimi', 'mimo', 'minimax',
            'openai', 'qwen', 'zhipu'
            ]::TEXT[]
            AND ARRAY[
                'antigravity', 'anthropic', 'deepseek', 'doubao',
                'gemini', 'glm', 'grok', 'kimi', 'mimo', 'minimax',
                'openai', 'qwen', 'zhipu'
            ]::TEXT[] <@ template_provider_values
        )
    ) THEN
        ALTER TABLE channel_monitor_request_templates
            DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
        ALTER TABLE channel_monitor_request_templates
            ADD CONSTRAINT channel_monitor_request_templates_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                                'antigravity', 'kimi', 'zhipu', 'deepseek'));
    END IF;
END $$;

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS check_mode VARCHAR(32) NOT NULL DEFAULT 'probe';

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_check_mode_check
    CHECK (check_mode IN ('probe', 'quota', 'quota_probe'));

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_account_id ON channel_monitors(account_id);

COMMENT ON COLUMN channel_monitors.check_mode IS
    'probe = LLM 探活（默认）；quota = 仅查关联账号用量；quota_probe = 探活 + 配额';
COMMENT ON COLUMN channel_monitors.account_id IS
    '配额模式关联的账号 ID（数据源）；账号删除时置空';

ALTER TABLE channel_monitor_histories
    ADD COLUMN IF NOT EXISTS quota JSONB;

COMMENT ON COLUMN channel_monitor_histories.quota IS
    '配额模式监控的归一化配额快照（domain.MonitorQuotaSnapshot）；探活模式为 NULL';

-- 用户端是否展示配额/余额（默认关闭，fail-closed 解析：仅 "true" 视为开启）。
-- 管理端不受此开关影响。
INSERT INTO settings (key, value)
VALUES ('channel_monitor_show_quota', 'false')
ON CONFLICT (key) DO NOTHING;
