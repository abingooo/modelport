package securityaudit

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestShouldStorePromptAuditEvent(t *testing.T) {
	tests := []struct {
		name            string
		storePassEvents bool
		decision        EventDecision
		want            bool
	}{
		{name: "pass disabled", storePassEvents: false, decision: EventPass, want: false},
		{name: "flag disabled", storePassEvents: false, decision: EventFlag, want: true},
		{name: "critical disabled", storePassEvents: false, decision: EventCritical, want: true},
		{name: "pass enabled", storePassEvents: true, decision: EventPass, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStorePromptAuditEvent(tt.decision, tt.storePassEvents); got != tt.want {
				t.Fatalf("shouldStorePromptAuditEvent(%q, %t) = %t, want %t", tt.decision, tt.storePassEvents, got, tt.want)
			}
		})
	}
}

func TestClaimNextPromptAuditJobOnlyClaimsCurrentModelContract(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	now := time.Date(2026, time.August, 15, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`
		WITH candidate AS (
			SELECT id FROM prompt_audit_jobs
			WHERE status IN ('queued','retry') AND next_attempt_at <= $1
			  AND model_contract_version = $2`)).
		WithArgs(now, PromptAuditModelContractVersion).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	job, claimed, err := NewPostgreSQLRepository(db).ClaimNextJob(context.Background(), now)
	require.NoError(t, err)
	require.False(t, claimed)
	require.Nil(t, job)
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPausePromptAuditJobRestoresClaimAttemptWithFence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	repo := NewPostgreSQLRepository(db)
	now := time.Date(2026, time.August, 15, 8, 30, 0, 0, time.UTC)
	query := regexp.QuoteMeta(`
		UPDATE prompt_audit_jobs SET status='retry', attempts=GREATEST(attempts-1, 0), next_attempt_at=$3,
			processing_started_at=NULL, updated_at=NOW(), last_error_code=$4, last_error_message=$5
		WHERE id=$1 AND status='processing' AND claim_version=$2`)
	_, message := sanitizeStoredError(promptAuditWorkerPausedCode)

	mock.ExpectExec(query).
		WithArgs(int64(51), int64(7), now, promptAuditWorkerPausedCode, message).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.Pause(context.Background(), 51, 7, now, promptAuditWorkerPausedCode, "ignored"))

	mock.ExpectExec(query).
		WithArgs(int64(51), int64(6), now, promptAuditWorkerPausedCode, message).
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, repo.Pause(context.Background(), 51, 6, now, promptAuditWorkerPausedCode, "ignored"), ErrLeaseLost)

	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}
