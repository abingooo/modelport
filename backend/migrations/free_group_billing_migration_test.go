package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFreeGroupBillingMigrationDefaultsExistingGroupsToStandardBilling(t *testing.T) {
	content, err := FS.ReadFile("192_add_free_group_billing.sql")
	require.NoError(t, err)
	require.Contains(t, string(content), "is_free BOOLEAN NOT NULL DEFAULT FALSE")
}
