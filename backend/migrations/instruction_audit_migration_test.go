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

func TestInstructionAuditGroupScopeMigrationIsAdditiveAndIdempotent(t *testing.T) {
	body, err := FS.ReadFile("199_instruction_audit_group_scope.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "create table if not exists instruction_audit_group_bindings")
	require.Contains(t, sql, "group_id          bigint not null references groups(id)")
	require.Contains(t, sql, "constraint uq_instruction_audit_group_binding unique (group_id, rule_set_id)")
	require.Contains(t, sql, "add column if not exists group_id")
	require.Contains(t, sql, "add column if not exists group_name_snapshot")
	require.Contains(t, sql, "set value = 'false'")
	require.Contains(t, sql, "not exists (select 1 from instruction_audit_group_bindings)")
	for _, destructive := range []string{
		"drop table instruction_audit_bindings",
		"delete from instruction_audit_bindings",
		"update instruction_audit_bindings",
		"truncate instruction_audit_bindings",
	} {
		require.NotContains(t, sql, destructive)
	}
}

func TestInstructionAuditReviewMigrationEncryptsEvidenceAndSplitsNotifications(t *testing.T) {
	body, err := FS.ReadFile("200_instruction_audit_review_notifications.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "('instruction_audit_evidence_retention_days', '30'")
	require.Contains(t, sql, "create table if not exists instruction_audit_evidence")
	require.Contains(t, sql, "ciphertext        bytea not null")
	require.Contains(t, sql, "create table if not exists instruction_audit_evidence_access_logs")
	require.Contains(t, sql, "create table if not exists security_notification_outbox")
	require.Contains(t, sql, "audience in ('user', 'ops')")
	require.Contains(t, sql, "'suppressed', 'no_recipient'")
	require.Contains(t, sql, "create unique index if not exists uq_security_notification_dedup_active")
	require.NotContains(t, sql, "plaintext text")
	require.NotContains(t, sql, "request_body")
	require.NotContains(t, sql, "bearer_token")
}

func TestInstructionAuditClientScopeMigrationPreservesExistingCoverage(t *testing.T) {
	body, err := FS.ReadFile("201_instruction_audit_client_scope.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "add column if not exists client_types")
	require.Contains(t, sql, "default array['all']::text[]")
	require.Contains(t, sql, "'codex_vscode', 'codex_cli', 'codex_desktop'")
	require.Contains(t, sql, "'opencode', 'modelport_internal', 'other', 'unknown'")
	require.Contains(t, sql, "add column if not exists client_type")
	require.Contains(t, sql, "add column if not exists client_user_agent")
	require.NotContains(t, sql, "drop table")
	require.NotContains(t, sql, "truncate")
}
