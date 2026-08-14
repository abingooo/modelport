package migrations

import (
	"io/fs"
	"regexp"
	"sort"
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

func TestUpstreamGroupModelPricingMigrationUsesNextModelPortSequence(t *testing.T) {
	content, err := FS.ReadFile("223_group_model_pricing.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	require.Contains(t, sql, "add column if not exists long_context_pricing_enabled boolean not null default true")
	require.Contains(t, sql, "add column if not exists model_pricing jsonb")
	require.Contains(t, sql, "set long_context_pricing_enabled = true")
	require.Contains(t, sql, "where long_context_pricing_enabled is distinct from true")
	require.NotContains(t, sql, "default false")
}

func TestModelPortBridgeMigrationSequenceNumbersAreUnique(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	require.NoError(t, err)

	sequencePattern := regexp.MustCompile(`^(\d+)_.*\.sql$`)
	targetSequences := map[string]bool{"221": true, "222": true, "223": true}
	seen := make(map[string]string)
	var bridgeFiles []string
	for _, entry := range entries {
		match := sequencePattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 || !targetSequences[match[1]] {
			continue
		}
		if previous, ok := seen[match[1]]; ok {
			require.Failf(t, "duplicate migration sequence", "sequence %s is used by %s and %s", match[1], previous, entry.Name())
		}
		seen[match[1]] = entry.Name()
		bridgeFiles = append(bridgeFiles, entry.Name())
	}
	sort.Strings(bridgeFiles)
	require.Equal(t, []string{
		"221_modelport_channel_monitor_v2_defaults.sql",
		"222_restore_modelport_non_grok_video_prices.sql",
		"223_group_model_pricing.sql",
	}, bridgeFiles)
}
