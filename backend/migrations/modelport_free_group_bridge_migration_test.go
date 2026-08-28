package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelPortFreeGroupBridgeMigrationIsExplicitAndIdempotent(t *testing.T) {
	body, err := FS.ReadFile("232_modelport_free_group_bridge.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))

	for _, fragment := range []string{
		"alter table groups",
		"add column if not exists is_free boolean",
		"set is_free = false",
		"where is_free is null",
		"alter column is_free set default false",
		"alter column is_free set not null",
		"alter table if exists batch_image_jobs",
		"add column if not exists is_free_billing boolean",
		"set is_free_billing = false",
		"where is_free_billing is null",
		"alter column is_free_billing set default false",
		"alter column is_free_billing set not null",
		"old.is_free is not distinct from new.is_free",
	} {
		require.Contains(t, sql, fragment)
	}

	// The bridge must not replay the archived legacy migrations or contain
	// destructive data operations.
	for _, fragment := range []string{
		"191_create_lottery_system.sql",
		"202_lottery_full_draw.sql",
		"drop table",
		"truncate",
		"delete from",
	} {
		require.NotContains(t, sql, fragment)
	}
}
