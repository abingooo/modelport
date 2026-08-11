-- Extend the untouched upstream Channel Monitor V2 factory configuration with
-- ModelPort's domestic providers. The model list is only a presentation
-- allow-list; unlisted models continue to aggregate under __other__.
--
-- Migration 197 advances the upstream factory row to version 2. Requiring that
-- version plus updated_by IS NULL keeps operator-managed rows unchanged.

UPDATE channel_monitor_v2_config
SET platforms = platforms || $platforms$
[
  {
    "platform": "deepseek",
    "enabled": true,
    "models": ["deepseek-chat", "deepseek-reasoner"]
  },
  {
    "platform": "qwen",
    "enabled": true,
    "models": [
      "qwen3.7-plus",
      "qwen3.6-plus",
      "qwen3.5-plus",
      "qwen-plus",
      "qwen-max",
      "qwen3-235b-a22b"
    ]
  },
  {
    "platform": "glm",
    "enabled": true,
    "models": ["glm-5.2", "glm-5", "glm-4.6", "glm-4.5", "glm-4-flash"]
  },
  {
    "platform": "kimi",
    "enabled": true,
    "models": ["kimi-k3", "kimi-k2.5", "kimi-k2", "kimi-latest", "moonshot-v1-128k"]
  },
  {
    "platform": "doubao",
    "enabled": true,
    "models": [
      "doubao-seed-1.8",
      "doubao-seed-1.6",
      "doubao-1.5-pro-256k",
      "doubao-1.5-thinking-pro"
    ]
  },
  {
    "platform": "minimax",
    "enabled": true,
    "models": ["MiniMax-M3", "MiniMax-M2.5", "MiniMax-M2.1", "MiniMax-Text-01"]
  },
  {
    "platform": "mimo",
    "enabled": true,
    "models": ["mimo-v2.5", "mimo-v2-flash", "mimo-v2-pro", "mimo-v2-omni"]
  }
]
$platforms$::jsonb,
    refresh_interval_seconds = 300,
    version = version + 1,
    updated_at = NOW()
WHERE id = 1
  AND version = 2
  AND updated_by IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM jsonb_array_elements(platforms) AS item
      WHERE item->>'platform' IN (
          'deepseek', 'qwen', 'glm', 'kimi', 'doubao', 'minimax', 'mimo'
      )
  );
