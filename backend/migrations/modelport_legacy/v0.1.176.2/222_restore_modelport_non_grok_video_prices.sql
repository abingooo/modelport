-- ModelPort permits existing non-Grok groups to retain explicitly configured
-- video prices. Upstream migration 220 snapshots those values and clears the
-- live columns; restore only rows that are still fully empty so this bridge
-- never overwrites a later operator change.

UPDATE groups AS g
SET video_price_480p = b.video_price_480p,
    video_price_720p = b.video_price_720p,
    video_price_1080p = b.video_price_1080p,
    video_model_prices = b.video_model_prices
FROM groups_video_price_backup_220 AS b
WHERE g.id = b.group_id
  AND g.platform IS DISTINCT FROM 'grok'
  AND g.platform IS DISTINCT FROM 'composite'
  AND g.video_price_480p IS NULL
  AND g.video_price_720p IS NULL
  AND g.video_price_1080p IS NULL
  AND g.video_model_prices IS NULL;

COMMENT ON TABLE groups_video_price_backup_220 IS
    'Migration 220 snapshot retained by ModelPort after migration 222 restored existing non-Grok video prices. Keep until the upgrade backup retention window ends.';
