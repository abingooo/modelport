package securityaudit

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func openInstructionV2ReviewIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openInstructionAuditIntegrationDB(t)
	applyInstructionAuditMigration(t, db, "217_instruction_audit_v2.sql")
	applyInstructionAuditMigration(t, db, "219_instruction_audit_v2_review_pipeline.sql")
	applyInstructionAuditMigration(t, db, "219_instruction_audit_v2_review_pipeline.sql")
	applyInstructionAuditMigration(t, db, "220_instruction_audit_v2_source_accounts.sql")
	applyInstructionAuditMigration(t, db, "220_instruction_audit_v2_source_accounts.sql")
	return db
}

func openInstructionV2ReviewConcurrentDB(
	t *testing.T,
	db *sql.DB,
	applicationName string,
) *sql.DB {
	t.Helper()
	var schema string
	require.NoError(t, db.QueryRow(`SELECT current_schema()`).Scan(&schema))
	dsnURL, err := url.Parse(strings.TrimSpace(os.Getenv(instructionAuditPostgresTestEnv)))
	require.NoError(t, err)
	query := dsnURL.Query()
	query.Set("search_path", schema)
	query.Set("application_name", applicationName)
	dsnURL.RawQuery = query.Encode()
	concurrentDB, err := sql.Open("postgres", dsnURL.String())
	require.NoError(t, err)
	concurrentDB.SetMaxOpenConns(4)
	concurrentDB.SetMaxIdleConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, concurrentDB.PingContext(ctx))
	t.Cleanup(func() { require.NoError(t, concurrentDB.Close()) })
	return concurrentDB
}

type instructionV2PersistTestOutcome struct {
	result instructionV2PersistResult
	err    error
}

func TestInstructionV2ReviewMigrationIsIdempotent(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	for _, column := range []string{
		"review_criteria", "observe_only", "source_user_id", "source_user_email_snapshot",
	} {
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

func TestInstructionV2SourceAccountMigrationBackfillsHistoricalRows(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	applyInstructionAuditMigration(t, db, "217_instruction_audit_v2.sql")
	applyInstructionAuditMigration(t, db, "219_instruction_audit_v2_review_pipeline.sql")

	const sourceEmail = "historical-source@example.test"
	userID := insertInstructionAuditUser(t, db, sourceEmail, "user")

	var eventID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_v2_events
			(request_id, user_id, user_email_snapshot, mode, decision, outcome, reason, config_version)
		VALUES ('historical-source-event', $1, $2, 'enforce', 'allow', 'ai_pass', 'sync_ai_pass', 1)
		RETURNING id`, userID, sourceEmail).Scan(&eventID))

	var hashID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_v2_hashes
			(sha256, status, source, observed_field, source_event_id, global_trust)
		VALUES ($1, 'active', 'ai_review', 'instructions', $2, TRUE)
		RETURNING id`, strings.Repeat("a", 64), eventID).Scan(&hashID))

	insertVault := func(digest string) int64 {
		t.Helper()
		var vaultID int64
		require.NoError(t, db.QueryRow(`
			INSERT INTO instruction_audit_v2_content_vault
				(sha256, raw_ciphertext, content_bytes, stored_bytes, observed_field)
			VALUES ($1, $2, 8, 8, 'instructions')
			RETURNING id`, digest, []byte("encrypted")).Scan(&vaultID))
		return vaultID
	}

	riskDigest := strings.Repeat("b", 64)
	riskVaultID := insertVault(riskDigest)
	var riskID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_v2_risk_hashes
			(sha256, content_vault_id, observed_field, source, source_event_id)
		VALUES ($1, $2, 'instructions', 'sync_ai', $3)
		RETURNING id`, riskDigest, riskVaultID, eventID).Scan(&riskID))

	jobDigest := strings.Repeat("c", 64)
	jobVaultID := insertVault(jobDigest)
	var jobID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_v2_review_jobs
			(sha256, content_vault_id, selected_field, source_event_id,
			 prompt_version, config_version, content_bytes)
		VALUES ($1, $2, 'instructions', $3, 'historical-prompt', 1, 8)
		RETURNING id`, jobDigest, jobVaultID, eventID).Scan(&jobID))

	for _, table := range []string{
		"instruction_audit_v2_hashes",
		"instruction_audit_v2_risk_hashes",
		"instruction_audit_v2_review_jobs",
	} {
		var exists bool
		require.NoError(t, db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema()
				  AND table_name = $1
				  AND column_name = 'source_user_id'
			)`, table).Scan(&exists))
		require.False(t, exists, table)
	}

	applyInstructionAuditMigration(t, db, "220_instruction_audit_v2_source_accounts.sql")
	applyInstructionAuditMigration(t, db, "220_instruction_audit_v2_source_accounts.sql")

	rows := []struct {
		name  string
		query string
		id    int64
	}{
		{
			name:  "trusted hash",
			query: `SELECT source_event_id, source_user_id, source_user_email_snapshot FROM instruction_audit_v2_hashes WHERE id = $1`,
			id:    hashID,
		},
		{
			name:  "risk hash",
			query: `SELECT source_event_id, source_user_id, source_user_email_snapshot FROM instruction_audit_v2_risk_hashes WHERE id = $1`,
			id:    riskID,
		},
		{
			name:  "review job",
			query: `SELECT source_event_id, source_user_id, source_user_email_snapshot FROM instruction_audit_v2_review_jobs WHERE id = $1`,
			id:    jobID,
		},
	}
	assertSources := func(expectedEventID, expectedUserID sql.NullInt64) {
		t.Helper()
		for _, row := range rows {
			var sourceEventID, sourceUserID sql.NullInt64
			var emailSnapshot string
			require.NoError(t, db.QueryRow(row.query, row.id).Scan(
				&sourceEventID, &sourceUserID, &emailSnapshot,
			), row.name)
			require.Equal(t, expectedEventID, sourceEventID, row.name)
			require.Equal(t, expectedUserID, sourceUserID, row.name)
			require.Equal(t, sourceEmail, emailSnapshot, row.name)
		}
	}

	assertSources(
		sql.NullInt64{Int64: eventID, Valid: true},
		sql.NullInt64{Int64: userID, Valid: true},
	)

	_, err := db.Exec(`DELETE FROM instruction_audit_v2_events WHERE id = $1`, eventID)
	require.NoError(t, err)
	assertSources(
		sql.NullInt64{},
		sql.NullInt64{Int64: userID, Valid: true},
	)
	_, err = db.Exec(`DELETE FROM users WHERE id = $1`, userID)
	require.NoError(t, err)
	assertSources(sql.NullInt64{}, sql.NullInt64{})
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

func TestInstructionV2ReviewFailedJobResumesWithoutDiscardingVotes(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	ctx := context.Background()
	field := newInstructionV2TextField("resume failed review", false)
	result := persistInstructionV2ReviewTestJob(t, repository, field, false)
	require.NotNil(t, result.JobID)

	job, err := repository.ClaimReviewJob(ctx, "failed-worker", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	_, completed, err := repository.RecordReviewAttempts(ctx, job.ID, job.LeaseOwner, []InstructionV2ReviewAttempt{
		reviewAttempt("async_1", "pass"),
		reviewAttempt("async_2", "reject"),
		reviewAttempt("async_3", "timeout"),
	})
	require.NoError(t, err)
	require.False(t, completed)
	exhausted, err := repository.ScheduleReviewRetry(ctx, job.ID, job.LeaseOwner, nil, "async_3 timeout")
	require.NoError(t, err)
	require.True(t, exhausted)

	reuseWrite := instructionV2ReviewJobWrite{
		Vault: instructionV2VaultWrite{SHA256: field.SHA256, ContentBytes: field.Bytes},
	}
	reused, err := repository.ResumeOrGetReviewJobBySHA(ctx, reuseWrite)
	require.NoError(t, err)
	require.NotNil(t, reused)
	require.Equal(t, job.ID, reused.JobID)
	require.Equal(t, "retry", reused.Status)
	require.True(t, reused.Requeued)

	var status string
	require.NoError(t, db.QueryRow(`
		SELECT status FROM instruction_audit_v2_review_jobs WHERE id = $1`, job.ID).Scan(&status))
	require.Equal(t, "failed", status)

	retryEvent := instructionV2ReviewTestEvent(field)
	retryEvent.ReviewJobID = &job.ID
	retryResult, err := repository.PersistInstructionV2Event(ctx, instructionV2PersistEvent{
		Event: retryEvent,
		ReviewReuse: &instructionV2ReviewReuseWrite{
			Reuse: *reused,
			Job:   reuseWrite,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, retryResult.JobID)
	require.Equal(t, job.ID, *retryResult.JobID)

	var retryRound, attemptCount int
	require.NoError(t, db.QueryRow(`
		SELECT status, retry_round FROM instruction_audit_v2_review_jobs WHERE id = $1`,
		job.ID).Scan(&status, &retryRound))
	require.Equal(t, "retry", status)
	require.Zero(t, retryRound)
	var sourceEventID int64
	require.NoError(t, db.QueryRow(`
		SELECT source_event_id FROM instruction_audit_v2_review_jobs WHERE id = $1`,
		job.ID).Scan(&sourceEventID))
	require.Equal(t, retryResult.EventID, sourceEventID)
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*) FROM instruction_audit_v2_review_attempts WHERE job_id = $1`,
		job.ID).Scan(&attemptCount))
	require.Equal(t, 3, attemptCount)
}

func TestInstructionV2ReviewReusePersistsConcurrentEventsWithoutLockInversion(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	field := newInstructionV2TextField("concurrent failed review reuse", false)
	initial := persistInstructionV2ReviewTestJob(t, NewInstructionV2Repository(db), field, false)
	require.NotNil(t, initial.JobID)
	jobID := *initial.JobID
	_, err := db.Exec(`
		UPDATE instruction_audit_v2_review_jobs
		SET status = 'failed', last_error = 'endpoint timeout'
		WHERE id = $1`, jobID)
	require.NoError(t, err)

	applicationName := fmt.Sprintf("instruction_v2_review_reuse_%d", time.Now().UnixNano())
	concurrentDB := openInstructionV2ReviewConcurrentDB(t, db, applicationName)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	blocker, err := concurrentDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	_, err = blocker.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, field.SHA256)
	require.NoError(t, err)

	outcomes := make(chan instructionV2PersistTestOutcome, 2)
	repository := NewInstructionV2Repository(concurrentDB)
	for index := range 2 {
		go func(index int) {
			event := instructionV2ReviewTestEvent(field)
			event.RequestID = fmt.Sprintf("concurrent-review-reuse-%d", index)
			event.ReviewJobID = &jobID
			result, persistErr := repository.PersistInstructionV2Event(ctx, instructionV2PersistEvent{
				Event: event,
				ReviewReuse: &instructionV2ReviewReuseWrite{
					Reuse: instructionV2ReviewReuse{JobID: jobID, Requeued: true},
					Job: instructionV2ReviewJobWrite{
						Vault: instructionV2VaultWrite{SHA256: field.SHA256, ContentBytes: field.Bytes},
					},
				},
			})
			outcomes <- instructionV2PersistTestOutcome{result: result, err: persistErr}
		}(index)
	}

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		require.NoError(t, concurrentDB.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM pg_locks locks
			JOIN pg_stat_activity activity ON activity.pid = locks.pid
			WHERE activity.application_name = $1
			  AND locks.locktype = 'advisory'
			  AND NOT locks.granted`, applicationName).Scan(&waiting))
		if waiting == 2 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for concurrent review reuse writers: got %d", waiting)
		}
	}

	var lockedStatus string
	require.NoError(t, blocker.QueryRowContext(ctx, `
		SELECT status FROM instruction_audit_v2_review_jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&lockedStatus))
	require.Equal(t, "failed", lockedStatus)
	require.NoError(t, blocker.Commit())

	eventIDs := make([]int64, 0, 2)
	for range 2 {
		select {
		case outcome := <-outcomes:
			require.NoError(t, outcome.err)
			eventIDs = append(eventIDs, outcome.result.EventID)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}

	var status string
	var sourceEventID int64
	require.NoError(t, concurrentDB.QueryRowContext(ctx, `
		SELECT status, source_event_id
		FROM instruction_audit_v2_review_jobs WHERE id = $1`, jobID).Scan(&status, &sourceEventID))
	require.Equal(t, "retry", status)
	require.Contains(t, eventIDs, sourceEventID)
	var eventCount int
	require.NoError(t, concurrentDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM instruction_audit_v2_events
		WHERE request_id IN ('concurrent-review-reuse-0', 'concurrent-review-reuse-1')
		  AND review_job_id = $1`, jobID).Scan(&eventCount))
	require.Equal(t, 2, eventCount)
}

func TestInstructionV2ReviewJobPersistLocksJobBeforeVault(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	field := newInstructionV2TextField("observe review reset lock ordering", false)
	initial := persistInstructionV2ReviewTestJob(t, NewInstructionV2Repository(db), field, true)
	require.NotNil(t, initial.JobID)
	jobID := *initial.JobID
	var vaultID int64
	require.NoError(t, db.QueryRow(`
		SELECT content_vault_id FROM instruction_audit_v2_review_jobs WHERE id = $1`, jobID).Scan(&vaultID))

	applicationName := fmt.Sprintf("instruction_v2_review_job_%d", time.Now().UnixNano())
	concurrentDB := openInstructionV2ReviewConcurrentDB(t, db, applicationName)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	blocker, err := concurrentDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	var status string
	require.NoError(t, blocker.QueryRowContext(ctx, `
		SELECT status FROM instruction_audit_v2_review_jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&status))

	outcome := make(chan instructionV2PersistTestOutcome, 1)
	go func() {
		event := instructionV2ReviewTestEvent(field)
		event.RequestID = "review-job-lock-order"
		result, persistErr := NewInstructionV2Repository(concurrentDB).PersistInstructionV2Event(
			ctx,
			instructionV2PersistEvent{
				Event: event,
				ReviewJob: &instructionV2ReviewJobWrite{
					Vault: instructionV2VaultWrite{
						SHA256: field.SHA256, ObservedField: "instructions",
						RawCiphertext: []byte("encrypted:" + field.Plaintext),
						ContentBytes:  field.Bytes, StoredBytes: len([]byte(field.Plaintext)),
					},
					SelectedField: "instructions", PromptVersion: "test-prompt-v2",
					ReviewCriteria: "test criteria", ConfigVersion: 2,
					SampleBytes: len([]byte(field.Plaintext)),
				},
			},
		)
		outcome <- instructionV2PersistTestOutcome{result: result, err: persistErr}
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		require.NoError(t, concurrentDB.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM pg_locks locks
			JOIN pg_stat_activity activity ON activity.pid = locks.pid
			WHERE activity.application_name = $1
			  AND locks.locktype <> 'advisory'
			  AND NOT locks.granted`, applicationName).Scan(&waiting))
		if waiting > 0 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("timed out waiting for review job row lock")
		}
	}

	_, err = blocker.ExecContext(ctx, `
		UPDATE instruction_audit_v2_content_vault SET updated_at = NOW() WHERE id = $1`, vaultID)
	require.NoError(t, err)
	require.NoError(t, blocker.Commit())

	select {
	case persisted := <-outcome:
		require.NoError(t, persisted.err)
		require.NotNil(t, persisted.result.JobID)
		require.Equal(t, jobID, *persisted.result.JobID)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var observeOnly bool
	require.NoError(t, concurrentDB.QueryRowContext(ctx, `
		SELECT observe_only FROM instruction_audit_v2_review_jobs WHERE id = $1`, jobID).Scan(&observeOnly))
	require.False(t, observeOnly)
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

func TestInstructionV2SourceAccountsUseRiskLatestAndTrustedFirstSemantics(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	ctx := context.Background()
	firstUserID := insertInstructionAuditUser(t, db, "first-source@example.test", "user")
	latestUserID := insertInstructionAuditUser(t, db, "latest-source@example.test", "user")

	riskField := newInstructionV2TextField("same risk reviewed twice", false)
	firstRisk := persistInstructionV2SourcePolicyTest(
		t, repository, riskField, firstUserID, "first-source@example.test", false,
	)
	latestRisk := persistInstructionV2SourcePolicyTest(
		t, repository, riskField, latestUserID, "latest-source@example.test", false,
	)
	require.Equal(t, firstRisk.RiskID, latestRisk.RiskID)
	risk, _, err := repository.GetRiskHash(ctx, *latestRisk.RiskID)
	require.NoError(t, err)
	require.Equal(t, latestRisk.EventID, *risk.SourceEventID)
	require.Equal(t, latestUserID, *risk.SourceUserID)
	require.Equal(t, "latest-source@example.test", risk.SourceUserEmail)

	_, trustedFromRisk, _, err := repository.UpdateRiskHash(ctx, risk.ID, UpdateInstructionV2RiskHashRequest{
		Action: "confirm_safe",
	}, 0)
	require.NoError(t, err)
	require.NotNil(t, trustedFromRisk)
	require.Equal(t, latestRisk.EventID, *trustedFromRisk.SourceEventID)
	require.Equal(t, latestUserID, *trustedFromRisk.SourceUserID)
	require.Equal(t, "latest-source@example.test", trustedFromRisk.SourceUserEmail)

	trustedField := newInstructionV2TextField("same trusted reviewed twice", false)
	firstTrusted := persistInstructionV2SourcePolicyTest(
		t, repository, trustedField, firstUserID, "first-source@example.test", true,
	)
	latestTrusted := persistInstructionV2SourcePolicyTest(
		t, repository, trustedField, latestUserID, "latest-source@example.test", true,
	)
	require.Equal(t, firstTrusted.HashID, latestTrusted.HashID)
	trusted, _, err := repository.GetHash(ctx, *latestTrusted.HashID)
	require.NoError(t, err)
	require.Equal(t, firstTrusted.EventID, *trusted.SourceEventID)
	require.Equal(t, firstUserID, *trusted.SourceUserID)
	require.Equal(t, "first-source@example.test", trusted.SourceUserEmail)

	_, err = db.Exec(`DELETE FROM instruction_audit_v2_events WHERE id = $1`, firstTrusted.EventID)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM users WHERE id = $1`, firstUserID)
	require.NoError(t, err)
	trusted, _, err = repository.GetHash(ctx, *latestTrusted.HashID)
	require.NoError(t, err)
	require.Nil(t, trusted.SourceEventID)
	require.Nil(t, trusted.SourceUserID)
	require.Equal(t, "first-source@example.test", trusted.SourceUserEmail)

	imported, _, err := repository.SaveManualHash(ctx, instructionV2ManualHashWrite{
		SHA256: strings.Repeat("f", 64), Name: "digest import", Status: "active",
		Source: "import", GlobalTrust: true,
	}, 0)
	require.NoError(t, err)
	require.Nil(t, imported.SourceEventID)
	require.Nil(t, imported.SourceUserID)
	require.Empty(t, imported.SourceUserEmail)
}

func TestInstructionV2AsyncReviewCarriesDurableSourceAccount(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "async-source@example.test", "user")
	field := newInstructionV2TextField("async source snapshot", false)
	event := instructionV2ReviewTestEvent(field)
	event.UserID = &userID
	event.UserEmail = "async-source@example.test"
	result, err := repository.PersistInstructionV2Event(ctx, instructionV2PersistEvent{
		Event: event,
		ReviewJob: &instructionV2ReviewJobWrite{
			Vault: instructionV2VaultWrite{
				SHA256: field.SHA256, ObservedField: "instructions",
				RawCiphertext: []byte("encrypted:" + field.Plaintext),
				ContentBytes:  field.Bytes, StoredBytes: len([]byte(field.Plaintext)),
			},
			SelectedField: "instructions", PromptVersion: "test-prompt-v1",
			ReviewCriteria: "test criteria", ConfigVersion: 1,
			SampleBytes: len([]byte(field.Plaintext)),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result.JobID)

	job, err := repository.ClaimReviewJob(ctx, "source-worker", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.Equal(t, result.EventID, *job.SourceEventID)
	require.Equal(t, userID, *job.SourceUserID)
	require.Equal(t, "async-source@example.test", job.SourceUserEmail)
	_, completed, err := repository.RecordReviewAttempts(ctx, job.ID, job.LeaseOwner, []InstructionV2ReviewAttempt{
		reviewAttempt("async_1", "pass"),
		reviewAttempt("async_2", "pass"),
	})
	require.NoError(t, err)
	require.True(t, completed)

	var hashID int64
	require.NoError(t, db.QueryRow(`
		SELECT id FROM instruction_audit_v2_hashes WHERE sha256 = $1`, field.SHA256).Scan(&hashID))
	trusted, _, err := repository.GetHash(ctx, hashID)
	require.NoError(t, err)
	require.Equal(t, result.EventID, *trusted.SourceEventID)
	require.Equal(t, userID, *trusted.SourceUserID)
	require.Equal(t, "async-source@example.test", trusted.SourceUserEmail)
}

func TestInstructionV2TrustAdminEventCarriesRequestSourceAccount(t *testing.T) {
	db := openInstructionV2ReviewIntegrationDB(t)
	repository := NewInstructionV2Repository(db)
	ctx := context.Background()
	userID := insertInstructionAuditUser(t, db, "event-source@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "event-source-group")
	scope, _, err := repository.SaveScope(ctx, 0, SaveInstructionV2ScopeRequest{
		GroupID: groupID, Enabled: true,
	}, 0)
	require.NoError(t, err)
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{
			EncryptionKey: strings.Repeat("42", 32), EncryptionKeyConfigured: true,
		},
	})
	require.NoError(t, err)
	field := newInstructionV2TextField("trusted from audit event", false)
	evidenceCiphertext, err := cipher.Encrypt("instructions", field.SHA256, field.Plaintext)
	require.NoError(t, err)
	event := instructionV2ReviewTestEvent(field)
	event.UserID = &userID
	event.UserEmail = "event-source@example.test"
	event.GroupID = &groupID
	event.GroupName = "event-source-group"
	event.ScopeID = &scope.ID
	event.EvidenceStatus = "stored"
	persisted, err := repository.PersistInstructionV2Event(ctx, instructionV2PersistEvent{
		Event: event,
		Evidence: []instructionV2EvidenceWrite{{
			FieldName: "instructions", SHA256: field.SHA256, StorageKind: "full",
			Ciphertext: evidenceCiphertext, ContentBytes: field.Bytes,
			StoredBytes: len([]byte(field.Plaintext)), ExpiresAt: time.Now().Add(time.Hour),
		}},
	})
	require.NoError(t, err)

	service := ProvideInstructionV2Service(repository, nil, cipher, nil, nil, nil)
	result, err := service.TrustAdminEvent(ctx, persisted.EventID, InstructionV2TrustEventRequest{
		Fields: []string{"instructions"}, GlobalTrust: true,
	}, 0)
	require.NoError(t, err)
	require.Len(t, result.Hashes, 1)
	require.Equal(t, persisted.EventID, *result.Hashes[0].SourceEventID)
	require.Equal(t, userID, *result.Hashes[0].SourceUserID)
	require.Equal(t, "event-source@example.test", result.Hashes[0].SourceUserEmail)
}

func persistInstructionV2SourcePolicyTest(
	t *testing.T,
	repository *InstructionV2Repository,
	field InstructionV2Field,
	userID int64,
	email string,
	trusted bool,
) instructionV2PersistResult {
	t.Helper()
	event := instructionV2ReviewTestEvent(field)
	event.UserID = &userID
	event.UserEmail = email
	write := instructionV2PersistEvent{Event: event}
	vault := instructionV2VaultWrite{
		SHA256: field.SHA256, ObservedField: "instructions",
		RawCiphertext: []byte("encrypted:" + field.Plaintext),
		ContentBytes:  field.Bytes, StoredBytes: len([]byte(field.Plaintext)),
	}
	if trusted {
		write.Trusted = &instructionV2TrustedWrite{
			Vault: vault, Source: "ai_review", ObservedField: "instructions",
			ReviewerModel: "review-model", PromptVersion: "test-prompt-v1",
			Confidence: 0.99, ReviewReason: "safe", ReviewCategory: "safe", GlobalTrust: true,
		}
	} else {
		event.Decision = "block"
		event.Outcome = InstructionV2OutcomeBlocked
		event.Reason = "sync_ai_reject"
		event.AIResult = "reject"
		write.Event = event
		write.Risk = &instructionV2RiskWrite{
			Vault: vault, Source: "sync_ai", ObservedField: "instructions",
			ReviewerModel: "review-model", PromptVersion: "test-prompt-v1",
			Confidence: 0.99, ReviewReason: "risk", ReviewCategory: "risk",
		}
	}
	result, err := repository.PersistInstructionV2Event(context.Background(), write)
	require.NoError(t, err)
	return result
}

func instructionV2ReviewTestEvent(field InstructionV2Field) InstructionV2Event {
	return InstructionV2Event{
		RequestID: fmt.Sprintf("review-%d", time.Now().UnixNano()),
		ClientKey: "other", ClientName: "Other", Mode: InstructionV2ModeEnforce,
		Decision: "allow", Outcome: InstructionV2OutcomeAIPass, Reason: "sync_ai_pass",
		Instructions: field, Input1: InstructionV2Field{State: "missing"},
		SelectedField: "instructions", SelectedSHA256: field.SHA256,
		AIResult: "pass", AIReviewedField: "instructions", BodyBytes: field.Bytes,
		ConfigVersion: 1, EvidenceStatus: "not_stored",
	}
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
	event := instructionV2ReviewTestEvent(field)
	event.Mode = mode
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
