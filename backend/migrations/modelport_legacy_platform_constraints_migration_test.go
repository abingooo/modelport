package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelPortLegacyPlatformConstraintsMigrationPreservesStorageAndFailsClosed(t *testing.T) {
	body, err := FS.ReadFile("236_modelport_legacy_platform_constraints.sql")
	require.NoError(t, err)
	sql := string(body)

	for _, constraint := range []string{
		"user_platform_quotas_platform_check",
		"composite_model_routes_target_platform_check",
		"channel_monitors_provider_check",
		"channel_monitor_request_templates_provider_check",
	} {
		require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS "+constraint)
		require.Contains(t, sql, "ADD CONSTRAINT "+constraint)
	}
	for _, platform := range []string{
		"'antigravity'", "'kimi'", "'zhipu'", "'deepseek'",
		"'qwen'", "'glm'", "'doubao'", "'minimax'", "'mimo'",
	} {
		require.Contains(t, sql, platform)
	}
	require.Contains(t, sql, "'siliconflow'")
	require.Contains(t, sql, "'openrouter'")

	upper := strings.ToUpper(sql)
	require.NotContains(t, upper, "UPDATE ")
	require.NotContains(t, upper, "DELETE ")
	require.NotContains(t, upper, "INSERT ")
	require.Contains(t, upper, "RAISE EXCEPTION")
	require.Contains(t, sql, "legacy ModelPort provider configuration blocks migration")
	for _, source := range []string{
		"accounts.platform",
		"groups.platform",
		"composite_model_routes.target_platform",
		"user_platform_quotas.platform",
	} {
		require.Contains(t, sql, source)
	}
	require.Contains(t, sql, "this migration will not rename, delete, or disable data")
}
