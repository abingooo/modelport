package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstructionAuditMigrationIsDefaultOffAndStoresNoRequestBody(t *testing.T) {
	body, err := FS.ReadFile("198_instruction_audit.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "('instruction_audit_enabled', 'false')")
	for _, table := range []string{
		"instruction_audit_hashes",
		"instruction_audit_rule_sets",
		"instruction_audit_rule_set_hashes",
		"instruction_audit_bindings",
		"instruction_audit_events",
		"instruction_audit_notification_outbox",
	} {
		require.Contains(t, sql, "create table if not exists "+table)
	}
	require.Contains(t, sql, "digest            char(64) not null unique")
	require.Contains(t, sql, "status in ('candidate', 'active', 'disabled', 'expired')")
	require.Contains(t, sql, "dedup_key         varchar(64) not null unique")
	require.Contains(t, sql, "sent_recipient_ids bigint[] not null default '{}'")
	require.Contains(t, sql, "model = btrim(model)")
	for _, forbiddenColumn := range []string{"raw_body", "request_body", "full_text", "plaintext", "bearer_token", "api_key_value", "authorization"} {
		require.NotContains(t, sql, forbiddenColumn)
	}
}
