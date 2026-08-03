-- Allow ModelPort's dedicated domestic OpenAI-compatible platforms in channel monitoring.

ALTER TABLE channel_monitors
    DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN (
        'openai', 'anthropic', 'gemini', 'grok',
        'deepseek', 'qwen', 'glm', 'kimi', 'doubao', 'minimax', 'mimo'
    ));

ALTER TABLE channel_monitor_request_templates
    DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN (
        'openai', 'anthropic', 'gemini', 'grok',
        'deepseek', 'qwen', 'glm', 'kimi', 'doubao', 'minimax', 'mimo'
    ));
