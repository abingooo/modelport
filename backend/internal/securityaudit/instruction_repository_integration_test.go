package securityaudit

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	modelportrepository "github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const instructionAuditPostgresTestEnv = "INSTRUCTION_AUDIT_TEST_POSTGRES_DSN"

func openInstructionAuditIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openInstructionAuditSchema(t)
	migrations := []string{
		"198_instruction_audit.sql",
		"199_instruction_audit_group_scope.sql",
		"200_instruction_audit_review_notifications.sql",
		"201_instruction_audit_client_scope.sql",
		"203_instruction_audit_rule_exceptions.sql",
		"204_instruction_audit_outcomes_and_policies.sql",
		"205_instruction_audit_raw_ai_translation.sql",
		"206_instruction_audit_outcome_aggregation.sql",
		"208_instruction_audit_translation_execution.sql",
		"209_instruction_audit_aggregate_retention.sql",
		"210_instruction_audit_aggregate_shards.sql",
	}
	for iteration := range 2 {
		for _, name := range migrations {
			applyInstructionAuditMigration(t, db, name)
			if iteration == 0 && name == "198_instruction_audit.sql" {
				_, err := db.Exec(`UPDATE settings SET value = 'true' WHERE key = 'instruction_audit_enabled'`)
				require.NoError(t, err)
			}
		}
	}
	return db
}

func openInstructionAuditSchema(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(instructionAuditPostgresTestEnv))
	if dsn == "" {
		t.Skip(instructionAuditPostgresTestEnv + " is not set")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	schema := fmt.Sprintf("instruction_audit_test_%d", time.Now().UnixNano())
	_, err = db.ExecContext(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, dropErr := db.ExecContext(cleanupCtx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		require.NoError(t, dropErr)
		require.NoError(t, db.Close())
	})
	_, err = db.ExecContext(ctx, `SET search_path TO `+schema)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS groups (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			platform TEXT NOT NULL DEFAULT 'openai',
			status TEXT NOT NULL DEFAULT 'active',
			deleted_at TIMESTAMPTZ
		);
		CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			email TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'user',
			status TEXT NOT NULL DEFAULT 'active',
			deleted_at TIMESTAMPTZ
		);
		CREATE TABLE IF NOT EXISTS api_keys (id BIGSERIAL PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS settings (
			key VARCHAR(255) PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	require.NoError(t, err)
	return db
}

func applyInstructionAuditMigration(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	require.NoError(t, err)
	if strings.HasSuffix(name, "_notx.sql") {
		for _, statement := range strings.Split(string(body), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			_, err = db.Exec(statement)
			require.NoError(t, err, name)
		}
		return
	}
	_, err = db.Exec(string(body))
	require.NoError(t, err, name)
}

func instructionAuditTableCount(t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&count))
	return count
}

func TestInstructionAuditPostgresV13MigrationPreservesV12DataAndIsIdempotent(t *testing.T) {
	db := openInstructionAuditSchema(t)
	for _, name := range []string{
		"198_instruction_audit.sql",
		"199_instruction_audit_group_scope.sql",
		"200_instruction_audit_review_notifications.sql",
		"201_instruction_audit_client_scope.sql",
		"203_instruction_audit_rule_exceptions.sql",
	} {
		applyInstructionAuditMigration(t, db, name)
	}

	adminID := insertInstructionAuditUser(t, db, "migration-admin@example.test", "admin")
	groupID := insertInstructionAuditGroup(t, db, "Migration Group")
	var apiKeyID int64
	require.NoError(t, db.QueryRow(`INSERT INTO api_keys DEFAULT VALUES RETURNING id`).Scan(&apiKeyID))
	digest := sha256Hex("migration-standard")
	var hashID, ruleSetID, eventID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_hashes
			(digest, name, observed_source, status, created_by)
		VALUES ($1, 'migration hash', 'instructions', 'active', $2)
		RETURNING id`, digest, adminID).Scan(&hashID))
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_rule_sets (name, enabled, created_by, updated_by)
		VALUES ('migration rules', TRUE, $1, $1) RETURNING id`, adminID).Scan(&ruleSetID))
	_, err := db.Exec(`INSERT INTO instruction_audit_rule_set_hashes (rule_set_id, hash_id, created_by) VALUES ($1, $2, $3)`, ruleSetID, hashID, adminID)
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO instruction_audit_group_bindings
			(group_id, rule_set_id, client_types, enabled, created_by, updated_by)
		VALUES ($1, $2, ARRAY['codex_cli']::TEXT[], TRUE, $3, $3)`, groupID, ruleSetID, adminID)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_events
			(request_id, user_id, user_email_snapshot, api_key_id, group_id, group_name_snapshot,
			 client_type, client_user_agent, model, endpoint, stage,
			 instructions_present, instructions_sha256, instructions_result,
			 input1_present, input1_sha256, input1_result,
			 decision, reason, rule_set_ids, config_version, latency_ms, evidence_status)
		VALUES
			('migration-request', $1, 'migration-admin@example.test', $2, $3, 'Migration Group',
			 'codex_cli', 'codex_cli_rs/0.145.0', 'gpt-test', '/v1/responses', 'http',
			 TRUE, $4, 'mismatch', FALSE, '', 'missing',
			 'blocked', 'hash_mismatch', jsonb_build_array($5::BIGINT), 7, 3, 'stored')
		RETURNING id`, adminID, apiKeyID, groupID, digest, ruleSetID).Scan(&eventID))
	_, err = db.Exec(`
		INSERT INTO instruction_audit_evidence
			(event_id, source, digest, ciphertext, key_version, plaintext_bytes, expires_at)
		VALUES ($1, 'instructions', $2, decode('010203', 'hex'), 'legacy-v1', 18, NOW() + INTERVAL '30 days')`, eventID, digest)
	require.NoError(t, err)
	for _, audience := range []string{"user", "ops"} {
		_, err = db.Exec(`
			INSERT INTO security_notification_outbox
				(source_type, source_id, audience, user_id, template_event, status)
			VALUES ('instruction_audit', $1, $2, $3, $4, 'sent')`,
			eventID, audience, adminID, "instruction_audit."+audience+"_notice")
		require.NoError(t, err)
	}
	_, err = db.Exec(`
		INSERT INTO instruction_audit_evidence_access_logs
			(event_id, actor_id, action, source, succeeded)
		VALUES ($1, $2, 'reveal', 'instructions', TRUE)`, eventID, adminID)
	require.NoError(t, err)

	preserved := map[string]int64{}
	for _, table := range []string{
		"instruction_audit_events", "instruction_audit_hashes", "instruction_audit_rule_sets",
		"instruction_audit_rule_set_hashes", "instruction_audit_group_bindings",
		"instruction_audit_evidence", "instruction_audit_evidence_access_logs", "security_notification_outbox",
	} {
		preserved[table] = instructionAuditTableCount(t, db, table)
	}

	for iteration := range 2 {
		for _, name := range []string{
			"204_instruction_audit_outcomes_and_policies.sql",
			"205_instruction_audit_raw_ai_translation.sql",
			"206_instruction_audit_outcome_aggregation.sql",
			"207_instruction_audit_v13_event_indexes_notx.sql",
			"208_instruction_audit_translation_execution.sql",
			"209_instruction_audit_aggregate_retention.sql",
			"210_instruction_audit_aggregate_shards.sql",
		} {
			applyInstructionAuditMigration(t, db, name)
		}
		for table, expected := range preserved {
			require.Equal(t, expected, instructionAuditTableCount(t, db, table), "%s iteration %d", table, iteration)
		}
	}

	var initialReason, finalReason, outcome, action string
	var bodyBytes, aiLatency sql.NullInt64
	require.NoError(t, db.QueryRow(`
		SELECT initial_reason, final_reason, final_outcome, policy_action, body_bytes, ai_latency_ms
		FROM instruction_audit_events WHERE id = $1`, eventID).Scan(
		&initialReason, &finalReason, &outcome, &action, &bodyBytes, &aiLatency,
	))
	require.Equal(t, "hash_mismatch", initialReason)
	require.Equal(t, "hash_mismatch", finalReason)
	require.Equal(t, InstructionOutcomeBlocked, outcome)
	require.Equal(t, InstructionPolicyActionBlock, action)
	require.False(t, bodyBytes.Valid)
	require.False(t, aiLatency.Valid)

	var rawStatus, sourceType string
	require.NoError(t, db.QueryRow(`SELECT raw_content_status FROM instruction_audit_hash_raw_contents WHERE hash_id = $1`, hashID).Scan(&rawStatus))
	require.NoError(t, db.QueryRow(`SELECT source_type FROM instruction_audit_hash_sources WHERE hash_id = $1`, hashID).Scan(&sourceType))
	require.Equal(t, "raw_content_unavailable", rawStatus)
	require.Equal(t, "import", sourceType)
	require.EqualValues(t, 13, instructionAuditTableCount(t, db, "instruction_audit_reason_policies"))

	var maxBodyBytes int64
	var aggregateRetentionDays int
	var aiEnabled, translationEnabled bool
	require.NoError(t, db.QueryRow(`
		SELECT max_body_bytes, aggregate_retention_days, ai_enabled, translation_enabled
		FROM instruction_audit_runtime_config WHERE id = 1`).Scan(
		&maxBodyBytes, &aggregateRetentionDays, &aiEnabled, &translationEnabled,
	))
	require.EqualValues(t, InstructionDefaultMaxBodyBytes, maxBodyBytes)
	require.Equal(t, 365, aggregateRetentionDays)
	require.False(t, aiEnabled)
	require.False(t, translationEnabled)

	var rollingEventID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_events
			(request_id, decision, reason, config_version, latency_ms)
		VALUES ('rolling-v12-write', 'blocked', 'fields_missing', 7, 1)
		RETURNING id`).Scan(&rollingEventID))
	var rollingOutcome, rollingReason string
	require.NoError(t, db.QueryRow(`
		SELECT final_outcome, final_reason FROM instruction_audit_events WHERE id = $1`, rollingEventID).Scan(&rollingOutcome, &rollingReason))
	require.Equal(t, InstructionOutcomeBlocked, rollingOutcome)
	require.Equal(t, "fields_missing", rollingReason)
}

func TestInstructionAuditPostgresArchivesPassEventsWithoutDoubleCounting(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "statistics-user@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "Statistics Group")
	archivedAt := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour).Add(10 * time.Minute)

	type outcomeCase struct {
		outcome string
		reason  string
		action  string
	}
	cases := []outcomeCase{
		{InstructionOutcomeBlocked, "hash_mismatch", InstructionPolicyActionBlock},
		{InstructionOutcomePolicyAllow, "hash_mismatch", InstructionPolicyActionAllowAndRecord},
		{InstructionOutcomeAIPass, "instructions_match", "ai_review"},
		{InstructionOutcomeHashPass, "instructions_match", "hash_match"},
		{InstructionOutcomeExceptionPass, "user_allowlist", "exception"},
	}
	eventIDs := make(map[string]int64, len(cases))
	for _, item := range cases {
		decision := &InstructionDecision{
			Allow: item.outcome != InstructionOutcomeBlocked, InitialReason: item.reason,
			FinalReason: item.reason, FinalOutcome: item.outcome, PolicyAction: item.action,
			Instructions: InstructionFieldResult{Present: true, SHA256: sha256Hex(item.outcome), Result: "match"},
			Input1:       InstructionFieldResult{Result: "not_checked"}, ConfigVersion: 9,
			BodyBytes: 1024, Latency: 2 * time.Millisecond,
		}
		eventID, err := repository.RecordEvent(ctx, Request{
			RequestID: "statistics-" + item.outcome, UserID: userID,
			UserEmail: "statistics-user@example.test", GroupID: &groupID,
			GroupName: "Statistics Group", InstructionClientType: InstructionClientCodexCLI,
			UserAgent: "codex_cli_rs/0.145.0", Model: "gpt-statistics", Endpoint: "/v1/responses", Stage: "http",
		}, decision, "not_available", nil, nil)
		require.NoError(t, err)
		eventIDs[item.outcome] = eventID
		_, err = db.Exec(`UPDATE instruction_audit_events SET created_at = $1 WHERE id = $2`, archivedAt, eventID)
		require.NoError(t, err)
	}
	var deterministicAILatency sql.NullInt64
	require.NoError(t, db.QueryRow(`
		SELECT ai_latency_ms FROM instruction_audit_events WHERE id = $1`,
		eventIDs[InstructionOutcomeHashPass],
	).Scan(&deterministicAILatency))
	require.False(t, deterministicAILatency.Valid)
	_, err := db.Exec(`UPDATE instruction_audit_events SET ai_latency_ms = 25 WHERE id = $1`, eventIDs[InstructionOutcomeAIPass])
	require.NoError(t, err)

	archived, err := repository.ArchiveExpiredPassEvents(ctx, 1, 100)
	require.NoError(t, err)
	require.EqualValues(t, 2, archived)
	archived, err = repository.ArchiveExpiredPassEvents(ctx, 1, 100)
	require.NoError(t, err)
	require.Zero(t, archived)

	from := archivedAt.Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	statistics, err := repository.InstructionStatistics(ctx, InstructionEventFilter{
		From: &from, To: &to, GroupIDs: []int64{groupID}, UserID: userID,
		Model: "gpt-stat", ClientTypes: []string{InstructionClientCodexCLI},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, statistics.Blocked)
	require.EqualValues(t, 1, statistics.PolicyAllow)
	require.EqualValues(t, 1, statistics.AIPass)
	require.EqualValues(t, 1, statistics.HashPass)
	require.EqualValues(t, 1, statistics.ExceptionPass)
	require.EqualValues(t, 5, statistics.Total)
	require.InDelta(t, 0.2, statistics.BlockRate, 0.0001)

	persisted, aggregated, err := repository.InstructionOutcomeStorageCounts(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, persisted)
	require.EqualValues(t, 2, aggregated)
	var archiveWatermark int64
	require.NoError(t, db.QueryRow(`SELECT last_event_id FROM instruction_audit_outcome_rollup_state WHERE id = 1`).Scan(&archiveWatermark))
	require.Equal(t, eventIDs[InstructionOutcomeExceptionPass], archiveWatermark)

	filtered, err := repository.InstructionStatistics(ctx, InstructionEventFilter{
		From: &from, To: &to, Outcomes: []string{InstructionOutcomeHashPass},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, filtered.HashPass)
	require.EqualValues(t, 1, filtered.Total)

	filtered, err = repository.InstructionStatistics(ctx, InstructionEventFilter{
		From: &from, To: &to, FinalReasons: []string{"hash_mismatch"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, filtered.Blocked)
	require.EqualValues(t, 1, filtered.PolicyAllow)
	require.EqualValues(t, 2, filtered.Total)

	partialFrom := archivedAt.Add(time.Minute)
	partialTo := archivedAt.Add(20 * time.Minute)
	partial, err := repository.InstructionStatistics(ctx, InstructionEventFilter{
		From: &partialFrom, To: &partialTo,
		Outcomes: []string{InstructionOutcomeHashPass, InstructionOutcomeExceptionPass},
	})
	require.NoError(t, err)
	require.Zero(t, partial.Total)

	matchingFrom := archivedAt.Add(-time.Minute)
	matchingTo := archivedAt.Add(time.Minute)
	partial, err = repository.InstructionStatistics(ctx, InstructionEventFilter{
		From: &matchingFrom, To: &matchingTo,
		Outcomes: []string{InstructionOutcomeHashPass, InstructionOutcomeExceptionPass},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, partial.HashPass)
	require.EqualValues(t, 1, partial.ExceptionPass)
	require.EqualValues(t, 2, partial.Total)
	var archivedEventTimes int
	require.NoError(t, db.QueryRow(`
		SELECT COALESCE(SUM(cardinality(event_times)), 0)
		FROM instruction_audit_outcome_hourly`).Scan(&archivedEventTimes))
	require.Equal(t, 2, archivedEventTimes)

	latency, err := repository.InstructionLatencyMetrics(ctx, from)
	require.NoError(t, err)
	require.EqualValues(t, 2, latency.AuditSampleCount)
	require.EqualValues(t, 2, latency.AuditP95MS)
	require.EqualValues(t, 2, latency.AuditP99MS)
	require.EqualValues(t, 1, latency.AISampleCount)
	require.EqualValues(t, 25, latency.AIP95MS)
	require.EqualValues(t, 25, latency.AIP99MS)

	result, err := repository.DeleteEventsByIDs(ctx, []int64{eventIDs[InstructionOutcomeBlocked]})
	require.NoError(t, err)
	require.EqualValues(t, 1, result.DeletedEvents)
	statistics, err = repository.InstructionStatistics(ctx, InstructionEventFilter{From: &from, To: &to})
	require.NoError(t, err)
	require.Zero(t, statistics.Blocked)
	require.EqualValues(t, 4, statistics.Total)

	expiredBucket := time.Now().UTC().Add(-400 * 24 * time.Hour).Truncate(time.Hour)
	_, err = db.Exec(`
		UPDATE instruction_audit_outcome_hourly
		SET bucket_at = $1::TIMESTAMPTZ,
			event_times = ARRAY[$1::TIMESTAMPTZ + INTERVAL '10 minutes'],
			first_event_at = $1::TIMESTAMPTZ + INTERVAL '10 minutes',
			last_event_at = $1::TIMESTAMPTZ + INTERVAL '10 minutes'`, expiredBucket)
	require.NoError(t, err)
	deletedRows, deletedEvents, err := repository.PruneExpiredOutcomeAggregates(ctx, 365, 100)
	require.NoError(t, err)
	require.EqualValues(t, 2, deletedRows)
	require.EqualValues(t, 2, deletedEvents)
	deletedRows, deletedEvents, err = repository.PruneExpiredOutcomeAggregates(ctx, 365, 100)
	require.NoError(t, err)
	require.Zero(t, deletedRows)
	require.Zero(t, deletedEvents)
	expiredCount, err := repository.ExpiredAggregateEventCount(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, expiredCount)
}

func TestInstructionAuditPostgresBoundsAggregateTimestampShards(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	ctx := context.Background()
	archivedAt := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	eventCount := instructionOutcomeAggregateShardSize + 1

	result, err := db.Exec(`
		INSERT INTO instruction_audit_events
			(request_id, decision, reason, initial_reason, final_reason, final_outcome,
			 policy_action, created_at)
		SELECT 'aggregate-shard-' || value::TEXT, 'hash_pass', 'instructions_match',
			'instructions_match', 'instructions_match', 'hash_pass', 'hash_match', $2
		FROM generate_series(1, $1) AS value`, eventCount, archivedAt)
	require.NoError(t, err)
	inserted, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, eventCount, inserted)

	archived, err := repository.ArchiveExpiredPassEvents(ctx, 1, 5000)
	require.NoError(t, err)
	require.Equal(t, eventCount, archived)
	var rows, maxEventsPerShard, totalEvents int64
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*), COALESCE(MAX(cardinality(event_times)), 0), COALESCE(SUM(event_count), 0)
		FROM instruction_audit_outcome_hourly`).Scan(&rows, &maxEventsPerShard, &totalEvents))
	require.EqualValues(t, 2, rows)
	require.LessOrEqual(t, maxEventsPerShard, instructionOutcomeAggregateShardSize)
	require.Equal(t, eventCount, totalEvents)

	from, to := archivedAt.Add(-time.Minute), archivedAt.Add(time.Minute)
	statistics, err := repository.InstructionStatistics(ctx, InstructionEventFilter{
		From: &from, To: &to, Outcomes: []string{InstructionOutcomeHashPass},
	})
	require.NoError(t, err)
	require.Equal(t, eventCount, statistics.HashPass)
	require.Equal(t, eventCount, statistics.Total)
}

func TestInstructionAuditPostgresEventListPrioritizesActionableOutcomes(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	ctx := context.Background()
	baseTime := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

	type outcomeCase struct {
		outcome string
		reason  string
		action  string
		offset  time.Duration
	}
	cases := []outcomeCase{
		{InstructionOutcomeBlocked, "hash_mismatch", InstructionPolicyActionBlock, time.Minute},
		{InstructionOutcomePolicyAllow, "hash_mismatch", InstructionPolicyActionAllowAndRecord, 2 * time.Minute},
		{InstructionOutcomeAIPass, "instructions_match", "ai_review", 3 * time.Minute},
		{InstructionOutcomeExceptionPass, "user_allowlist", "exception", 4 * time.Minute},
		{InstructionOutcomeHashPass, "instructions_match", "hash_match", 5 * time.Minute},
	}
	for _, item := range cases {
		decision := &InstructionDecision{
			Allow: item.outcome != InstructionOutcomeBlocked, InitialReason: item.reason,
			FinalReason: item.reason, FinalOutcome: item.outcome, PolicyAction: item.action,
			Instructions: InstructionFieldResult{Present: true, SHA256: sha256Hex(item.outcome), Result: "match"},
			Input1:       InstructionFieldResult{Result: "not_checked"}, ConfigVersion: 3,
			BodyBytes: 256, Latency: 7 * time.Millisecond,
		}
		eventID, err := repository.RecordEvent(ctx, Request{
			RequestID: "event-order-" + item.outcome, InstructionClientType: InstructionClientCodexCLI,
			Model: "gpt-order", Endpoint: "/v1/responses", Stage: "http",
		}, decision, "not_available", nil, nil)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE instruction_audit_events SET created_at = $1 WHERE id = $2`, baseTime.Add(item.offset), eventID)
		require.NoError(t, err)
	}

	page, err := repository.ListEvents(ctx, 1, 10, InstructionEventFilter{})
	require.NoError(t, err)
	require.Len(t, page.Items, 5)
	require.Equal(t, []string{
		InstructionOutcomeAIPass,
		InstructionOutcomePolicyAllow,
		InstructionOutcomeBlocked,
		InstructionOutcomeHashPass,
		InstructionOutcomeExceptionPass,
	}, []string{
		page.Items[0].FinalOutcome,
		page.Items[1].FinalOutcome,
		page.Items[2].FinalOutcome,
		page.Items[3].FinalOutcome,
		page.Items[4].FinalOutcome,
	})
	for _, event := range page.Items {
		require.Equal(t, event.LatencyMS, event.AuditLatencyMS)
	}
}

func TestInstructionAuditPostgresCommitsAIPassWithExactTemporaryScope(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "ai-review-user@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "AI Review Group")
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("35", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)
	plaintext := "approved stable client instruction"
	digest := sha256Hex(plaintext)
	ciphertext, err := cipher.EncryptHashRaw(digest, plaintext)
	require.NoError(t, err)
	rawExpiresAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	automaticUntil := time.Now().UTC().Add(24 * time.Hour)
	decision := &InstructionDecision{
		Allow: true, InitialReason: "hash_mismatch", FinalReason: "instructions_match",
		FinalOutcome: InstructionOutcomeAIPass, PolicyAction: "ai_review",
		Instructions: InstructionFieldResult{
			Present: true, SHA256: digest, Result: "mismatch", Plaintext: plaintext,
		},
		Input1: InstructionFieldResult{Result: "missing"}, ConfigVersion: 1,
		BodyBytes: 4096, Latency: 3 * time.Millisecond, AILatency: 25 * time.Millisecond,
	}
	result, err := repository.CommitAIOutcome(ctx, instructionAIOutcomeCommit{
		Request: Request{
			RequestID: "ai-pass-request", UserID: userID, UserEmail: "ai-review-user@example.test",
			GroupID: &groupID, GroupName: "AI Review Group", InstructionClientType: InstructionClientCodexCLI,
			UserAgent: "codex_cli_rs/0.145.0", Model: "gpt-ai-review", Endpoint: "/v1/responses", Stage: "http",
		},
		Decision: decision, EvidenceStatus: "not_available",
		Attempts: []instructionAIReviewAttempt{{
			ReviewedSource: "instructions", ReviewedSHA256: digest, Result: "pass",
			ApprovedSource: "instructions", Confidence: 0.99, Reason: "stable template",
			ReviewerModel: "review-model", PromptVersion: "review-v1", LatencyMS: 25,
		}},
		FinalAttempt: 0,
		ApprovedRaw: &instructionHashRawStorage{
			Ciphertext: ciphertext, Status: "stored", ContentBytes: len([]byte(plaintext)),
			HashAlgorithm: InstructionHashAlgorithmSHA256,
			Normalization: InstructionHashNormalizationIdentityV1,
			KeyVersion:    instructionHashRawKeyVersion, ExpiresAt: &rawExpiresAt,
		},
		ApprovedField: decision.Instructions, AutomaticUntil: automaticUntil,
	})
	require.NoError(t, err)
	require.Positive(t, result.EventID)
	require.Positive(t, result.AIReviewID)
	require.NotNil(t, result.AutomaticHash)
	require.Equal(t, digest, result.AutomaticHash.Digest)
	require.Equal(t, "stored", result.AutomaticHash.RawStatus)
	require.Equal(t, "active", result.AutomaticHash.Status)
	require.NotNil(t, result.AutomaticHash.ValidUntil)
	require.EqualValues(t, 2, result.ConfigVersion)

	raw, err := repository.GetHashRaw(ctx, result.AutomaticHash.ID)
	require.NoError(t, err)
	revealed, err := cipher.DecryptHashRaw(digest, raw.Ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, revealed)

	reviews, err := repository.ListAIReviewsForEvent(ctx, result.EventID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	require.Equal(t, "pass", reviews[0].Result)
	require.Equal(t, result.AutomaticHash.ID, *reviews[0].AutomaticHashID)

	snapshot, err := repository.LoadSnapshot(ctx)
	require.NoError(t, err)
	policy, applicable := instructionPolicyFor(snapshot, groupID, InstructionClientCodexCLI)
	require.True(t, applicable)
	require.Len(t, policy.Hashes, 1)
	_, otherApplicable := instructionPolicyFor(snapshot, groupID, InstructionClientOpenCode)
	require.False(t, otherApplicable)
	var clientTypes []string
	require.NoError(t, db.QueryRow(`
		SELECT client_types FROM instruction_audit_group_bindings
		WHERE group_id = $1`, groupID).Scan(pq.Array(&clientTypes)))
	require.Equal(t, []string{InstructionClientCodexCLI}, clientTypes)

	var sourceEventID, sourceReviewID int64
	require.NoError(t, db.QueryRow(`
		SELECT event_id, ai_review_id FROM instruction_audit_hash_sources
		WHERE hash_id = $1 AND source_type = 'ai_review'`, result.AutomaticHash.ID).Scan(&sourceEventID, &sourceReviewID))
	require.Equal(t, result.EventID, sourceEventID)
	require.Equal(t, result.AIReviewID, sourceReviewID)

	renewedUntil := automaticUntil.Add(6 * time.Hour)
	renewedCiphertext, err := cipher.EncryptHashRaw(digest, plaintext)
	require.NoError(t, err)
	renewedDecision := *decision
	renewedDecision.AIReviewID = nil
	renewedDecision.RuleSetIDs = nil
	renewedResult, err := repository.CommitAIOutcome(ctx, instructionAIOutcomeCommit{
		Request: Request{
			RequestID: "ai-pass-renewal", UserID: userID, UserEmail: "ai-review-user@example.test",
			GroupID: &groupID, GroupName: "AI Review Group", InstructionClientType: InstructionClientCodexCLI,
			UserAgent: "codex_cli_rs/0.145.0", Model: "gpt-ai-review", Endpoint: "/v1/responses", Stage: "http",
		},
		Decision: &renewedDecision, EvidenceStatus: "not_available",
		Attempts: []instructionAIReviewAttempt{{
			ReviewedSource: "instructions", ReviewedSHA256: digest, Result: "pass",
			ApprovedSource: "instructions", Confidence: 0.99, Reason: "stable template",
			ReviewerModel: "review-model", PromptVersion: "review-v1", LatencyMS: 25,
		}},
		FinalAttempt: 0,
		ApprovedRaw: &instructionHashRawStorage{
			Ciphertext: renewedCiphertext, Status: "stored", ContentBytes: len([]byte(plaintext)),
			HashAlgorithm: InstructionHashAlgorithmSHA256,
			Normalization: InstructionHashNormalizationIdentityV1,
			KeyVersion:    instructionHashRawKeyVersion, ExpiresAt: &rawExpiresAt,
		},
		ApprovedField: decision.Instructions, AutomaticUntil: renewedUntil,
	})
	require.NoError(t, err)
	require.NotNil(t, renewedResult.AutomaticHash)
	require.Equal(t, result.AutomaticHash.ID, renewedResult.AutomaticHash.ID)
	require.NotNil(t, renewedResult.AutomaticHash.ValidUntil)
	require.WithinDuration(t, renewedUntil, *renewedResult.AutomaticHash.ValidUntil, time.Second)
	require.EqualValues(t, 3, renewedResult.ConfigVersion)
}

func TestInstructionAuditPostgresAIOutcomeRollsBackAtomically(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "ai-rollback-user@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "AI Rollback Group")
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("36", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)
	plaintext := "transaction must roll back"
	digest := sha256Hex(plaintext)
	ciphertext, err := cipher.EncryptHashRaw(digest, plaintext)
	require.NoError(t, err)
	rawExpiresAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	decision := &InstructionDecision{
		Allow: true, InitialReason: "hash_mismatch", FinalReason: "instructions_match",
		FinalOutcome: InstructionOutcomeAIPass, PolicyAction: "ai_review",
		Instructions: InstructionFieldResult{
			Present: true, SHA256: digest, Result: "mismatch", Plaintext: plaintext,
		},
		Input1: InstructionFieldResult{Result: "missing"}, ConfigVersion: 1,
	}
	_, err = db.Exec(`
		CREATE FUNCTION reject_instruction_audit_rollback_event() RETURNS TRIGGER AS $$
		BEGIN
			IF NEW.request_id = 'force-ai-rollback' THEN
				RAISE EXCEPTION 'forced AI transaction rollback';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER trg_reject_instruction_audit_rollback_event
		BEFORE INSERT ON instruction_audit_events
		FOR EACH ROW EXECUTE FUNCTION reject_instruction_audit_rollback_event()`)
	require.NoError(t, err)

	_, err = repository.CommitAIOutcome(ctx, instructionAIOutcomeCommit{
		Request: Request{
			RequestID: "force-ai-rollback", UserID: userID, GroupID: &groupID,
			InstructionClientType: InstructionClientCodexCLI, Model: "gpt-ai-review", Stage: "http",
		},
		Decision: decision, EvidenceStatus: "not_available",
		Attempts: []instructionAIReviewAttempt{{
			ReviewedSource: "instructions", ReviewedSHA256: digest, Result: "pass",
			ApprovedSource: "instructions", Confidence: 0.99, Reason: "stable",
			ReviewerModel: "review-model", PromptVersion: "review-v1",
		}},
		FinalAttempt: 0,
		ApprovedRaw: &instructionHashRawStorage{
			Ciphertext: ciphertext, Status: "stored", ContentBytes: len([]byte(plaintext)),
			HashAlgorithm: InstructionHashAlgorithmSHA256,
			Normalization: InstructionHashNormalizationIdentityV1,
			KeyVersion:    instructionHashRawKeyVersion, ExpiresAt: &rawExpiresAt,
		},
		ApprovedField: decision.Instructions, AutomaticUntil: time.Now().UTC().Add(24 * time.Hour),
	})
	require.ErrorContains(t, err, "forced AI transaction rollback")
	for _, table := range []string{
		"instruction_audit_events", "instruction_audit_ai_reviews", "instruction_audit_hashes",
		"instruction_audit_hash_raw_contents", "instruction_audit_hash_sources",
		"instruction_audit_rule_sets", "instruction_audit_rule_set_hashes", "instruction_audit_group_bindings",
	} {
		require.Zero(t, instructionAuditTableCount(t, db, table), table)
	}
	var configVersion int64
	require.NoError(t, db.QueryRow(`SELECT config_version FROM instruction_audit_state WHERE id = 1`).Scan(&configVersion))
	require.EqualValues(t, 1, configVersion)
}

func TestInstructionAuditPostgresAICannotExpandPermanentManualHashScope(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	ctx := context.Background()
	adminID := insertInstructionAuditUser(t, db, "ai-candidate-admin@example.test", "admin")
	userID := insertInstructionAuditUser(t, db, "ai-candidate-user@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "AI Candidate Group")
	plaintext := "permanent manual standard"
	digest := sha256Hex(plaintext)
	hash, err := repository.CreateHash(ctx, CreateInstructionHashRequest{
		Digest: digest, Name: "manual permanent", ObservedSource: "instructions", Status: "active",
	}, adminID)
	require.NoError(t, err)
	_, err = repository.SaveRuleSet(ctx, 0, SaveInstructionRuleSetRequest{
		Name: "ordinary rules", Enabled: true, HashIDs: []int64{hash.ID},
	}, adminID)
	require.NoError(t, err)
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("37", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)
	ciphertext, err := cipher.EncryptHashRaw(digest, plaintext)
	require.NoError(t, err)
	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	decision := &InstructionDecision{
		Allow: true, InitialReason: "hash_mismatch", FinalReason: "instructions_match",
		FinalOutcome: InstructionOutcomeAIPass, PolicyAction: "ai_review",
		Instructions: InstructionFieldResult{Present: true, SHA256: digest, Result: "mismatch", Plaintext: plaintext},
		Input1:       InstructionFieldResult{Result: "missing"}, ConfigVersion: 1,
	}
	_, err = repository.CommitAIOutcome(ctx, instructionAIOutcomeCommit{
		Request: Request{
			RequestID: "ai-permanent-scope-expansion", UserID: userID, GroupID: &groupID,
			InstructionClientType: InstructionClientCodexCLI, Model: "gpt-ai-review", Stage: "http",
		},
		Decision: decision, EvidenceStatus: "not_available",
		Attempts: []instructionAIReviewAttempt{{
			ReviewedSource: "instructions", ReviewedSHA256: digest, Result: "pass",
			ApprovedSource: "instructions", Confidence: 0.99, Reason: "stable",
			ReviewerModel: "review-model", PromptVersion: "review-v1",
		}},
		FinalAttempt: 0,
		ApprovedRaw: &instructionHashRawStorage{
			Ciphertext: ciphertext, Status: "stored", ContentBytes: len([]byte(plaintext)),
			HashAlgorithm: InstructionHashAlgorithmSHA256,
			Normalization: InstructionHashNormalizationIdentityV1,
			KeyVersion:    instructionHashRawKeyVersion, ExpiresAt: &expiresAt,
		},
		ApprovedField: decision.Instructions, AutomaticUntil: time.Now().UTC().Add(24 * time.Hour),
	})
	require.ErrorIs(t, err, errInstructionAIAutomaticHashUnavailable)
	unchanged, err := repository.GetHash(ctx, hash.ID)
	require.NoError(t, err)
	require.Equal(t, "active", unchanged.Status)
	require.Zero(t, instructionAuditTableCount(t, db, "instruction_audit_ai_reviews"))
	require.Zero(t, instructionAuditTableCount(t, db, "instruction_audit_events"))
	var systemRuleSets int64
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM instruction_audit_rule_sets WHERE system_managed`).Scan(&systemRuleSets))
	require.Zero(t, systemRuleSets)
}

func TestInstructionAuditPostgresConcurrentAIPassDeduplicatesScope(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "ai-concurrent-user@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "AI Concurrent Group")
	var schema string
	require.NoError(t, db.QueryRow(`SELECT current_schema()`).Scan(&schema))
	dsnURL, err := url.Parse(strings.TrimSpace(os.Getenv(instructionAuditPostgresTestEnv)))
	require.NoError(t, err)
	query := dsnURL.Query()
	query.Set("search_path", schema)
	dsnURL.RawQuery = query.Encode()
	concurrentDB, err := sql.Open("postgres", dsnURL.String())
	require.NoError(t, err)
	concurrentDB.SetMaxOpenConns(4)
	concurrentDB.SetMaxIdleConns(4)
	require.NoError(t, concurrentDB.PingContext(ctx))
	t.Cleanup(func() { require.NoError(t, concurrentDB.Close()) })
	repository := NewInstructionRepository(concurrentDB)
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("38", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)
	plaintext := "concurrent stable instruction"
	digest := sha256Hex(plaintext)
	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	automaticUntil := time.Now().UTC().Add(24 * time.Hour)

	commit := func(requestID string) error {
		ciphertext, encryptErr := cipher.EncryptHashRaw(digest, plaintext)
		if encryptErr != nil {
			return encryptErr
		}
		decision := &InstructionDecision{
			Allow: true, InitialReason: "hash_mismatch", FinalReason: "instructions_match",
			FinalOutcome: InstructionOutcomeAIPass, PolicyAction: "ai_review",
			Instructions: InstructionFieldResult{Present: true, SHA256: digest, Result: "mismatch", Plaintext: plaintext},
			Input1:       InstructionFieldResult{Result: "missing"}, ConfigVersion: 1,
		}
		_, commitErr := repository.CommitAIOutcome(ctx, instructionAIOutcomeCommit{
			Request: Request{
				RequestID: requestID, UserID: userID, GroupID: &groupID,
				InstructionClientType: InstructionClientCodexCLI, Model: "gpt-ai-review", Stage: "http",
			},
			Decision: decision, EvidenceStatus: "not_available",
			Attempts: []instructionAIReviewAttempt{{
				ReviewedSource: "instructions", ReviewedSHA256: digest, Result: "pass",
				ApprovedSource: "instructions", Confidence: 0.99, Reason: "stable",
				ReviewerModel: "review-model", PromptVersion: "review-v1",
			}},
			FinalAttempt: 0,
			ApprovedRaw: &instructionHashRawStorage{
				Ciphertext: ciphertext, Status: "stored", ContentBytes: len([]byte(plaintext)),
				HashAlgorithm: InstructionHashAlgorithmSHA256,
				Normalization: InstructionHashNormalizationIdentityV1,
				KeyVersion:    instructionHashRawKeyVersion, ExpiresAt: &expiresAt,
			},
			ApprovedField: decision.Instructions, AutomaticUntil: automaticUntil,
		})
		return commitErr
	}

	start := make(chan struct{})
	errorsByRequest := make(chan error, 2)
	var wait sync.WaitGroup
	for index := range 2 {
		wait.Add(1)
		go func(requestID string) {
			defer wait.Done()
			<-start
			errorsByRequest <- commit(requestID)
		}(fmt.Sprintf("ai-concurrent-%d", index))
	}
	close(start)
	wait.Wait()
	close(errorsByRequest)
	for commitErr := range errorsByRequest {
		require.NoError(t, commitErr)
	}
	require.EqualValues(t, 1, instructionAuditTableCount(t, db, "instruction_audit_hashes"))
	require.EqualValues(t, 1, instructionAuditTableCount(t, db, "instruction_audit_rule_sets"))
	require.EqualValues(t, 1, instructionAuditTableCount(t, db, "instruction_audit_group_bindings"))
	require.EqualValues(t, 2, instructionAuditTableCount(t, db, "instruction_audit_ai_reviews"))
	require.EqualValues(t, 2, instructionAuditTableCount(t, db, "instruction_audit_events"))
	require.EqualValues(t, 2, instructionAuditTableCount(t, db, "instruction_audit_hash_sources"))
}

func TestInstructionAuditPostgresPersistsMultipleAIAttemptsForRejectedEvent(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "ai-rejected-user@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "AI Rejected Group")
	instructionsDigest := sha256Hex("rejected instructions")
	inputDigest := sha256Hex("uncertain input")
	decision := &InstructionDecision{
		Allow: false, InitialReason: "hash_mismatch", FinalReason: "ai_uncertain",
		FinalOutcome: InstructionOutcomeBlocked, PolicyAction: InstructionPolicyActionBlock,
		Instructions:  InstructionFieldResult{Present: true, SHA256: instructionsDigest, Result: "mismatch"},
		Input1:        InstructionFieldResult{Present: true, SHA256: inputDigest, Result: "mismatch"},
		ConfigVersion: 1,
	}
	result, err := repository.CommitAIOutcome(ctx, instructionAIOutcomeCommit{
		Request: Request{
			RequestID: "ai-rejected-request", UserID: userID, GroupID: &groupID,
			InstructionClientType: InstructionClientCodexCLI, Model: "gpt-ai-review", Stage: "http",
		},
		Decision: decision, EvidenceStatus: "not_available",
		Attempts: []instructionAIReviewAttempt{
			{ReviewedSource: "instructions", ReviewedSHA256: instructionsDigest, Result: "reject", Confidence: 0.98, Reason: "unsafe", ReviewerModel: "review-model", PromptVersion: "review-v1", LatencyMS: 10},
			{ReviewedSource: "input1", ReviewedSHA256: inputDigest, Result: "uncertain", Confidence: 0.6, Reason: "ambiguous", ReviewerModel: "review-model", PromptVersion: "review-v1", LatencyMS: 11},
		},
		FinalAttempt: 1,
	})
	require.NoError(t, err)
	require.Zero(t, result.ConfigVersion)
	reviews, err := repository.ListAIReviewsForEvent(ctx, result.EventID)
	require.NoError(t, err)
	require.Len(t, reviews, 2)
	require.Equal(t, "reject", reviews[0].Result)
	require.Equal(t, "uncertain", reviews[1].Result)
	event, err := repository.GetEvent(ctx, result.EventID)
	require.NoError(t, err)
	require.Equal(t, "ai_uncertain", event.FinalReason)
	require.Equal(t, InstructionOutcomeBlocked, event.FinalOutcome)
	require.Equal(t, result.AIReviewID, *event.AIReviewID)
	require.EqualValues(t, 1, event.ConfigVersion)
}

func TestInstructionAuditPostgresRuleSetExceptionsAreEffective(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repo := NewInstructionRepository(db)
	ctx := context.Background()
	actorID := insertInstructionAuditUser(t, db, "exception-admin@example.test", "admin")
	allowedUserID := insertInstructionAuditUser(t, db, "allowed@example.test", "user")
	otherUserID := insertInstructionAuditUser(t, db, "other@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "Exception Group")

	ruleSet, err := repo.SaveRuleSet(ctx, 0, SaveInstructionRuleSetRequest{
		Name: "exception-only", Enabled: true, AllowEmptyFields: true,
		AllowedUserIDs: []int64{allowedUserID},
	}, actorID)
	require.NoError(t, err)
	require.True(t, ruleSet.AllowEmptyFields)
	require.Empty(t, ruleSet.Hashes)
	require.Equal(t, []InstructionRuleSetUser{{
		ID: allowedUserID, Email: "allowed@example.test", Deleted: false,
	}}, ruleSet.AllowedUsers)

	_, err = repo.SaveGroupBindings(ctx, SaveInstructionGroupBindingsRequest{
		GroupIDs: []int64{groupID}, RuleSetID: ruleSet.ID, Enabled: true,
	}, actorID)
	require.NoError(t, err)
	update, err := repo.SetEnabled(ctx, true)
	require.NoError(t, err)
	require.False(t, update.Before)

	bindings, err := repo.ListGroupBindings(ctx)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.True(t, bindings[0].Effective)

	snapshot, err := repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	policy := snapshot.Policies[groupID]
	require.True(t, policy.AllowEmptyFields)
	_, userAllowed := policy.AllowedUsers[allowedUserID]
	require.True(t, userAllowed)
	require.Empty(t, policy.Hashes)

	service := NewInstructionService(repo, nil, nil)
	service.snapshot.Store(snapshot)
	whitelisted := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: allowedUserID, GroupID: &groupID,
		InstructionBody: []byte(`{`),
	})
	require.True(t, whitelisted.Allow)
	require.Equal(t, "user_allowlist", whitelisted.Reason)

	empty := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: otherUserID, GroupID: &groupID,
		InstructionBody: []byte(`{"instructions":"","input":[{}, {"content":[]}]}`),
	})
	require.True(t, empty.Allow)
	require.Equal(t, "empty_fields_allowed", empty.Reason)

	nonEmpty := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: otherUserID, GroupID: &groupID,
		InstructionBody: []byte(`{"instructions":"not-empty"}`),
	})
	require.False(t, nonEmpty.Allow)
	require.Equal(t, "hash_mismatch", nonEmpty.Reason)

	_, _, _, _, effectiveGroups, _, err := repo.OverviewCounts(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, effectiveGroups)

	_, err = repo.SaveRuleSet(ctx, ruleSet.ID, SaveInstructionRuleSetRequest{
		Name: "must-roll-back", Enabled: true, AllowedUserIDs: []int64{999999999},
	}, actorID)
	require.ErrorIs(t, err, errInstructionAuditAllowedUserNotFound)
	rules, err := repo.ListRuleSets(ctx)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, "exception-only", rules[0].Name)
	require.True(t, rules[0].AllowEmptyFields)
	require.Equal(t, []InstructionRuleSetUser{{
		ID: allowedUserID, Email: "allowed@example.test", Deleted: false,
	}}, rules[0].AllowedUsers)
}

func insertInstructionAuditUser(t *testing.T, db *sql.DB, email, role string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO users (email, username, role, status)
		VALUES ($1, $2, $3, 'active') RETURNING id`, email, strings.Split(email, "@")[0], role).Scan(&id))
	return id
}

func insertInstructionAuditGroup(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO groups (name, platform, status)
		VALUES ($1, 'openai', 'active') RETURNING id`, name).Scan(&id))
	return id
}

func createInstructionAuditPolicy(t *testing.T, repo *InstructionRepository, actorID, groupID int64, plaintext string) *instructionSnapshot {
	t.Helper()
	ctx := context.Background()
	hash, err := repo.CreateHash(ctx, CreateInstructionHashRequest{
		Digest: sha256Hex(plaintext), Name: "trusted client", ObservedSource: "instructions", Status: "active",
	}, actorID)
	require.NoError(t, err)
	ruleSet, err := repo.SaveRuleSet(ctx, 0, SaveInstructionRuleSetRequest{
		Name: "codex stable", Enabled: true, HashIDs: []int64{hash.ID},
	}, actorID)
	require.NoError(t, err)
	_, err = repo.SaveGroupBindings(ctx, SaveInstructionGroupBindingsRequest{
		GroupIDs: []int64{groupID}, RuleSetID: ruleSet.ID, Enabled: true,
	}, actorID)
	require.NoError(t, err)
	update, err := repo.SetEnabled(ctx, true)
	require.NoError(t, err)
	require.False(t, update.Before)
	snapshot, err := repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	return snapshot
}

func TestInstructionAuditPostgresConfigAndUnifiedHashPool(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repo := NewInstructionRepository(db)
	ctx := context.Background()
	var enabled string
	require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE key = $1`, SettingKeyInstructionAuditEnabled).Scan(&enabled))
	require.Equal(t, "false", enabled)

	update, err := repo.SetEnabled(ctx, true)
	require.ErrorIs(t, err, ErrInstructionAuditNoEffectiveGroupRules)
	require.False(t, update.Before)

	actorID := insertInstructionAuditUser(t, db, "admin@example.test", "admin")
	firstUserID := insertInstructionAuditUser(t, db, "first@example.test", "user")
	secondUserID := insertInstructionAuditUser(t, db, "second@example.test", "user")
	auditedGroupID := insertInstructionAuditGroup(t, db, "Audited OpenAI")
	unboundGroupID := insertInstructionAuditGroup(t, db, "Unbound OpenAI")
	snapshot := createInstructionAuditPolicy(t, repo, actorID, auditedGroupID, "trusted")
	require.True(t, snapshot.Enabled)
	migration199, err := os.ReadFile(filepath.Join("..", "..", "migrations", "199_instruction_audit_group_scope.sql"))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration199))
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE key = $1`, SettingKeyInstructionAuditEnabled).Scan(&enabled))
	require.Equal(t, "true", enabled, "reapplying the migration must not disable configured group scope")
	update, err = repo.SetEnabled(ctx, true)
	require.NoError(t, err)
	require.True(t, update.Before)
	service := NewInstructionService(repo, nil, nil)
	service.snapshot.Store(snapshot)
	decision := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: firstUserID, GroupID: &auditedGroupID, Model: "gpt-a",
		InstructionBody: []byte(`{"model":"gpt-a","instructions":"other","input":[{}, {"content":[{"type":"input_text","text":"trust"},{"type":"input_text","text":"ed"}]}]}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)
	require.Equal(t, "input1_match", decision.Reason)

	sameGroup := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: secondUserID, GroupID: &auditedGroupID, Model: "completely-different-model",
		InstructionBody: []byte(`{"instructions":"trusted"}`),
	})
	require.True(t, sameGroup.Applicable)
	require.True(t, sameGroup.Allow)

	unbound := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: firstUserID, GroupID: &unboundGroupID, Model: "gpt-a", InstructionBody: []byte(`{`),
	})
	require.False(t, unbound.Applicable)
	require.True(t, unbound.Allow)

	secondHash, err := repo.CreateHash(ctx, CreateInstructionHashRequest{
		Digest: sha256Hex("alternate"), Name: "alternate client", Status: "active",
	}, actorID)
	require.NoError(t, err)
	secondRule, err := repo.SaveRuleSet(ctx, 0, SaveInstructionRuleSetRequest{
		Name: "alternate policy", Enabled: true, HashIDs: []int64{secondHash.ID},
	}, actorID)
	require.NoError(t, err)
	_, err = repo.SaveGroupBindings(ctx, SaveInstructionGroupBindingsRequest{
		GroupIDs: []int64{auditedGroupID}, RuleSetID: secondRule.ID, Enabled: true,
	}, actorID)
	require.NoError(t, err)
	snapshot, err = repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	service.snapshot.Store(snapshot)
	union := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: secondUserID, GroupID: &auditedGroupID, Model: "any-model",
		InstructionBody: []byte(`{"instructions":"alternate"}`),
	})
	require.True(t, union.Allow)
	require.Len(t, union.RuleSetIDs, 2)
}

func TestInstructionAuditPostgresBoundGroupFailsClosedWhenRuleBecomesIneffective(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repo := NewInstructionRepository(db)
	ctx := context.Background()
	actorID := insertInstructionAuditUser(t, db, "rule-admin@example.test", "admin")
	groupID := insertInstructionAuditGroup(t, db, "Fail Closed Group")
	snapshot := createInstructionAuditPolicy(t, repo, actorID, groupID, "trusted")
	service := NewInstructionService(repo, nil, nil)
	service.snapshot.Store(snapshot)

	rules, err := repo.ListRuleSets(ctx)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	_, err = repo.SaveRuleSet(ctx, rules[0].ID, SaveInstructionRuleSetRequest{
		Name: rules[0].Name, Description: rules[0].Description, Enabled: false,
		HashIDs: []int64{rules[0].Hashes[0].ID},
	}, actorID)
	require.NoError(t, err)
	snapshot, err = repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	service.snapshot.Store(snapshot)

	decision := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, GroupID: &groupID, Model: "any-model",
		InstructionBody: []byte(`{"instructions":"trusted"}`),
	})
	require.True(t, decision.Applicable)
	require.False(t, decision.Allow)
	require.Equal(t, "hash_mismatch", decision.Reason)
	require.Empty(t, decision.RuleSetIDs)
	require.Zero(t, service.failedBlockedEventPersists.Load())

	var persistedRuleSetIDs string
	require.NoError(t, db.QueryRow(`
		SELECT rule_set_ids::text
		FROM instruction_audit_events
		ORDER BY id DESC
		LIMIT 1`).Scan(&persistedRuleSetIDs))
	require.Equal(t, "[]", persistedRuleSetIDs)
}

func TestInstructionAuditPostgresClientScopeAndEventMetadata(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repo := NewInstructionRepository(db)
	ctx := context.Background()
	actorID := insertInstructionAuditUser(t, db, "client-scope-admin@example.test", "admin")
	groupID := insertInstructionAuditGroup(t, db, "Client Scoped Group")
	_ = createInstructionAuditPolicy(t, repo, actorID, groupID, "trusted")

	bindings, err := repo.ListGroupBindings(ctx)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, []string{InstructionClientAll}, bindings[0].ClientTypes)

	bindings, err = repo.SaveGroupBindings(ctx, SaveInstructionGroupBindingsRequest{
		GroupIDs: []int64{groupID}, RuleSetID: bindings[0].RuleSetID,
		ClientTypes: []string{InstructionClientCodexCLI, InstructionClientCodexVSCode}, Enabled: true,
	}, actorID)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, []string{InstructionClientCodexVSCode, InstructionClientCodexCLI}, bindings[0].ClientTypes)

	bindings, err = repo.SaveGroupBindings(ctx, SaveInstructionGroupBindingsRequest{
		GroupIDs: []int64{groupID}, RuleSetID: bindings[0].RuleSetID, Enabled: true,
	}, actorID)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, []string{InstructionClientCodexVSCode, InstructionClientCodexCLI}, bindings[0].ClientTypes,
		"legacy updates that omit client_types must preserve the configured scope")

	snapshot, err := repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	instructionService := NewInstructionService(repo, nil, nil)
	instructionService.snapshot.Store(snapshot)

	bypassed := instructionService.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, GroupID: &groupID, UserAgent: "opencode/1.0",
		InstructionBody: []byte(`{`),
	})
	require.False(t, bypassed.Applicable)
	require.True(t, bypassed.Allow)

	blocked := instructionService.EvaluateInstruction(ctx, Request{
		RequestID: "client-scope-blocked", Protocol: instructionAuditProtocol,
		GroupID: &groupID, GroupName: "Client Scoped Group", UserAgent: "codex_cli_rs/0.145.0 (Windows)",
		InstructionBody: []byte(`{"instructions":"untrusted"}`),
	})
	require.True(t, blocked.Applicable)
	require.False(t, blocked.Allow)

	var clientType, userAgent string
	require.NoError(t, db.QueryRow(`
		SELECT client_type, client_user_agent
		FROM instruction_audit_events WHERE request_id = 'client-scope-blocked'`).Scan(&clientType, &userAgent))
	require.Equal(t, InstructionClientCodexCLI, clientType)
	require.Equal(t, "codex_cli_rs/0.145.0 (Windows)", userAgent)
}

func TestInstructionAuditPostgresPersistsEncryptedEvidenceAndReviewTrail(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repo := NewInstructionRepository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "user@example.test", "user")
	adminID := insertInstructionAuditUser(t, db, "admin@example.test", "admin")
	groupID := insertInstructionAuditGroup(t, db, "Metadata Group")
	var apiKeyID int64
	require.NoError(t, db.QueryRow(`INSERT INTO api_keys DEFAULT VALUES RETURNING id`).Scan(&apiKeyID))
	snapshot := createInstructionAuditPolicy(t, repo, userID, groupID, "trusted")
	instructionService := NewInstructionService(repo, nil, nil)
	evidenceCipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{
			EncryptionKey:           strings.Repeat("42", 32),
			EncryptionKeyConfigured: true,
		},
	})
	require.NoError(t, err)
	instructionService.evidenceCipher = evidenceCipher
	instructionService.snapshot.Store(snapshot)

	const plaintextCanary = "INSTRUCTION_AUDIT_PLAINTEXT_CANARY_DO_NOT_STORE"
	const secondPlaintextCanary = "INSTRUCTION_AUDIT_SECOND_CANARY_DO_NOT_STORE"
	request := Request{
		RequestID: "request-1", UserID: userID, UserEmail: "user@example.test", APIKeyID: apiKeyID,
		GroupID: &groupID, GroupName: "Metadata Group",
		Endpoint: "/v1/responses", Protocol: instructionAuditProtocol, Model: "gpt-test", Stage: "http",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"` + plaintextCanary + `"}`),
	}
	first := instructionService.EvaluateInstruction(ctx, request)
	require.True(t, first.Applicable)
	require.False(t, first.Allow)
	require.Equal(t, sha256Hex(plaintextCanary), first.Instructions.SHA256)

	request.RequestID = "request-2"
	request.InstructionBody = []byte(`{"model":"gpt-test","instructions":"` + secondPlaintextCanary + `"}`)
	second := instructionService.EvaluateInstruction(ctx, request)
	require.False(t, second.Allow)

	var eventCount, legacyOutboxCount, evidenceCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM instruction_audit_events`).Scan(&eventCount))
	require.Equal(t, 2, eventCount)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM instruction_audit_notification_outbox`).Scan(&legacyOutboxCount))
	require.Zero(t, legacyOutboxCount, "new events must use the unified notification queue")
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM instruction_audit_evidence`).Scan(&evidenceCount))
	require.Equal(t, 2, evidenceCount)

	var persisted string
	require.NoError(t, db.QueryRow(`
		SELECT COALESCE(string_agg(row_to_json(e)::text, ''), '')
		FROM instruction_audit_events e`).Scan(&persisted))
	require.NotContains(t, persisted, plaintextCanary)
	require.NotContains(t, persisted, "Bearer ")
	require.Contains(t, persisted, "Metadata Group")

	var ciphertext []byte
	var digest string
	var plaintextBytes int
	require.NoError(t, db.QueryRow(`
		SELECT ciphertext, digest, plaintext_bytes
		FROM instruction_audit_evidence
		WHERE event_id = (SELECT id FROM instruction_audit_events WHERE request_id = 'request-1')
		  AND source = 'instructions'`).Scan(&ciphertext, &digest, &plaintextBytes))
	require.NotContains(t, string(ciphertext), plaintextCanary)
	require.False(t, bytes.Contains(ciphertext, []byte(plaintextCanary)))
	require.Equal(t, len([]byte(plaintextCanary)), plaintextBytes)
	decrypted, err := evidenceCipher.Decrypt("instructions", digest, ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintextCanary, decrypted)

	var eventID int64
	require.NoError(t, db.QueryRow(`SELECT id FROM instruction_audit_events WHERE request_id = 'request-1'`).Scan(&eventID))
	review, err := instructionService.RevealEvidence(ctx, eventID, InstructionEvidenceAccess{ActorID: adminID})
	require.NoError(t, err)
	require.Equal(t, "stored", review.Status)
	require.Len(t, review.Fields, 1)
	require.Equal(t, plaintextCanary, review.Fields[0].Plaintext)
	require.True(t, review.Fields[0].DigestConsistent)
	require.NoError(t, instructionService.RecordEvidenceCopy(ctx, eventID, InstructionEvidenceAccess{
		ActorID: adminID, Source: "instructions_plaintext",
	}))
	var revealCount, copyCount int
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*) FILTER (WHERE action = 'reveal'), COUNT(*) FILTER (WHERE action = 'copy')
		FROM instruction_audit_evidence_access_logs WHERE event_id = $1`, eventID).Scan(&revealCount, &copyCount))
	require.Equal(t, 1, revealCount)
	require.Equal(t, 1, copyCount)
	candidate, err := instructionService.CreateCandidateFromEvent(ctx, eventID, CreateInstructionCandidateRequest{
		Source: "instructions", ReviewConfirmed: true, Name: "reviewed canary",
	}, adminID)
	require.NoError(t, err)
	require.Equal(t, sha256Hex(plaintextCanary), candidate.Digest)
	require.Equal(t, "stored", candidate.RawStatus)
	require.Equal(t, len([]byte(plaintextCanary)), candidate.ContentBytes)
	raw, err := repo.GetHashRaw(ctx, candidate.ID)
	require.NoError(t, err)
	require.NotContains(t, string(raw.Ciphertext), plaintextCanary)
	decryptedRaw, err := evidenceCipher.DecryptHashRaw(candidate.Digest, raw.Ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintextCanary, decryptedRaw)
	sources, err := repo.ListHashSources(ctx, candidate.ID)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	require.Equal(t, "manual", sources[0].SourceType)
	require.Equal(t, "instructions", sources[0].FieldName)
	require.NotNil(t, sources[0].EventID)
	require.Equal(t, eventID, *sources[0].EventID)

	var secondEventID int64
	require.NoError(t, db.QueryRow(`SELECT id FROM instruction_audit_events WHERE request_id = 'request-2'`).Scan(&secondEventID))
	secondReview, err := instructionService.RevealEvidence(ctx, secondEventID, InstructionEvidenceAccess{ActorID: adminID})
	require.NoError(t, err)
	require.Equal(t, "stored", secondReview.Status)
	ruleSet, err := repo.SaveRuleSet(ctx, 0, SaveInstructionRuleSetRequest{
		Name: "reviewed event rules", Enabled: true,
	}, adminID)
	require.NoError(t, err)
	added, err := instructionService.AddEventToRuleSet(ctx, secondEventID, AddInstructionEventToRuleSetRequest{
		RuleSetID: ruleSet.ID, Sources: []string{"instructions"}, ReviewConfirmed: true,
	}, adminID)
	require.NoError(t, err)
	require.Equal(t, 1, added.CreatedHashes)
	require.Equal(t, 1, added.AttachedHashes)
	require.Len(t, added.HashIDs, 1)
	addedHash, err := repo.GetHash(ctx, added.HashIDs[0])
	require.NoError(t, err)
	require.Equal(t, sha256Hex(secondPlaintextCanary), addedHash.Digest)
	require.Equal(t, "stored", addedHash.RawStatus)
	addedRaw, err := repo.GetHashRaw(ctx, addedHash.ID)
	require.NoError(t, err)
	decryptedAddedRaw, err := evidenceCipher.DecryptHashRaw(addedHash.Digest, addedRaw.Ciphertext)
	require.NoError(t, err)
	require.Equal(t, secondPlaintextCanary, decryptedAddedRaw)
	addedSources, err := repo.ListHashSources(ctx, addedHash.ID)
	require.NoError(t, err)
	require.Len(t, addedSources, 1)
	require.NotNil(t, addedSources[0].EventID)
	require.Equal(t, secondEventID, *addedSources[0].EventID)

	notificationRepo := modelportrepository.NewSecurityNotificationRepository(db)
	for _, input := range []service.SecurityNotificationAudienceInput{
		{SourceType: service.SecurityNotificationSourceInstructionAudit, SourceID: eventID, Audience: "ops", Recipients: []string{"ops@example.test"}, TemplateEvent: service.NotificationEmailEventInstructionAuditOpsNotice, DedupKey: "same-dedup-key"},
		{SourceType: service.SecurityNotificationSourceInstructionAudit, SourceID: eventID + 1, Audience: "ops", Recipients: []string{"ops@example.test"}, TemplateEvent: service.NotificationEmailEventInstructionAuditOpsNotice, DedupKey: "same-dedup-key"},
	} {
		require.NoError(t, notificationRepo.Enqueue(ctx, input))
	}
	var pendingCount, suppressedCount int
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*) FILTER (WHERE status = 'pending'), COUNT(*) FILTER (WHERE status = 'suppressed')
		FROM security_notification_outbox WHERE audience = 'ops'`).Scan(&pendingCount, &suppressedCount))
	require.Equal(t, 1, pendingCount)
	require.Equal(t, 1, suppressedCount)
}

func TestInstructionAuditPostgresSnapshotEnforcesValidityWindows(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repo := NewInstructionRepository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "validity@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "Validity Group")
	hash, err := repo.CreateHash(ctx, CreateInstructionHashRequest{
		Digest: sha256Hex("trusted"), Name: "scheduled hash", Status: "active",
	}, userID)
	require.NoError(t, err)
	ruleSet, err := repo.SaveRuleSet(ctx, 0, SaveInstructionRuleSetRequest{
		Name: "scheduled policy", Enabled: true, HashIDs: []int64{hash.ID},
	}, userID)
	require.NoError(t, err)
	_, err = repo.SaveGroupBindings(ctx, SaveInstructionGroupBindingsRequest{
		GroupIDs: []int64{groupID}, RuleSetID: ruleSet.ID, Enabled: true,
	}, userID)
	require.NoError(t, err)
	_, err = repo.SetEnabled(ctx, true)
	require.NoError(t, err)

	evaluate := func(snapshot *instructionSnapshot) *InstructionDecision {
		service := NewInstructionService(repo, nil, nil)
		service.snapshot.Store(snapshot)
		return service.EvaluateInstruction(ctx, Request{
			Protocol: instructionAuditProtocol, UserID: userID, GroupID: &groupID, Model: "gpt-test",
			InstructionBody: []byte(`{"model":"gpt-test","instructions":"trusted"}`),
		})
	}
	snapshot, err := repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	require.True(t, evaluate(snapshot).Allow)

	notBefore := time.Now().UTC().Add(time.Hour)
	hash.ValidFrom = &notBefore
	hash, err = repo.UpdateHash(ctx, *hash)
	require.NoError(t, err)
	snapshot, err = repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	require.False(t, evaluate(snapshot).Allow)

	activeUntil := time.Now().UTC().Add(time.Hour)
	hash.ValidFrom = nil
	hash.ValidUntil = &activeUntil
	hash, err = repo.UpdateHash(ctx, *hash)
	require.NoError(t, err)
	snapshot, err = repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	require.True(t, evaluate(snapshot).Allow)

	expiredAt := time.Now().UTC().Add(-time.Second)
	hash.ValidUntil = &expiredAt
	_, err = repo.UpdateHash(ctx, *hash)
	require.NoError(t, err)
	snapshot, err = repo.LoadSnapshot(ctx)
	require.NoError(t, err)
	require.False(t, evaluate(snapshot).Allow)
}
