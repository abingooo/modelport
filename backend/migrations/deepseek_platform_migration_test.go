package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepSeekPlatformMigrationUpdatesPlatformChecks(t *testing.T) {
	content, err := FS.ReadFile("187_add_deepseek_platform.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "user_platform_quotas_platform_check")
	require.Contains(t, sql, "composite_model_routes_target_platform_check")
	require.GreaterOrEqual(t, strings.Count(sql, "'deepseek'"), 2)
}
