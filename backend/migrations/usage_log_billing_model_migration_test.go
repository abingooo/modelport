package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogBillingModelMigration(t *testing.T) {
	content, err := FS.ReadFile("189_add_usage_log_billing_model.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE usage_logs")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS billing_model VARCHAR(100)")
}
