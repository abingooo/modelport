package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationCyberEvidenceMigration(t *testing.T) {
	sql, err := FS.ReadFile("218_content_moderation_cyber_evidence.sql")
	require.NoError(t, err)
	text := string(sql)
	require.Contains(t, text, "CREATE TABLE IF NOT EXISTS content_moderation_cyber_evidence")
	require.Contains(t, text, "REFERENCES content_moderation_logs(id) ON DELETE CASCADE")
	require.Contains(t, text, "request_body_ciphertext TEXT NOT NULL")
	require.Contains(t, text, "request_body_sha256")
	require.NotContains(t, text, "request_body_plaintext")
}
