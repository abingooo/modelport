package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelPricingUserVisibilityMigrationDefaultsExistingPricingToVisible(t *testing.T) {
	content, err := FS.ReadFile("193_add_channel_pricing_user_visibility.sql")
	require.NoError(t, err)
	require.Contains(t, string(content), "user_visible BOOLEAN NOT NULL DEFAULT TRUE")
}
