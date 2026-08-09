package securityaudit

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

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
	mock.ExpectExec(`(?s)DELETE FROM instruction_audit_v2_hash_scopes.*status = 'candidate'.*candidate_expires_at <= NOW\(\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM instruction_audit_v2_hashes h.*h.status = 'candidate'.*h.candidate_expires_at <= NOW\(\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	err = NewInstructionV2Repository(db).Cleanup(context.Background(), InstructionV2Config{EventRetentionDays: 30})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}
