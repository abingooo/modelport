package securityaudit

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestInstructionRepositoryListEventsAppliesLegacyUserAndModelFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	filterClause := `(?s).*\$11 = 0 OR e\.user_id = \$11.*\$12 = '%%' OR e\.model ILIKE \$12.*cardinality\(\$13::TEXT\[\]\).*\$14 = 0 OR e\.id = \$14.*`
	args := []driver.Value{
		"", "%%", nil, nil,
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), int64(42), "%gpt-5%", sqlmock.AnyArg(), int64(0),
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)") + filterClause).
		WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(`(?s)SELECT e\.id.*` + filterClause + `.*LIMIT \$15 OFFSET \$16`).
		WithArgs(append(args, 20, 0)...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "user_id", "user_email_snapshot", "api_key_id", "group_id",
			"group_name_snapshot", "client_type", "client_user_agent", "model", "endpoint", "stage", "instructions_present",
			"instructions_sha256", "instructions_result", "input1_present", "input1_sha256",
			"input1_result", "decision", "reason", "rule_set_ids", "config_version", "latency_ms",
			"evidence_status", "evidence_expires_at", "user_notification_status",
			"ops_notification_status", "created_at",
		}).AddRow(
			int64(1), "request-1", int64(42), "user@example.test", nil, int64(7),
			"test-group", "codex_cli", "codex_cli_rs/0.145.0", "gpt-5", "/v1/responses", "http", true, "", "mismatch",
			false, "", "missing", "blocked", "hash_mismatch", []byte("[]"), int64(1), 2,
			"stored", nil, "pending", "sent", time.Now().UTC(),
		))
	mock.ExpectClose()

	page, err := NewInstructionRepository(db).ListEvents(context.Background(), 1, 20, InstructionEventFilter{
		UserID: 42,
		Model:  "gpt-5",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	require.EqualValues(t, 42, *page.Items[0].UserID)
	require.Equal(t, InstructionClientCodexCLI, page.Items[0].ClientType)
}

func TestInstructionRepositoryDeleteHashRollsBackWhenReferenced(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM instruction_audit_hashes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM instruction_audit_rule_set_hashes WHERE hash_id = \$1`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectRollback()

	version, references, err := NewInstructionRepository(db).DeleteHash(context.Background(), 9)
	require.NoError(t, err)
	require.Zero(t, version)
	require.EqualValues(t, 2, references)
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionRepositoryDeleteRuleSetRollsBackWhenBound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM instruction_audit_rule_sets WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	mock.ExpectQuery(`(?s)SELECT.*instruction_audit_group_bindings.*instruction_audit_bindings`).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))
	mock.ExpectRollback()

	version, references, err := NewInstructionRepository(db).DeleteRuleSet(context.Background(), 11)
	require.NoError(t, err)
	require.Zero(t, version)
	require.EqualValues(t, 3, references)
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionRepositoryDeleteEventsRemovesUnifiedNotificationsFirst(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM security_notification_outbox`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectQuery(`DELETE FROM instruction_audit_events WHERE id = ANY\(\$1\) RETURNING id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(4)).AddRow(int64(7)))
	mock.ExpectCommit()

	result, err := NewInstructionRepository(db).DeleteEventsByIDs(context.Background(), []int64{7, 4, 7})
	require.NoError(t, err)
	require.EqualValues(t, 2, result.DeletedEvents)
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionRepositoryAddHashesToRuleSetCreatesAndReactivatesAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	const actorID int64 = 5
	const ruleSetID int64 = 12
	expiredAt := time.Now().UTC().Add(-time.Hour)
	firstDigest := sha256Hex("existing-expired")
	secondDigest := sha256Hex("new-value")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM instruction_audit_rule_sets WHERE id = \$1 FOR UPDATE`).
		WithArgs(ruleSetID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(ruleSetID))
	mock.ExpectQuery(`(?s)SELECT id, status, valid_from, valid_until.*FROM instruction_audit_hashes WHERE digest = \$1 FOR UPDATE`).
		WithArgs(firstDigest).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "valid_from", "valid_until"}).
			AddRow(int64(21), "active", nil, expiredAt))
	mock.ExpectExec(`UPDATE instruction_audit_hashes`).
		WithArgs(int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO instruction_audit_rule_set_hashes`).
		WithArgs(ruleSetID, int64(21), actorID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id, status, valid_from, valid_until.*FROM instruction_audit_hashes WHERE digest = \$1 FOR UPDATE`).
		WithArgs(secondDigest).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO instruction_audit_hashes`).
		WithArgs(secondDigest, "new", "from event", "input1", "codex_cli", "", actorID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(22)))
	mock.ExpectExec(`INSERT INTO instruction_audit_rule_set_hashes`).
		WithArgs(ruleSetID, int64(22), actorID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE instruction_audit_rule_sets`).
		WithArgs(ruleSetID, actorID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`UPDATE instruction_audit_state`).
		WillReturnRows(sqlmock.NewRows([]string{"config_version"}).AddRow(int64(31)))
	mock.ExpectCommit()

	result, err := NewInstructionRepository(db).AddHashesToRuleSet(context.Background(), ruleSetID, []CreateInstructionHashRequest{
		{Digest: firstDigest, Name: "existing", Status: "active"},
		{Digest: secondDigest, Name: "new", Note: "from event", ObservedSource: "input1", ClientName: "codex_cli", Status: "active"},
	}, actorID)
	require.NoError(t, err)
	require.Equal(t, []int64{21, 22}, result.HashIDs)
	require.Equal(t, 1, result.CreatedHashes)
	require.Equal(t, 1, result.ActivatedHashes)
	require.Equal(t, 2, result.AttachedHashes)
	require.EqualValues(t, 31, result.ConfigVersion)
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionEventFilterHashIsCanonicalAndSnapshotBound(t *testing.T) {
	from := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	to := from.Add(time.Hour)
	first := InstructionEventFilter{
		Query: " user@example.test ", From: &from, To: &to,
		GroupIDs: []int64{9, 3, 9}, ClientTypes: []string{"other", "codex_cli"},
		Reasons: []string{"hash_mismatch", "fields_missing"},
	}
	second := InstructionEventFilter{
		Query: "user@example.test", From: &from, To: &to,
		GroupIDs: []int64{3, 9}, ClientTypes: []string{"codex_cli", "other"},
		Reasons: []string{"fields_missing", "hash_mismatch"},
	}

	require.Equal(t, instructionEventFilterHash(first, 100), instructionEventFilterHash(second, 100))
	require.NotEqual(t, instructionEventFilterHash(first, 100), instructionEventFilterHash(first, 101))
}
