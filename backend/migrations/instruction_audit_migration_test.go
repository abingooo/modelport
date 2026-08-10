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

func TestInstructionAuditLegacyAggregateRepartitionMigrationIsBounded(t *testing.T) {
	body, err := FS.ReadFile("211_instruction_audit_repartition_legacy_aggregates.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "where shard_no = -1")
	require.Contains(t, sql, "generate_series")
	require.Contains(t, sql, "cardinality(event_times) <= 4096")
	require.Contains(t, sql, "check (shard_no >= 0)")
	require.Contains(t, sql, "sum(event_count)")
	require.Contains(t, sql, "sum(latency_total_ms)")
	require.Contains(t, sql, "sum(ai_latency_total_ms)")
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditHashScopeLifecycleMigrationSeparatesGrantExpiry(t *testing.T) {
	body, err := FS.ReadFile("212_instruction_audit_hash_scope_lifecycle.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "add column if not exists source_type")
	require.Contains(t, sql, "add column if not exists valid_until")
	require.Contains(t, sql, "add column if not exists status")
	require.Contains(t, sql, "add column if not exists updated_by")
	require.Contains(t, sql, "source_type in ('manual', 'ai_review')")
	require.Contains(t, sql, "source_type <> 'ai_review' or valid_until is not null")
	require.Contains(t, sql, "status in ('active', 'disabled', 'revoked')")
	require.Contains(t, sql, "resource_type in ('event_evidence', 'hash_raw', 'translation', 'ai_hash', 'ai_scope')")
	require.Contains(t, sql, "scope_rule_set_id")
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditSensitiveAccessMigrationBootstrapsOneAdminAndLinksAuditRows(t *testing.T) {
	body, err := FS.ReadFile("213_instruction_audit_sensitive_access_grants.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "create table if not exists instruction_audit_sensitive_access_grants")
	require.Contains(t, sql, "uq_instruction_audit_sensitive_active_grant")
	require.Contains(t, sql, "grant_source in ('setup_bootstrap', 'migration_bootstrap', 'manual', 'emergency_cli')")
	require.Contains(t, sql, "add column if not exists grant_id")
	require.Contains(t, sql, "add column if not exists auth_method")
	require.Contains(t, sql, "add column if not exists authorization_result")
	require.Contains(t, sql, "add column if not exists authorized_grant_id")
	require.Contains(t, sql, "and not exists (select 1 from instruction_audit_sensitive_access_grants)")
	require.Contains(t, sql, "order by u.created_at asc, u.id asc")
	require.Contains(t, sql, "limit 1")
	require.NotContains(t, sql, "raw_content")
	require.NotContains(t, sql, "request_body")
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditNotificationIntentMigrationDistinguishesEnqueueFailure(t *testing.T) {
	body, err := FS.ReadFile("214_instruction_audit_notification_intents.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "enqueue_failed")
	require.Contains(t, sql, "drop constraint if exists chk_security_notification_status")
	require.NotContains(t, sql, "truncate")
}

func TestInstructionAuditBodyWorkingSetBudgetMigrationRaisesLegacyValues(t *testing.T) {
	body, err := FS.ReadFile("215_instruction_audit_body_working_set_budget.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "greatest(max_inflight_body_bytes, max_body_bytes * 3)")
	require.Contains(t, sql, "max_inflight_body_bytes between max_body_bytes * 3 and 2147483648")
	require.Contains(t, sql, "drop constraint if exists chk_instruction_audit_runtime_inflight_limit")
	require.NotContains(t, sql, "truncate")
	require.NotContains(t, sql, "delete from")
}

func TestInstructionAuditOperationalCountersMigrationIsAdditiveAndNormalizesRawExpiry(t *testing.T) {
	body, err := FS.ReadFile("216_instruction_audit_operational_counters.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "create table if not exists instruction_audit_operational_counters")
	require.Contains(t, sql, "persist_failure_count")
	require.Contains(t, sql, "statistics_loss_count")
	require.Contains(t, sql, "set raw_content_status = 'raw_content_unavailable'")
	require.NotContains(t, sql, "truncate")
	require.NotContains(t, sql, "delete from")
}

func TestInstructionAuditV2MigrationIsIsolatedAndSupportsNotifications(t *testing.T) {
	body, err := FS.ReadFile("217_instruction_audit_v2.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "values ('instruction_audit_enabled', 'false', now())")
	require.Contains(t, sql, "'instruction_audit_v2', 'cyber_policy'")
	require.Contains(t, sql, "drop constraint if exists chk_security_notification_source")
	require.Contains(t, sql, "references instruction_audit_v2_client_profiles(id) on delete restrict")
	require.Contains(t, sql, "ai_global_concurrency   int not null default 64")
	require.Contains(t, sql, "max_concurrency     int not null default 16")
	require.NotContains(t, sql, "drop table")
	require.NotContains(t, sql, "truncate")
	require.NotContains(t, sql, "delete from instruction_audit_")
}

func TestInstructionAuditV2ReviewPipelineMigrationReplacesCandidates(t *testing.T) {
	body, err := FS.ReadFile("219_instruction_audit_v2_review_pipeline.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(body))
	require.Contains(t, sql, "create table if not exists instruction_audit_v2_content_vault")
	require.Contains(t, sql, "create table if not exists instruction_audit_v2_risk_hashes")
	require.Contains(t, sql, "create table if not exists instruction_audit_v2_review_jobs")
	require.Contains(t, sql, "create table if not exists instruction_audit_v2_review_attempts")
	require.Contains(t, sql, "add column if not exists review_criteria")
	require.Contains(t, sql, "add column if not exists observe_only")
	require.Contains(t, sql, "'sync', 'async_1', 'async_2', 'async_3'")
	require.Contains(t, sql, "delete from instruction_audit_v2_hashes where status = 'candidate'")
	require.NotContains(t, sql, "truncate")
}
