package migrations

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelPortInstructionAuditBridgeDeclaresRuntimeSchema(t *testing.T) {
	body, err := FS.ReadFile("234_modelport_instruction_audit_bridge.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))

	for _, table := range []string{
		"instruction_audit_state",
		"instruction_audit_hashes",
		"instruction_audit_rule_sets",
		"instruction_audit_rule_set_hashes",
		"instruction_audit_group_bindings",
		"instruction_audit_events",
		"instruction_audit_evidence",
		"instruction_audit_reason_policies",
		"instruction_audit_runtime_config",
		"instruction_audit_hash_raw_contents",
		"instruction_audit_ai_reviews",
		"instruction_audit_hash_sources",
		"instruction_audit_sensitive_access_grants",
		"instruction_audit_sensitive_access_logs",
		"instruction_audit_translation_jobs",
		"instruction_audit_outcome_hourly",
		"instruction_audit_operational_counters",
		"security_notification_outbox",
		"instruction_audit_v2_config",
		"instruction_audit_v2_ai_nodes",
		"instruction_audit_v2_client_profiles",
		"instruction_audit_v2_scopes",
		"instruction_audit_v2_user_allowlist",
		"instruction_audit_v2_hashes",
		"instruction_audit_v2_hash_scopes",
		"instruction_audit_v2_events",
		"instruction_audit_v2_event_evidence",
		"instruction_audit_v2_ai_reviews",
		"instruction_audit_v2_raw_access_logs",
		"instruction_audit_v2_content_vault",
		"instruction_audit_v2_risk_hashes",
		"instruction_audit_v2_review_jobs",
		"instruction_audit_v2_review_attempts",
		"content_moderation_cyber_evidence",
	} {
		require.Contains(t, sql, "create table if not exists "+table, table)
	}

	for _, fragment := range []string{
		"add column if not exists allow_empty_fields",
		"add column if not exists async_retry_schedule_seconds",
		"add column if not exists slot",
		"add column if not exists response_mode",
		"add column if not exists max_output_tokens",
		"add column if not exists prompt_audit_enabled",
		"add column if not exists global_trust",
		"add column if not exists content_vault_id",
		"add column if not exists source_user_id",
		"add column if not exists selected_field",
		"add column if not exists review_job_id",
		"add column if not exists audit_source",
		"'risk_hash_blocked'",
		"'ai_review_pending'",
		"'timeout', 'invalid'",
	} {
		require.Contains(t, sql, fragment)
	}
}

func TestModelPortInstructionAuditBridgeIsAdditiveAndActiveOnly(t *testing.T) {
	body, err := FS.ReadFile("234_modelport_instruction_audit_bridge.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))

	active, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)
	require.Contains(t, active, "234_modelport_instruction_audit_bridge.sql")

	destructive := regexp.MustCompile(`(?mi)^\s*(update|delete\s+from|truncate|drop\s+table|drop\s+index)\b`)
	require.Empty(t, destructive.FindAllString(sql, -1))
	require.NotContains(t, sql, " concurrently ")
	require.NotContains(t, sql, "do update set")

	for _, fragment := range []string{
		"198_instruction_audit.sql",
		"217_instruction_audit_v2.sql",
		"219_instruction_audit_v2_review_pipeline.sql",
		"224_prompt_audit_instruction_patch.sql",
		"modelport_legacy/",
		"insert into instruction_audit_v2_content_vault",
		"insert into instruction_audit_hash_raw_contents",
		"insert into security_notification_outbox",
	} {
		require.NotContains(t, sql, fragment)
	}

	// Existing ciphertext and its linkage are schema declarations only. The
	// bridge must never move, rewrite, or normalize encrypted audit material.
	require.NotRegexp(t, regexp.MustCompile(`(?is)alter\s+table\s+\w+\s+alter\s+column\s+\w*(ciphertext|key_version)`), sql)
}
