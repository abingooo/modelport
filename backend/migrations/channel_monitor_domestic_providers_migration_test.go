package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorDomesticProvidersMigration(t *testing.T) {
	content, err := FS.ReadFile("197_channel_monitor_domestic_providers.sql")
	require.NoError(t, err)

	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	for _, provider := range []string{"deepseek", "qwen", "glm", "kimi", "doubao", "minimax", "mimo"} {
		require.Contains(t, sql, "'"+provider+"'")
	}
}
