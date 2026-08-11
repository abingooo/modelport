package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelPortChannelMonitorV2DefaultsMigration(t *testing.T) {
	content, err := FS.ReadFile("221_modelport_channel_monitor_v2_defaults.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	for _, platform := range []string{
		"deepseek", "qwen", "glm", "kimi", "doubao", "minimax", "mimo",
	} {
		require.Contains(t, sql, `"platform": "`+platform+`"`)
	}
	for _, model := range []string{"qwen3.5-plus", "qwen3.6-plus", "qwen3.7-plus"} {
		require.Contains(t, sql, `"`+model+`"`)
	}

	require.Contains(t, sql, "refresh_interval_seconds = 300")
	require.Contains(t, sql, "version = 2")
	require.Contains(t, sql, "updated_by is null")
	require.NotContains(t, sql, "refresh_interval_seconds = 60")
	require.Contains(t, sql, "not exists")
}

func TestModelPortNonGrokVideoPriceRestoreMigration(t *testing.T) {
	content, err := FS.ReadFile("222_restore_modelport_non_grok_video_prices.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	require.Contains(t, sql, "from groups_video_price_backup_220")
	require.Contains(t, sql, "g.platform is distinct from 'grok'")
	require.Contains(t, sql, "g.platform is distinct from 'composite'")
	for _, column := range []string{
		"video_price_480p", "video_price_720p", "video_price_1080p", "video_model_prices",
	} {
		require.Contains(t, sql, "g."+column+" is null")
	}
	require.NotContains(t, sql, "drop table")
}
