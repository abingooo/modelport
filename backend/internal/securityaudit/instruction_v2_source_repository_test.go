package securityaudit

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

const (
	trustedSourceTupleInsertOnly = "trusted source tuple insert only"
	manualSourceTupleInsertOnly  = "manual source tuple insert only"
)

func TestInstructionV2GetHashReturnsDurableSourceAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	digest := strings.Repeat("d", 64)
	createdAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT h.id, h.sha256.*h.source_event_id, h.source_user_id.*h.source_user_email_snapshot.*WHERE h.id = \$1`).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "sha256", "name", "note", "status", "source", "observed_field",
			"hash_algorithm", "normalization_version", "content_bytes", "raw_storage",
			"raw_ciphertext", "stored_bytes", "ai_sampled", "source_event_id", "source_user_id",
			"source_user_email_snapshot", "reviewer_node_id", "reviewer_model", "prompt_version",
			"confidence", "review_reason", "review_category", "candidate_expires_at", "created_by",
			"updated_by", "created_at", "updated_at", "global_trust", "content_vault_id",
		}).AddRow(
			int64(11), digest, "trusted", "", "active", "manual", "instructions",
			"sha256", "identity_utf8_v1", int64(12), "full", []byte("ciphertext"), 12, false,
			int64(21), int64(31), "durable@example.test", nil, "", "", nil, "", "", nil,
			nil, nil, createdAt, createdAt, true, int64(41),
		))
	mock.ExpectQuery(`(?s)SELECT hs.hash_id, s.id, s.group_id.*WHERE hs.hash_id = \$1`).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{
			"hash_id", "scope_id", "group_id", "group_name", "client_profile_id",
			"client_profile_key", "client_profile_name", "status", "source",
			"candidate_expires_at", "created_at", "updated_at",
		}))
	mock.ExpectClose()

	item, ciphertext, err := NewInstructionV2Repository(db).GetHash(context.Background(), 11)
	require.NoError(t, err)
	require.Equal(t, []byte("ciphertext"), ciphertext)
	require.Equal(t, int64(21), *item.SourceEventID)
	require.Equal(t, int64(31), *item.SourceUserID)
	require.Equal(t, "durable@example.test", item.SourceUserEmail)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionV2RiskUpsertReplacesEntireSourceTuple(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	digest := strings.Repeat("a", 64)
	userID := int64(202)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO instruction_audit_v2_content_vault.*ON CONFLICT.*RETURNING id`).
		WithArgs(digest, []byte("ciphertext"), int64(12), 12, "instructions").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
	mock.ExpectQuery(`(?s)INSERT INTO instruction_audit_v2_risk_hashes.*ON CONFLICT.*source_event_id = EXCLUDED.source_event_id.*source_user_id = EXCLUDED.source_user_id.*source_user_email_snapshot = EXCLUDED.source_user_email_snapshot.*RETURNING id`).
		WithArgs(digest, int64(91), "instructions", "sync_ai", int64(101), userID,
			"latest@example.test", nil, "review-model", "prompt-v2", 0.97, "latest", "safe").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(301)))
	mock.ExpectRollback()
	mock.ExpectClose()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	riskID, err := upsertInstructionV2RiskTx(context.Background(), tx, 101, instructionV2RiskWrite{
		Vault: instructionV2VaultWrite{
			SHA256: digest, ObservedField: "instructions", RawCiphertext: []byte("ciphertext"),
			ContentBytes: 12, StoredBytes: 12,
		},
		Source: "sync_ai", ObservedField: "instructions", ReviewerModel: "review-model",
		PromptVersion: "prompt-v2", Confidence: 0.97, ReviewReason: "latest",
		ReviewCategory: "safe", SourceUserID: &userID, SourceUserEmail: "latest@example.test",
	})
	require.NoError(t, err)
	require.Equal(t, int64(301), riskID)
	require.NoError(t, tx.Rollback())
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionV2TrustedUpsertPreservesFirstSourceTupleOnConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sourceTupleQueryMatcher{}))
	require.NoError(t, err)
	digest := strings.Repeat("b", 64)
	userID := int64(402)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT EXISTS \(.*instruction_audit_v2_risk_hashes.*\)`).
		WithArgs(digest).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`(?s)INSERT INTO instruction_audit_v2_content_vault.*ON CONFLICT.*RETURNING id`).
		WithArgs(digest, []byte("ciphertext"), int64(12), 12, "instructions").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(391)))
	mock.ExpectQuery(trustedSourceTupleInsertOnly).
		WithArgs(digest, "AI 可信 "+digest[:12], "由指令审核复核通过", "ai_review",
			"instructions", int64(12), false, int64(401), userID, "first@example.test",
			nil, "review-model", "prompt-v2", 0.99, "safe", "safe", true, int64(391), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(501)))
	mock.ExpectRollback()
	mock.ExpectClose()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	hashID, err := upsertInstructionV2TrustedTx(context.Background(), tx, 401, instructionV2TrustedWrite{
		Vault: instructionV2VaultWrite{
			SHA256: digest, ObservedField: "instructions", RawCiphertext: []byte("ciphertext"),
			ContentBytes: 12, StoredBytes: 12,
		},
		Source: "ai_review", ObservedField: "instructions", ReviewerModel: "review-model",
		PromptVersion: "prompt-v2", Confidence: 0.99, ReviewReason: "safe",
		ReviewCategory: "safe", GlobalTrust: true, SourceUserID: &userID,
		SourceUserEmail: "first@example.test",
	}, 0)
	require.NoError(t, err)
	require.Equal(t, int64(501), hashID)
	require.NoError(t, tx.Rollback())
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionV2ManualHashStoresRequestSourceTuple(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sourceTupleQueryMatcher{}))
	require.NoError(t, err)
	digest := strings.Repeat("c", 64)
	eventID, userID := int64(601), int64(602)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT EXISTS \(.*instruction_audit_v2_risk_hashes.*\)`).
		WithArgs(digest).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(manualSourceTupleInsertOnly).
		WithArgs(digest, "trusted", "", "active", "manual", "instructions", int64(12),
			eventID, userID, "request@example.test", true, nil, int64(701)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(801)))
	mock.ExpectRollback()
	mock.ExpectClose()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	hashID, err := saveInstructionV2ManualHashTx(context.Background(), tx, instructionV2ManualHashWrite{
		SHA256: digest, Name: "trusted", Status: "active", Source: "manual",
		ContentBytes: 12, GlobalTrust: true, ObservedField: "instructions",
		SourceEventID: &eventID, SourceUserID: &userID, SourceUserEmail: "request@example.test",
	}, 701)
	require.NoError(t, err)
	require.Equal(t, int64(801), hashID)
	require.NoError(t, tx.Rollback())
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

type sourceTupleQueryMatcher struct{}

func (sourceTupleQueryMatcher) Match(expectedSQL, actualSQL string) error {
	if expectedSQL != trustedSourceTupleInsertOnly && expectedSQL != manualSourceTupleInsertOnly {
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	}
	normalized := strings.ToLower(actualSQL)
	conflictIndex := strings.Index(normalized, "on conflict")
	if conflictIndex < 0 {
		return fmt.Errorf("source tuple upsert has no conflict clause")
	}
	insertClause, conflictClause := normalized[:conflictIndex], normalized[conflictIndex:]
	for _, column := range []string{"source_event_id", "source_user_id", "source_user_email_snapshot"} {
		if !strings.Contains(insertClause, column) {
			return fmt.Errorf("source tuple insert is missing %s", column)
		}
		if strings.Contains(conflictClause, column+" =") {
			return fmt.Errorf("trusted hash conflict overwrites first source column %s", column)
		}
	}
	return nil
}
