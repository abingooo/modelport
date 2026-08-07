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

func TestInstructionAuditRuleExceptionsMigrationIsAdditive(t *testing.T) {
	body, err := FS.ReadFile("203_instruction_audit_rule_exceptions.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "add column if not exists allow_empty_fields")
	require.Contains(t, sql, "create table if not exists instruction_audit_rule_set_users")
	require.Contains(t, sql, "primary key (rule_set_id, user_id)")
	require.Contains(t, sql, "references instruction_audit_rule_sets(id) on delete cascade")
	require.Contains(t, sql, "references users(id) on delete cascade")
	require.NotContains(t, sql, "drop table")
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditOutcomesMigrationPreservesBlockedHistory(t *testing.T) {
	body, err := FS.ReadFile("204_instruction_audit_outcomes_and_policies.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "add column if not exists final_outcome")
	require.Contains(t, sql, "initial_reason = coalesce(initial_reason, reason)")
	require.Contains(t, sql, "create or replace function instruction_audit_event_v13_compat")
	require.Contains(t, sql, "create table if not exists instruction_audit_reason_policies")
	require.Contains(t, sql, "'policy_allow', 'ai_pass', 'hash_pass', 'exception_pass'")
	require.Contains(t, sql, "reason not in ('config_unavailable', 'ai_error') or action = 'block'")
	require.NotContains(t, sql, "delete from instruction_audit_events")
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditRawAITranslationMigrationStoresOnlyCiphertext(t *testing.T) {
	body, err := FS.ReadFile("205_instruction_audit_raw_ai_translation.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "max_body_bytes                  bigint not null default 67108864")
	require.Contains(t, sql, "ai_enabled                      boolean not null default false")
	require.Contains(t, sql, "translation_enabled             boolean not null default false")
	require.Contains(t, sql, "create table if not exists instruction_audit_hash_raw_contents")
	require.Contains(t, sql, "ciphertext              bytea")
	require.Contains(t, sql, "raw_content_unavailable")
	require.Contains(t, sql, "create table if not exists instruction_audit_ai_reviews")
	require.Contains(t, sql, "create table if not exists instruction_audit_hash_sources")
	require.Contains(t, sql, "create table if not exists instruction_audit_sensitive_access_logs")
	require.Contains(t, sql, "create table if not exists instruction_audit_translation_jobs")
	for _, forbidden := range []string{"raw_content text", "translation_text", "bearer_token", "api_key_value", "request_body"} {
		require.NotContains(t, sql, forbidden)
	}
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditOutcomeAggregationMigrationIsAdditive(t *testing.T) {
	body, err := FS.ReadFile("206_instruction_audit_outcome_aggregation.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "create table if not exists instruction_audit_outcome_hourly")
	require.Contains(t, sql, "create table if not exists instruction_audit_outcome_rollup_state")
	require.Contains(t, sql, "last_event_id         bigint not null default 0")
	require.Contains(t, sql, "event_count           bigint not null default 0")
	require.Contains(t, sql, "event_times           timestamptz[] not null")
	require.NotContains(t, sql, "delete from instruction_audit_events")
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditEventIndexesAreConcurrent(t *testing.T) {
	body, err := FS.ReadFile("207_instruction_audit_v13_event_indexes_notx.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Equal(t, 4, strings.Count(sql, "create index concurrently if not exists"))
	require.Contains(t, sql, "idx_instruction_audit_events_pass_cleanup")
	require.Contains(t, sql, "where final_outcome in ('hash_pass', 'exception_pass')")
}

func TestInstructionAuditTranslationExecutionMigrationStoresNoTranslationText(t *testing.T) {
	body, err := FS.ReadFile("208_instruction_audit_translation_execution.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "claim_version")
	require.Contains(t, sql, "processing_started_at")
	require.Contains(t, sql, "redaction_count")
	require.Contains(t, sql, "provider_latency_ms")
	require.NotContains(t, sql, "translated_text")
	require.NotContains(t, sql, "raw_content")
}

func TestInstructionAuditAggregateRetentionMigrationIsAdditive(t *testing.T) {
	body, err := FS.ReadFile("209_instruction_audit_aggregate_retention.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "add column if not exists aggregate_retention_days")
	require.Contains(t, sql, "default 365")
	require.Contains(t, sql, "expired_aggregate_event_count")
	require.Contains(t, sql, "last_aggregate_pruned_at")
	require.NotContains(t, sql, "delete from")
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditAggregateShardMigrationBoundsGrowingArrays(t *testing.T) {
	body, err := FS.ReadFile("210_instruction_audit_aggregate_shards.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "add column if not exists shard_no")
	require.Contains(t, sql, "final_outcome, final_reason, shard_no")
	require.Contains(t, sql, "chk_instruction_audit_outcome_hourly_shard")
	require.NotContains(t, sql, "delete from")
	require.NotContains(t, sql, "truncate")
}
