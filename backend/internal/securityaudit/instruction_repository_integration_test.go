package securityaudit

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const instructionAuditPostgresTestEnv = "INSTRUCTION_AUDIT_TEST_POSTGRES_DSN"

func openInstructionAuditIntegrationDB(t *testing.T) *sql.DB {
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
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "198_instruction_audit.sql"))
	require.NoError(t, err)
	for range 2 {
		_, err = db.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	return db
}

func insertInstructionAuditUser(t *testing.T, db *sql.DB, email, role string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO users (email, username, role, status)
		VALUES ($1, $2, $3, 'active') RETURNING id`, email, strings.Split(email, "@")[0], role).Scan(&id))
	return id
}

func createInstructionAuditPolicy(t *testing.T, repo *InstructionRepository, userID int64, model, plaintext string) *instructionSnapshot {
	t.Helper()
	ctx := context.Background()
	hash, err := repo.CreateHash(ctx, CreateInstructionHashRequest{
		Digest: sha256Hex(plaintext), Name: "trusted client", ObservedSource: "instructions", Status: "active",
	}, userID)
	require.NoError(t, err)
	ruleSet, err := repo.SaveRuleSet(ctx, 0, SaveInstructionRuleSetRequest{
		Name: "codex stable", Enabled: true, HashIDs: []int64{hash.ID},
	}, userID)
	require.NoError(t, err)
	_, err = repo.SaveBinding(ctx, CreateInstructionBindingRequest{
		UserID: userID, Model: model, RuleSetID: ruleSet.ID, Enabled: true,
	}, userID)
	require.NoError(t, err)
	update, err := repo.SetEnabled(ctx, true, false)
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

	update, err := repo.SetEnabled(ctx, true, false)
	require.ErrorIs(t, err, ErrInstructionAuditConfirmationRequired)
	require.False(t, update.Before)

	userID := insertInstructionAuditUser(t, db, "user@example.test", "user")
	snapshot := createInstructionAuditPolicy(t, repo, userID, "gpt-test", "trusted")
	require.True(t, snapshot.Enabled)
	update, err = repo.SetEnabled(ctx, true, false)
	require.NoError(t, err)
	require.True(t, update.Before)
	service := NewInstructionService(repo, nil, nil)
	service.snapshot.Store(snapshot)
	decision := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: userID, Model: "gpt-test",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"other","input":[{}, {"content":[{"type":"input_text","text":"trust"},{"type":"input_text","text":"ed"}]}]}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)
	require.Equal(t, "input1_match", decision.Reason)

	unbound := service.EvaluateInstruction(ctx, Request{
		Protocol: instructionAuditProtocol, UserID: userID, Model: "other", InstructionBody: []byte(`{`),
	})
	require.False(t, unbound.Applicable)
	require.True(t, unbound.Allow)
}

func TestInstructionAuditPostgresPersistsMetadataAndRateLimitsOutbox(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repo := NewInstructionRepository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "user@example.test", "user")
	insertInstructionAuditUser(t, db, "admin@example.test", "admin")
	var apiKeyID int64
	require.NoError(t, db.QueryRow(`INSERT INTO api_keys DEFAULT VALUES RETURNING id`).Scan(&apiKeyID))
	snapshot := createInstructionAuditPolicy(t, repo, userID, "gpt-test", "trusted")
	service := NewInstructionService(repo, nil, nil)
	service.snapshot.Store(snapshot)

	const plaintextCanary = "INSTRUCTION_AUDIT_PLAINTEXT_CANARY_DO_NOT_STORE"
	request := Request{
		RequestID: "request-1", UserID: userID, UserEmail: "user@example.test", APIKeyID: apiKeyID,
		Endpoint: "/v1/responses", Protocol: instructionAuditProtocol, Model: "gpt-test", Stage: "http",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"` + plaintextCanary + `"}`),
	}
	first := service.EvaluateInstruction(ctx, request)
	require.True(t, first.Applicable)
	require.False(t, first.Allow)
	require.Equal(t, sha256Hex(plaintextCanary), first.Instructions.SHA256)

	request.RequestID = "request-2"
	second := service.EvaluateInstruction(ctx, request)
	require.False(t, second.Allow)

	var eventCount, outboxCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM instruction_audit_events`).Scan(&eventCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM instruction_audit_notification_outbox`).Scan(&outboxCount))
	require.Equal(t, 2, eventCount)
	require.Equal(t, 1, outboxCount)

	var persisted string
	require.NoError(t, db.QueryRow(`
		SELECT COALESCE(string_agg(row_to_json(e)::text, ''), '')
		FROM instruction_audit_events e`).Scan(&persisted))
	require.NotContains(t, persisted, plaintextCanary)
	require.NotContains(t, persisted, "Bearer ")

	service.processOutbox(ctx)
	var status string
	var attempts int
	require.NoError(t, db.QueryRow(`SELECT status, attempts FROM instruction_audit_notification_outbox`).Scan(&status, &attempts))
	require.Equal(t, "retry", status)
	require.Equal(t, 1, attempts)

	recipients, err := repo.ListAdminRecipients(ctx)
	require.NoError(t, err)
	require.Len(t, recipients, 1)
	require.Equal(t, "admin@example.test", recipients[0].Email)
}

func TestInstructionAuditPostgresSnapshotEnforcesValidityWindows(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repo := NewInstructionRepository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "validity@example.test", "user")
	notBefore := time.Now().UTC().Add(time.Hour)
	hash, err := repo.CreateHash(ctx, CreateInstructionHashRequest{
		Digest: sha256Hex("trusted"), Name: "scheduled hash", Status: "active", ValidFrom: &notBefore,
	}, userID)
	require.NoError(t, err)
	ruleSet, err := repo.SaveRuleSet(ctx, 0, SaveInstructionRuleSetRequest{
		Name: "scheduled policy", Enabled: true, HashIDs: []int64{hash.ID},
	}, userID)
	require.NoError(t, err)
	_, err = repo.SaveBinding(ctx, CreateInstructionBindingRequest{
		UserID: userID, Model: "gpt-test", RuleSetID: ruleSet.ID, Enabled: true,
	}, userID)
	require.NoError(t, err)
	_, err = repo.SetEnabled(ctx, true, true)
	require.NoError(t, err)

	evaluate := func(snapshot *instructionSnapshot) *InstructionDecision {
		service := NewInstructionService(repo, nil, nil)
		service.snapshot.Store(snapshot)
		return service.EvaluateInstruction(ctx, Request{
			Protocol: instructionAuditProtocol, UserID: userID, Model: "gpt-test",
			InstructionBody: []byte(`{"model":"gpt-test","instructions":"trusted"}`),
		})
	}
	snapshot, err := repo.LoadSnapshot(ctx)
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

func TestInstructionAuditRetryDelayIsBounded(t *testing.T) {
	require.Equal(t, 5*time.Second, instructionRetryDelay(1))
	require.Equal(t, 15*time.Minute, instructionRetryDelay(100))
}
