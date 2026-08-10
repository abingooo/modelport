package securityaudit

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openInstructionV2ReviewIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openInstructionAuditIntegrationDB(t)
	applyInstructionAuditMigration(t, db, "217_instruction_audit_v2.sql")
	applyInstructionAuditMigration(t, db, "219_instruction_audit_v2_review_pipeline.sql")
	applyInstructionAuditMigration(t, db, "219_instruction_audit_v2_review_pipeline.sql")
	return db
}

func TestInstructionV2ReviewMigrationIsIdempotent(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	for _, column := range []string{"review_criteria", "observe_only"} {
		var exists bool
		require.NoError(t, db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema()
				  AND table_name = 'instruction_audit_v2_review_jobs'
				  AND column_name = $1
			)`, column).Scan(&exists))
		require.True(t, exists, column)
	}
}

func TestInstructionV2ReviewConfigRoundTripsRetrySchedule(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	ctx := context.Background()

	config, err := repository.GetConfig(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, config.AsyncRetrySchedule)

	expected := []int{15, 90, 600}
	updated, err := repository.UpdateConfig(ctx, UpdateInstructionV2ConfigRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		Mode:                  config.Mode,
		ReviewCriteria:        config.ReviewCriteria,
		ConfidenceThreshold:   config.ConfidenceThreshold,
		AIInputMaxChars:       config.AIInputMaxChars,
		AIGlobalConcurrency:   config.AIGlobalConcurrency,
		AIQueueWaitMS:         config.AIQueueWaitMS,
		AITotalTimeoutMS:      config.AITotalTimeoutMS,
		AICacheTTLSeconds:     config.AICacheTTLSeconds,
		EventRetentionDays:    config.EventRetentionDays,
		EvidenceRetentionDays: config.EvidenceRetentionDays,
		RawFullMaxBytes:       config.RawFullMaxBytes,
		AllowEmptyFields:      config.AllowEmptyFields,
		AsyncRetrySchedule:    expected,
	}, 0)
	require.NoError(t, err)
	require.Equal(t, expected, updated.AsyncRetrySchedule)
}

func TestInstructionV2ReviewObserveJobResetsForEnforcement(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	ctx := context.Background()
	field := newInstructionV2TextField("observe then enforce", false)

	observeResult := persistInstructionV2ReviewTestJob(t, repository, field, true)
	require.NotNil(t, observeResult.JobID)
	job, err := repository.ClaimReviewJob(ctx, "observe-worker", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	final, completed, err := repository.RecordReviewAttempts(ctx, job.ID, job.LeaseOwner, []InstructionV2ReviewAttempt{
		reviewAttempt("async_1", "pass"),
		reviewAttempt("async_2", "pass"),
		reviewAttempt("async_3", "reject"),
	})
	require.NoError(t, err)
	require.True(t, completed)
	require.Equal(t, "pass", final)

	var trustedCount int
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*) FROM instruction_audit_v2_hashes WHERE sha256 = $1`, field.SHA256).Scan(&trustedCount))
	require.Zero(t, trustedCount)

	enforceResult := persistInstructionV2ReviewTestJob(t, repository, field, false)
	require.NotNil(t, enforceResult.JobID)
	require.Equal(t, *observeResult.JobID, *enforceResult.JobID)
	var status string
	var observeOnly bool
	var passVotes, rejectVotes, attemptCount int
	require.NoError(t, db.QueryRow(`
		SELECT status, observe_only, pass_votes, reject_votes
		FROM instruction_audit_v2_review_jobs WHERE id = $1`, *enforceResult.JobID).Scan(
		&status, &observeOnly, &passVotes, &rejectVotes,
	))
	require.Equal(t, "pending", status)
	require.False(t, observeOnly)
	require.Zero(t, passVotes)
	require.Zero(t, rejectVotes)
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*) FROM instruction_audit_v2_review_attempts WHERE job_id = $1`,
		*enforceResult.JobID).Scan(&attemptCount))
	require.Zero(t, attemptCount)
}

func TestInstructionV2ReviewMajorityCreatesGlobalPolicy(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	ctx := context.Background()

	trustedField := newInstructionV2TextField("majority trusted", false)
	persistInstructionV2ReviewTestJob(t, repository, trustedField, false)
	trustedJob, err := repository.ClaimReviewJob(ctx, "trusted-worker", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, trustedJob)
	final, completed, err := repository.RecordReviewAttempts(ctx, trustedJob.ID, trustedJob.LeaseOwner, []InstructionV2ReviewAttempt{
		reviewAttempt("async_1", "pass"),
		reviewAttempt("async_2", "uncertain"),
		reviewAttempt("async_3", "pass"),
	})
	require.NoError(t, err)
	require.True(t, completed)
	require.Equal(t, "pass", final)
	var globalTrust bool
	require.NoError(t, db.QueryRow(`
		SELECT global_trust FROM instruction_audit_v2_hashes WHERE sha256 = $1`,
		trustedField.SHA256).Scan(&globalTrust))
	require.True(t, globalTrust)

	riskField := newInstructionV2TextField("majority risk", false)
	persistInstructionV2ReviewTestJob(t, repository, riskField, false)
	riskJob, err := repository.ClaimReviewJob(ctx, "risk-worker", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, riskJob)
	final, completed, err = repository.RecordReviewAttempts(ctx, riskJob.ID, riskJob.LeaseOwner, []InstructionV2ReviewAttempt{
		reviewAttempt("async_1", "reject"),
		reviewAttempt("async_2", "pass"),
		reviewAttempt("async_3", "reject"),
	})
	require.NoError(t, err)
	require.True(t, completed)
	require.Equal(t, "reject", final)
	var riskStatus string
	require.NoError(t, db.QueryRow(`
		SELECT status FROM instruction_audit_v2_risk_hashes WHERE sha256 = $1`,
		riskField.SHA256).Scan(&riskStatus))
	require.Equal(t, "active", riskStatus)
}

func TestInstructionV2ReviewLeaseRejectsStaleWorker(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	ctx := context.Background()
	field := newInstructionV2TextField("lease takeover", false)
	persistInstructionV2ReviewTestJob(t, repository, field, false)

	first, err := repository.ClaimReviewJob(ctx, "worker-one", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, first)
	_, err = db.Exec(`
		UPDATE instruction_audit_v2_review_jobs
		SET lease_expires_at = NOW() - INTERVAL '1 second'
		WHERE id = $1`, first.ID)
	require.NoError(t, err)
	second, err := repository.ClaimReviewJob(ctx, "worker-two", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, first.ID, second.ID)

	_, _, err = repository.RecordReviewAttempts(ctx, first.ID, first.LeaseOwner, []InstructionV2ReviewAttempt{
		reviewAttempt("async_1", "pass"),
	})
	require.ErrorIs(t, err, errInstructionV2ReviewLeaseLost)
	_, err = repository.ScheduleReviewRetry(ctx, first.ID, first.LeaseOwner, []int{1}, "stale")
	require.ErrorIs(t, err, errInstructionV2ReviewLeaseLost)

	_, completed, err := repository.RecordReviewAttempts(ctx, second.ID, second.LeaseOwner, []InstructionV2ReviewAttempt{
		reviewAttempt("async_1", "uncertain"),
		reviewAttempt("async_2", "error"),
		reviewAttempt("async_3", "timeout"),
	})
	require.NoError(t, err)
	require.False(t, completed)
	exhausted, err := repository.ScheduleReviewRetry(ctx, second.ID, second.LeaseOwner, []int{1}, "no majority")
	require.NoError(t, err)
	require.False(t, exhausted)
}

func TestInstructionV2ReviewClaimsDifferentPendingJobs(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	persistInstructionV2ReviewTestJob(t, repository, newInstructionV2TextField("first pending", false), false)
	persistInstructionV2ReviewTestJob(t, repository, newInstructionV2TextField("second pending", false), false)

	first, err := repository.ClaimReviewJob(context.Background(), "worker-one", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, first)
	second, err := repository.ClaimReviewJob(context.Background(), "worker-two", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.NotEqual(t, first.ID, second.ID)
}

func persistInstructionV2ReviewTestJob(
	t *testing.T,
	repository *InstructionV2Repository,
	field InstructionV2Field,
	observeOnly bool,
) instructionV2PersistResult {
	t.Helper()
	mode := InstructionV2ModeEnforce
	if observeOnly {
		mode = InstructionV2ModeObserve
	}
	event := InstructionV2Event{
		RequestID: fmt.Sprintf("review-%d", time.Now().UnixNano()),
		ClientKey: "other", ClientName: "Other", Mode: mode,
		Decision: "allow", Outcome: InstructionV2OutcomeAIPass, Reason: "sync_ai_pass",
		Instructions: field, Input1: InstructionV2Field{State: "missing"},
		SelectedField: "instructions", SelectedSHA256: field.SHA256,
		AIResult: "pass", AIReviewedField: "instructions", BodyBytes: field.Bytes,
		ConfigVersion: 1, EvidenceStatus: "not_stored",
	}
	result, err := repository.PersistInstructionV2Event(context.Background(), instructionV2PersistEvent{
		Event: event,
		ReviewJob: &instructionV2ReviewJobWrite{
			Vault: instructionV2VaultWrite{
				SHA256: field.SHA256, ObservedField: "instructions",
				RawCiphertext: []byte("encrypted:" + field.Plaintext),
				ContentBytes:  field.Bytes, StoredBytes: len([]byte(field.Plaintext)),
			},
			SelectedField: "instructions", PromptVersion: "test-prompt-v1",
			ReviewCriteria: "test criteria", ConfigVersion: 1,
			ObserveOnly: observeOnly, SampleBytes: len([]byte(field.Plaintext)),
		},
	})
	require.NoError(t, err)
	return result
}

func reviewAttempt(slot, result string) InstructionV2ReviewAttempt {
	confidence := 0.99
	if result == "uncertain" || result == "error" || result == "timeout" {
		confidence = 0
	}
	return InstructionV2ReviewAttempt{
		NodeSlot: slot, NodeName: slot, ReviewerModel: "review-model",
		Result: result, Confidence: confidence, Reason: result,
		Category: "test", Sampled: false, LatencyMS: 1,
	}
}
