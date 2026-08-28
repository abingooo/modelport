package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelPortLotteryBridgeIsAdditiveAndSelfContained(t *testing.T) {
	body, err := FS.ReadFile("233_modelport_lottery_bridge.sql")
	require.NoError(t, err)

	sql := strings.ToLower(string(body))
	for _, fragment := range []string{
		"create table if not exists lottery_campaigns",
		"create table if not exists lottery_prizes",
		"create table if not exists lottery_entries",
		"create table if not exists lottery_draw_runs",
		"create table if not exists lottery_events",
		"add column if not exists full_draw_participant_limit",
		"add column if not exists full_draw_reached_at",
		"create index if not exists idx_lottery_campaigns_full_draw_due",
	} {
		require.Contains(t, sql, fragment)
	}
	for _, destructive := range []string{
		"drop table", "truncate", "delete from lottery_", "update lottery_",
	} {
		require.NotContains(t, sql, destructive)
	}
}

func TestModelPortLotteryBridgeDoesNotActivateLegacyMigrationNames(t *testing.T) {
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotEqual(t, "191_create_lottery_system.sql", entry.Name())
		require.NotEqual(t, "202_lottery_full_draw.sql", entry.Name())
	}
}
