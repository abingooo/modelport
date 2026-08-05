package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLotteryFullDrawMigrationIsAdditiveAndDisabledByDefault(t *testing.T) {
	body, err := FS.ReadFile("202_lottery_full_draw.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "add column if not exists full_draw_participant_limit")
	require.Contains(t, sql, "add column if not exists full_draw_reached_at")
	require.Contains(t, sql, "mode = 'scheduled'")
	require.Contains(t, sql, "full_draw_participant_limit between 1 and 1000000")
	require.Contains(t, sql, "create index if not exists idx_lottery_campaigns_full_draw_due")
	for _, destructive := range []string{"drop table", "truncate", "delete from lottery_"} {
		require.NotContains(t, sql, destructive)
	}
}
