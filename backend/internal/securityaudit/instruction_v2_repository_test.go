package securityaudit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestInstructionV2RepositorySavesScopeSetAtomicallyAndPreservesOnlyExactMatchID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT id FROM groups.*WHERE id = \$1.*FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\) FROM instruction_audit_v2_client_profiles.*WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`(?s)SELECT id, COALESCE\(client_profile_id, 0\).*WHERE group_id = \$1.*FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "client_profile_id"}).
			AddRow(int64(10), int64(0)).
			AddRow(int64(11), int64(2)).
			AddRow(int64(13), int64(3)))
	mock.ExpectExec(`(?s)UPDATE instruction_audit_v2_scopes.*SET enabled = \$2.*WHERE id = \$1`).
		WithArgs(int64(10), true, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE instruction_audit_v2_scopes.*SET enabled = \$2.*WHERE id = \$1`).
		WithArgs(int64(11), true, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO instruction_audit_v2_scopes.*VALUES \(\$1, \$2, \$3`).
		WithArgs(int64(7), int64(4), true, int64(9)).
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec(`DELETE FROM instruction_audit_v2_scopes WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)UPDATE instruction_audit_v2_config.*RETURNING config_version`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"config_version"}).AddRow(int64(22)))
	mock.ExpectQuery(`(?s)SELECT s.id, s.group_id, g.name, g.platform, g.status, s.client_profile_id`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "group_id", "group_name", "group_platform", "group_status", "client_profile_id",
			"client_profile_key", "client_profile_name", "enabled", "effective", "created_by", "updated_by",
			"created_at", "updated_at",
		}).
			AddRow(int64(10), int64(7), "group", "openai", "active", nil, "", "全部客户端", true, true, nil, int64(9), now, now).
			AddRow(int64(11), int64(7), "group", "openai", "active", int64(2), "codex", "Codex", true, true, nil, int64(9), now, now).
			AddRow(int64(12), int64(7), "group", "openai", "active", int64(4), "other", "Other", true, true, int64(9), int64(9), now, now))
	mock.ExpectCommit()
	mock.ExpectClose()

	items, version, err := NewInstructionV2Repository(db).SaveScopeSet(context.Background(), SaveInstructionV2ScopeSetRequest{
		GroupID: 7, ClientProfileIDs: []int64{2, 4}, AllClients: true, Enabled: true,
	}, 9)

	require.NoError(t, err)
	require.Equal(t, int64(22), version)
	require.Len(t, items, 3)
	require.Equal(t, int64(10), items[0].ID)
	require.Nil(t, items[0].ClientProfileID)
	require.Equal(t, int64(11), items[1].ID)
	require.Equal(t, int64(2), *items[1].ClientProfileID)
	require.Equal(t, int64(12), items[2].ID)
	require.Equal(t, int64(4), *items[2].ClientProfileID)
}

func TestInstructionV2RepositoryScopeUpdateCannotMoveIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)UPDATE instruction_audit_v2_scopes.*SET enabled = \$2.*WHERE id = \$1 RETURNING id`).
		WithArgs(int64(10), false, int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectQuery(`(?s)UPDATE instruction_audit_v2_config.*RETURNING config_version`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"config_version"}).AddRow(int64(23)))
	mock.ExpectQuery(`(?s)SELECT s.id, s.group_id, g.name, g.platform, g.status, s.client_profile_id`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "group_id", "group_name", "group_platform", "group_status", "client_profile_id",
			"client_profile_key", "client_profile_name", "enabled", "effective", "created_by", "updated_by",
			"created_at", "updated_at",
		}).AddRow(int64(10), int64(7), "group", "openai", "active", int64(2), "codex", "Codex", false, false, nil, int64(9), now, now))
	mock.ExpectCommit()
	mock.ExpectClose()

	profileID := int64(99)
	item, version, err := NewInstructionV2Repository(db).SaveScope(context.Background(), 10, SaveInstructionV2ScopeRequest{
		GroupID: 88, ClientProfileID: &profileID, Enabled: false,
	}, 9)
	require.NoError(t, err)
	require.Equal(t, int64(23), version)
	require.Equal(t, int64(7), item.GroupID)
	require.Equal(t, int64(2), *item.ClientProfileID)
}

func TestInstructionV2ServiceRejectsScopeIdentityMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT s.id, s.group_id, g.name, g.platform, g.status, s.client_profile_id`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "group_id", "group_name", "group_platform", "group_status", "client_profile_id",
			"client_profile_key", "client_profile_name", "enabled", "effective", "created_by", "updated_by",
			"created_at", "updated_at",
		}).AddRow(int64(10), int64(7), "group", "openai", "active", int64(2), "codex", "Codex", true, true, nil, nil, now, now))
	mock.ExpectClose()

	profileID := int64(3)
	service := &InstructionV2Service{repository: NewInstructionV2Repository(db)}
	_, err = service.SaveAdminScope(context.Background(), 10, SaveInstructionV2ScopeRequest{
		GroupID: 7, ClientProfileID: &profileID, Enabled: true,
	}, 9)
	require.Equal(t, "instruction_audit_v2_scope_identity_immutable", infraErrorReason(err))
}

func TestInstructionV2RepositoryRollsBackEntireScopeSet(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT id FROM groups.*FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\) FROM instruction_audit_v2_client_profiles`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`(?s)SELECT id, COALESCE\(client_profile_id, 0\).*FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "client_profile_id"}).AddRow(int64(11), int64(3)))
	mock.ExpectExec(`(?s)INSERT INTO instruction_audit_v2_scopes`).
		WithArgs(int64(7), int64(2), true, int64(9)).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()
	mock.ExpectClose()

	_, _, err = NewInstructionV2Repository(db).SaveScopeSet(context.Background(), SaveInstructionV2ScopeSetRequest{
		GroupID: 7, ClientProfileIDs: []int64{2, 4}, Enabled: true,
	}, 9)

	require.EqualError(t, err, "insert failed")
}

func TestInstructionV2RepositoryDeletesGroupScopeSetAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT id FROM groups WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectExec(`DELETE FROM instruction_audit_v2_scopes WHERE group_id = \$1`).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectQuery(`(?s)UPDATE instruction_audit_v2_config.*RETURNING config_version`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"config_version"}).AddRow(int64(23)))
	mock.ExpectCommit()
	mock.ExpectClose()

	version, err := NewInstructionV2Repository(db).DeleteScopeSet(context.Background(), 7, 9)

	require.NoError(t, err)
	require.Equal(t, int64(23), version)
}

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
		"stored_bytes", "ai_sampled", "source_event_id", "source_user_id",
		"source_user_email_snapshot", "reviewer_node_id",
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
				"sha256", "identity_utf8_v1", int64(42), "full", 58, false,
				int64(71), int64(81), "source@example.test", nil,
				"", "", nil, "", "", nil, nil, nil, createdAt, createdAt, true, int64(91)).
			AddRow(int64(12), strings.Repeat("b", 64), "digest-only", "", "active", "import", "",
				"sha256", "identity_utf8_v1", int64(0), "unavailable", 0, false,
				nil, nil, "", nil,
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
	require.Equal(t, int64(71), *page.Items[0].SourceEventID)
	require.Equal(t, int64(81), *page.Items[0].SourceUserID)
	require.Equal(t, "source@example.test", page.Items[0].SourceUserEmail)
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
	mock.ExpectQuery(`(?s)SELECT job.id, job.status, job.observe_only.*WHERE job.sha256 = \$1`).
		WithArgs(digest).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "observe_only", "decision"}).
			AddRow(int64(41), "failed", false, "allow"))
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
	mock.ExpectQuery(`(?s)SELECT job.id, job.status, job.observe_only.*WHERE job.sha256 = \$1`).
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
	mock.ExpectQuery(`(?s)SELECT job.id, job.status, job.observe_only.*WHERE job.sha256 = \$1`).
		WithArgs(digest).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "observe_only", "decision"}).
			AddRow(int64(43), "completed", true, "allow"))
	mock.ExpectCommit()
	mock.ExpectClose()

	result, err := NewInstructionV2Repository(db).ResumeOrGetReviewJobBySHA(
		context.Background(),
		instructionV2ReviewJobWrite{
			Vault:         instructionV2VaultWrite{SHA256: digest, ContentBytes: 12},
			SelectedField: "instructions", PromptVersion: "prompt-v2",
			ReviewCriteria: "criteria-v2", ConfigVersion: 2, SampleBytes: 12,
			SourceUserID: instructionV2TestInt64Pointer(55), SourceUserEmail: "source@example.test",
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

func TestInstructionV2RepositoryPersistsEventAndObserveResetAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	digest := strings.Repeat("d", 64)
	userID := int64(55)
	jobID := int64(43)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtext\(\$1\)\)`).
		WithArgs(digest).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)INSERT INTO instruction_audit_v2_events.*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
	mock.ExpectQuery(`(?s)SELECT status, observe_only.*WHERE id = \$1 AND sha256 = \$2.*FOR UPDATE`).
		WithArgs(jobID, digest).
		WillReturnRows(sqlmock.NewRows([]string{"status", "observe_only"}).AddRow("completed", true))
	mock.ExpectExec(`DELETE FROM instruction_audit_v2_review_attempts WHERE job_id = \$1`).
		WithArgs(jobID).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`(?s)UPDATE instruction_audit_v2_review_jobs.*SET selected_field = \$2, source_event_id = \$3.*status = 'pending'.*WHERE id = \$1`).
		WithArgs(jobID, "instructions", int64(91), userID, "source@example.test", "prompt-v2", "criteria-v2", int64(2), false, 12, int64(12)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE instruction_audit_v2_events SET review_job_id = \$2 WHERE id = \$1`).
		WithArgs(int64(91), jobID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	repository := NewInstructionV2Repository(db)
	result, err := repository.PersistInstructionV2Event(context.Background(), instructionV2PersistEvent{
		Event: InstructionV2Event{ReviewJobID: &jobID, UserID: &userID, UserEmail: "source@example.test"},
		ReviewReuse: &instructionV2ReviewReuseWrite{
			Reuse: instructionV2ReviewReuse{JobID: jobID, ResetForEnforcement: true},
			Job: instructionV2ReviewJobWrite{
				Vault:         instructionV2VaultWrite{SHA256: digest, ContentBytes: 12},
				SelectedField: "instructions", PromptVersion: "prompt-v2",
				ReviewCriteria: "criteria-v2", ConfigVersion: 2, SampleBytes: 12,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(91), result.EventID)
	require.NotNil(t, result.JobID)
	require.Equal(t, jobID, *result.JobID)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionV2RepositoryLocksReviewJobBeforeUpdatingVault(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	digest := strings.Repeat("e", 64)
	jobID := int64(44)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtext\(\$1\)\)`).
		WithArgs(digest).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)INSERT INTO instruction_audit_v2_events.*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(92)))
	mock.ExpectQuery(`(?s)SELECT id, observe_only.*WHERE sha256 = \$1.*FOR UPDATE`).
		WithArgs(digest).
		WillReturnRows(sqlmock.NewRows([]string{"id", "observe_only"}).AddRow(jobID, true))
	mock.ExpectQuery(`(?s)INSERT INTO instruction_audit_v2_content_vault.*ON CONFLICT.*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(82)))
	mock.ExpectExec(`DELETE FROM instruction_audit_v2_review_attempts WHERE job_id = \$1`).
		WithArgs(jobID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)UPDATE instruction_audit_v2_review_jobs.*SET content_vault_id = \$2.*status = 'pending'.*WHERE id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE instruction_audit_v2_events SET review_job_id = \$2 WHERE id = \$1`).
		WithArgs(int64(92), jobID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	result, err := NewInstructionV2Repository(db).PersistInstructionV2Event(
		context.Background(),
		instructionV2PersistEvent{
			Event: InstructionV2Event{},
			ReviewJob: &instructionV2ReviewJobWrite{
				Vault: instructionV2VaultWrite{
					SHA256: digest, RawCiphertext: []byte("encrypted"),
					ContentBytes: 12, StoredBytes: 12,
				},
				SelectedField: "instructions", PromptVersion: "prompt-v2",
				ReviewCriteria: "criteria-v2", ConfigVersion: 2, SampleBytes: 12,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result.JobID)
	require.Equal(t, jobID, *result.JobID)
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

func TestInstructionV2RepositoryUpdatesOnlyEnabledForImmutableClientProfile(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	createdAt := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT profile_key, name, description, matchers, priority, built_in, immutable_internal.*FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{
			"profile_key", "name", "description", "matchers", "priority", "built_in", "immutable_internal",
		}).AddRow(InstructionClientModelPortInternal, "ModelPort Internal", "trusted identity", []byte(`[]`), 0, true, true))
	mock.ExpectQuery(`(?s)UPDATE instruction_audit_v2_client_profiles.*SET enabled = \$2.*WHERE id = \$1`).
		WithArgs(int64(5), false, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "built_in", "immutable_internal", "created_at", "updated_at",
		}).AddRow(int64(5), true, true, createdAt, updatedAt))
	mock.ExpectQuery(`(?s)UPDATE instruction_audit_v2_config.*RETURNING config_version`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"config_version"}).AddRow(int64(12)))
	mock.ExpectCommit()
	mock.ExpectClose()

	item, version, err := NewInstructionV2Repository(db).SaveClientProfile(
		context.Background(), 5,
		SaveInstructionV2ClientProfileRequest{
			ProfileKey: "tampered", Name: "tampered", Description: "tampered", Priority: 999,
			Enabled: false, Matchers: []InstructionV2ClientMatcher{{Type: "prefix", Value: "tampered/"}},
		},
		7,
	)
	require.NoError(t, err)
	require.Equal(t, int64(12), version)
	require.Equal(t, InstructionClientModelPortInternal, item.ProfileKey)
	require.Equal(t, "ModelPort Internal", item.Name)
	require.Equal(t, "trusted identity", item.Description)
	require.Zero(t, item.Priority)
	require.Empty(t, item.Matchers)
	require.False(t, item.Enabled)
	require.True(t, item.ImmutableInternal)
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
