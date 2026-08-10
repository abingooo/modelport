package securityaudit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestInstructionV2RepositoryListHashesUsesVaultPlaintextMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	createdAt := time.Date(2026, 8, 10, 9, 30, 0, 0, time.UTC)
	columns := []string{
		"id", "sha256", "name", "note", "status", "source", "observed_field",
		"hash_algorithm", "normalization_version", "content_bytes", "raw_storage",
		"stored_bytes", "ai_sampled", "source_event_id", "reviewer_node_id",
		"reviewer_model", "prompt_version", "confidence", "review_reason",
		"review_category", "candidate_expires_at", "created_by", "updated_by",
		"created_at", "updated_at", "global_trust", "content_vault_id",
	}
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM instruction_audit_v2_hashes h WHERE 1 = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`(?s)CASE WHEN vault.id IS NULL THEN h.content_bytes ELSE vault.content_bytes END.*CASE WHEN vault.id IS NULL THEN h.raw_storage ELSE 'full' END.*CASE WHEN vault.id IS NULL THEN h.stored_bytes ELSE vault.stored_bytes END.*LEFT JOIN instruction_audit_v2_content_vault vault ON vault.id = h.content_vault_id`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(int64(11), strings.Repeat("a", 64), "vault-backed", "", "active", "manual", "instructions",
				"sha256", "identity_utf8_v1", int64(42), "full", 58, false, nil, nil,
				"", "", nil, "", "", nil, nil, nil, createdAt, createdAt, true, int64(91)).
			AddRow(int64(12), strings.Repeat("b", 64), "digest-only", "", "active", "import", "",
				"sha256", "identity_utf8_v1", int64(0), "unavailable", 0, false, nil, nil,
				"", "", nil, "", "", nil, nil, nil, createdAt, createdAt, true, nil))
	mock.ExpectQuery(`(?s)SELECT hs.hash_id, s.id, s.group_id.*WHERE hs.hash_id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"hash_id", "scope_id", "group_id", "group_name", "client_profile_id",
			"client_profile_key", "client_profile_name", "status", "source",
			"candidate_expires_at", "created_at", "updated_at",
		}))
	mock.ExpectClose()

	page, err := NewInstructionV2Repository(db).ListHashes(context.Background(), 1, 20, "", "")
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.Equal(t, int64(42), page.Items[0].ContentBytes)
	require.Equal(t, "full", page.Items[0].RawStorage)
	require.Equal(t, 58, page.Items[0].StoredBytes)
	require.NotNil(t, page.Items[0].ContentVaultID)
	require.Equal(t, int64(91), *page.Items[0].ContentVaultID)
	require.Equal(t, "unavailable", page.Items[1].RawStorage)
	require.Zero(t, page.Items[1].StoredBytes)
	require.Nil(t, page.Items[1].ContentVaultID)
}

func TestInstructionV2RepositoryResumesFailedReviewJobBySHA(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	digest := strings.Repeat("a", 64)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtext\(\$1\)\)`).
		WithArgs(digest).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT job.id, job.status, job.observe_only.*FOR UPDATE OF job`).
		WithArgs(digest).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "observe_only", "decision"}).
			AddRow(int64(41), "failed", false, "allow"))
	mock.ExpectExec(`(?s)UPDATE instruction_audit_v2_review_jobs.*SET status = 'retry'.*WHERE id = \$1`).
		WithArgs(int64(41)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	result, err := NewInstructionV2Repository(db).ResumeOrGetReviewJobBySHA(
		context.Background(),
		instructionV2ReviewJobWrite{Vault: instructionV2VaultWrite{SHA256: digest}},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(41), result.JobID)
	require.Equal(t, "retry", result.Status)
	require.Equal(t, "allow", result.SourceDecision)
	require.True(t, result.Requeued)
	require.False(t, result.ResetForEnforcement)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionV2RepositoryReusesProcessingReviewWithoutChangingLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	digest := strings.Repeat("b", 64)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtext\(\$1\)\)`).
		WithArgs(digest).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT job.id, job.status, job.observe_only.*FOR UPDATE OF job`).
		WithArgs(digest).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "observe_only", "decision"}).
			AddRow(int64(42), "processing", false, "block"))
	mock.ExpectCommit()
	mock.ExpectClose()

	result, err := NewInstructionV2Repository(db).ResumeOrGetReviewJobBySHA(
		context.Background(),
		instructionV2ReviewJobWrite{Vault: instructionV2VaultWrite{SHA256: digest}},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "processing", result.Status)
	require.Equal(t, "block", result.SourceDecision)
	require.False(t, result.Requeued)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionV2RepositoryResetsObserveReviewForEnforcement(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	digest := strings.Repeat("c", 64)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtext\(\$1\)\)`).
		WithArgs(digest).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT job.id, job.status, job.observe_only.*FOR UPDATE OF job`).
		WithArgs(digest).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "observe_only", "decision"}).
			AddRow(int64(43), "completed", true, "allow"))
	mock.ExpectExec(`DELETE FROM instruction_audit_v2_review_attempts WHERE job_id = \$1`).
		WithArgs(int64(43)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`(?s)UPDATE instruction_audit_v2_review_jobs.*SET selected_field = \$2, status = 'pending'.*WHERE id = \$1`).
		WithArgs(int64(43), "instructions", "prompt-v2", "criteria-v2", int64(2), false, 12, int64(12)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	result, err := NewInstructionV2Repository(db).ResumeOrGetReviewJobBySHA(
		context.Background(),
		instructionV2ReviewJobWrite{
			Vault:         instructionV2VaultWrite{SHA256: digest, ContentBytes: 12},
			SelectedField: "instructions", PromptVersion: "prompt-v2",
			ReviewCriteria: "criteria-v2", ConfigVersion: 2, SampleBytes: 12,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "pending", result.Status)
	require.True(t, result.ResetForEnforcement)
	require.False(t, result.Requeued)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionV2HashEmptyScopesMarshalAsArrays(t *testing.T) {
	item := InstructionV2Hash{}
	ensureInstructionV2HashCollections(&item)

	require.NotNil(t, item.ScopeIDs)
	require.NotNil(t, item.Scopes)
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, []any{}, decoded["scope_ids"])
	require.Equal(t, []any{}, decoded["scopes"])
}

func TestInstructionV2RepositoryRejectsDeletingReferencedClientProfile(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT built_in FROM instruction_audit_v2_client_profiles WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"built_in"}).AddRow(false))
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*instruction_audit_v2_scopes WHERE client_profile_id = \$1.*\)`).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	mock.ExpectClose()

	version, err := NewInstructionV2Repository(db).DeleteClientProfile(context.Background(), 12, 7)
	require.ErrorIs(t, err, errInstructionV2ProfileInUse)
	require.Zero(t, version)
}

func TestInstructionV2RepositoryDeletesEventNotificationsAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM security_notification_outbox`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM instruction_audit_v2_events WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	mock.ExpectClose()

	deleted, err := NewInstructionV2Repository(db).DeleteEvents(context.Background(), []int64{3, 4})
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionV2RepositoryCleanupDeletesExpiredEventNotificationsAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM instruction_audit_v2_event_evidence WHERE expires_at <= NOW\(\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM security_notification_outbox o.*source_type = 'instruction_audit_v2'.*instruction_audit_v2_events e.*created_at < NOW\(\) - \(\$1 \* INTERVAL '1 day'\)`).
		WithArgs(30).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`(?s)DELETE FROM instruction_audit_v2_events.*created_at < NOW\(\) - \(\$1 \* INTERVAL '1 day'\)`).
		WithArgs(30).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	mock.ExpectClose()

	err = NewInstructionV2Repository(db).Cleanup(context.Background(), InstructionV2Config{EventRetentionDays: 30})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}
