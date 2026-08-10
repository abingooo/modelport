package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func upsertInstructionV2VaultTx(
	ctx context.Context,
	tx *sql.Tx,
	write instructionV2VaultWrite,
) (int64, error) {
	if write.SHA256 == "" || len(write.RawCiphertext) == 0 || write.ContentBytes <= 0 || write.StoredBytes <= 0 {
		return 0, errors.New("instruction audit content vault write is incomplete")
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_v2_content_vault
			(sha256, raw_ciphertext, content_bytes, stored_bytes, observed_field)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (sha256) DO UPDATE
		SET observed_field = CASE
		        WHEN instruction_audit_v2_content_vault.observed_field = ''
		        THEN EXCLUDED.observed_field
		        ELSE instruction_audit_v2_content_vault.observed_field
		    END,
		    updated_at = NOW()
		WHERE instruction_audit_v2_content_vault.content_bytes = EXCLUDED.content_bytes
		RETURNING id`, write.SHA256, write.RawCiphertext, write.ContentBytes,
		write.StoredBytes, write.ObservedField).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("instruction audit content digest collision")
	}
	return id, err
}

func upsertInstructionV2RiskTx(
	ctx context.Context,
	tx *sql.Tx,
	eventID int64,
	write instructionV2RiskWrite,
) (int64, error) {
	vaultID, err := upsertInstructionV2VaultTx(ctx, tx, write.Vault)
	if err != nil {
		return 0, err
	}
	var riskID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_v2_risk_hashes
			(sha256, content_vault_id, observed_field, status, source, source_event_id,
			 reviewer_node_id, reviewer_model, prompt_version, confidence, review_reason,
			 review_category)
		VALUES ($1, $2, $3, 'active', $4, NULLIF($5, 0), $6, $7, $8, $9, $10, $11)
		ON CONFLICT (sha256) DO UPDATE
		SET status = 'active', content_vault_id = EXCLUDED.content_vault_id,
		    observed_field = EXCLUDED.observed_field, source = EXCLUDED.source,
		    source_event_id = COALESCE(EXCLUDED.source_event_id,
		        instruction_audit_v2_risk_hashes.source_event_id),
		    reviewer_node_id = EXCLUDED.reviewer_node_id,
		    reviewer_model = EXCLUDED.reviewer_model,
		    prompt_version = EXCLUDED.prompt_version,
		    confidence = EXCLUDED.confidence,
		    review_reason = EXCLUDED.review_reason,
		    review_category = EXCLUDED.review_category,
		    updated_at = NOW()
		RETURNING id`, write.Vault.SHA256, vaultID, write.ObservedField, write.Source,
		eventID, write.ReviewerNodeID, write.ReviewerModel, write.PromptVersion,
		write.Confidence, write.ReviewReason, write.ReviewCategory).Scan(&riskID)
	return riskID, err
}

func upsertInstructionV2TrustedTx(
	ctx context.Context,
	tx *sql.Tx,
	eventID int64,
	write instructionV2TrustedWrite,
	actorID int64,
) (int64, error) {
	var activeRisk bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM instruction_audit_v2_risk_hashes
			WHERE sha256 = $1 AND status = 'active'
		)`, write.Vault.SHA256).Scan(&activeRisk); err != nil {
		return 0, err
	}
	if activeRisk {
		return 0, errors.New("instruction audit risk hash takes precedence")
	}
	vaultID, err := upsertInstructionV2VaultTx(ctx, tx, write.Vault)
	if err != nil {
		return 0, err
	}
	var hashID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_v2_hashes
			(sha256, name, note, status, source, observed_field, content_bytes,
			 raw_storage, raw_ciphertext, stored_bytes, ai_sampled, source_event_id,
			 reviewer_node_id, reviewer_model, prompt_version, confidence, review_reason,
			 review_category, global_trust, content_vault_id, created_by, updated_by)
		VALUES ($1, $2, $3, 'active', $4, $5, $6,
		        'unavailable', NULL, 0, $7, NULLIF($8, 0),
		        $9, $10, $11, $12, $13, $14, $15, $16,
		        NULLIF($17, 0), NULLIF($17, 0))
		ON CONFLICT (sha256) DO UPDATE
		SET status = 'active',
		    global_trust = instruction_audit_v2_hashes.global_trust OR EXCLUDED.global_trust,
		    content_vault_id = COALESCE(instruction_audit_v2_hashes.content_vault_id,
		        EXCLUDED.content_vault_id),
		    observed_field = CASE WHEN instruction_audit_v2_hashes.observed_field = ''
		        THEN EXCLUDED.observed_field ELSE instruction_audit_v2_hashes.observed_field END,
		    source_event_id = COALESCE(EXCLUDED.source_event_id,
		        instruction_audit_v2_hashes.source_event_id),
		    reviewer_node_id = EXCLUDED.reviewer_node_id,
		    reviewer_model = EXCLUDED.reviewer_model,
		    prompt_version = EXCLUDED.prompt_version,
		    confidence = EXCLUDED.confidence,
		    review_reason = EXCLUDED.review_reason,
		    review_category = EXCLUDED.review_category,
		    updated_by = NULLIF($17, 0), updated_at = NOW()
		WHERE instruction_audit_v2_hashes.status <> 'revoked'
		RETURNING id`, write.Vault.SHA256, "AI 可信 "+write.Vault.SHA256[:12],
		"由指令审核复核通过", write.Source, write.ObservedField,
		write.Vault.ContentBytes, write.Vault.ContentBytes != int64(write.Vault.StoredBytes),
		eventID, write.ReviewerNodeID, write.ReviewerModel, write.PromptVersion,
		write.Confidence, write.ReviewReason, write.ReviewCategory, write.GlobalTrust,
		vaultID, actorID).Scan(&hashID)
	if err != nil {
		return 0, err
	}
	for _, scopeID := range write.ScopeIDs {
		if scopeID <= 0 || write.GlobalTrust {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_v2_hash_scopes
				(hash_id, scope_id, status, source, created_by, updated_by)
			VALUES ($1, $2, 'active', $3, NULLIF($4, 0), NULLIF($4, 0))
			ON CONFLICT (hash_id, scope_id) DO UPDATE
			SET status = 'active', source = EXCLUDED.source,
			    candidate_expires_at = NULL, updated_by = NULLIF($4, 0),
			    updated_at = NOW()`, hashID, scopeID, write.Source, actorID); err != nil {
			return 0, err
		}
	}
	return hashID, nil
}

func upsertInstructionV2ReviewJobTx(
	ctx context.Context,
	tx *sql.Tx,
	eventID int64,
	write instructionV2ReviewJobWrite,
) (int64, error) {
	vaultID, err := upsertInstructionV2VaultTx(ctx, tx, write.Vault)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, write.Vault.SHA256); err != nil {
		return 0, err
	}
	var existingID int64
	var existingObserveOnly bool
	err = tx.QueryRowContext(ctx, `
		SELECT id, observe_only
		FROM instruction_audit_v2_review_jobs
		WHERE sha256 = $1
		FOR UPDATE`, write.Vault.SHA256).Scan(&existingID, &existingObserveOnly)
	if err == nil {
		if existingObserveOnly && !write.ObserveOnly {
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM instruction_audit_v2_review_attempts WHERE job_id = $1`, existingID); err != nil {
				return 0, err
			}
			_, err = tx.ExecContext(ctx, `
				UPDATE instruction_audit_v2_review_jobs
				SET content_vault_id = $2, selected_field = $3,
				    source_event_id = NULLIF($4, 0), status = 'pending', final_result = '',
				    pass_votes = 0, reject_votes = 0, retry_round = 0,
				    next_attempt_at = NOW(), lease_owner = '', lease_expires_at = NULL,
				    prompt_version = $5, review_criteria = $6, config_version = $7,
				    observe_only = FALSE, sampled = $8, sample_bytes = $9,
				    content_bytes = $10, last_error = '', completed_at = NULL,
				    updated_at = NOW()
				WHERE id = $1`, existingID, vaultID, write.SelectedField, eventID,
				write.PromptVersion, write.ReviewCriteria, write.ConfigVersion,
				write.Sampled, write.SampleBytes, write.Vault.ContentBytes)
			if err != nil {
				return 0, err
			}
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE instruction_audit_v2_review_jobs
				SET source_event_id = COALESCE(source_event_id, NULLIF($2, 0)),
				    updated_at = NOW()
				WHERE id = $1`, existingID, eventID)
			if err != nil {
				return 0, err
			}
		}
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var jobID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_v2_review_jobs
			(sha256, content_vault_id, selected_field, source_event_id, prompt_version,
			 review_criteria, config_version, observe_only, sampled, sample_bytes, content_bytes)
		VALUES ($1, $2, $3, NULLIF($4, 0), $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`, write.Vault.SHA256, vaultID, write.SelectedField, eventID,
		write.PromptVersion, write.ReviewCriteria, write.ConfigVersion, write.ObserveOnly,
		write.Sampled, write.SampleBytes, write.Vault.ContentBytes).Scan(&jobID)
	return jobID, err
}

func (r *InstructionV2Repository) ResumeOrGetReviewJobBySHA(
	ctx context.Context,
	write instructionV2ReviewJobWrite,
) (*instructionV2ReviewReuse, error) {
	if strings.TrimSpace(write.Vault.SHA256) == "" {
		return nil, errors.New("instruction audit review digest is empty")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, write.Vault.SHA256); err != nil {
		return nil, err
	}
	result := instructionV2ReviewReuse{}
	var observeOnly bool
	err = tx.QueryRowContext(ctx, `
		SELECT job.id, job.status, job.observe_only, COALESCE(event.decision, '')
		FROM instruction_audit_v2_review_jobs job
		LEFT JOIN instruction_audit_v2_events event ON event.id = job.source_event_id
		WHERE job.sha256 = $1
		FOR UPDATE OF job`, write.Vault.SHA256).Scan(
		&result.JobID, &result.Status, &observeOnly, &result.SourceDecision,
	)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if observeOnly && !write.ObserveOnly {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM instruction_audit_v2_review_attempts WHERE job_id = $1`, result.JobID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_review_jobs
			SET selected_field = $2, status = 'pending', final_result = '',
			    pass_votes = 0, reject_votes = 0, retry_round = 0,
			    next_attempt_at = NOW(), lease_owner = '', lease_expires_at = NULL,
			    prompt_version = $3, review_criteria = $4, config_version = $5,
			    observe_only = FALSE, sampled = $6, sample_bytes = $7,
			    content_bytes = $8, last_error = '', completed_at = NULL,
			    updated_at = NOW()
			WHERE id = $1`, result.JobID, write.SelectedField, write.PromptVersion,
			write.ReviewCriteria, write.ConfigVersion, write.Sampled,
			write.SampleBytes, write.Vault.ContentBytes); err != nil {
			return nil, err
		}
		result.Status = "pending"
		result.ResetForEnforcement = true
	} else if result.Status == "failed" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_review_jobs
			SET status = 'retry', retry_round = 0, next_attempt_at = NOW(),
			    lease_owner = '', lease_expires_at = NULL, last_error = '',
			    completed_at = NULL, updated_at = NOW()
			WHERE id = $1`, result.JobID); err != nil {
			return nil, err
		}
		result.Status = "retry"
		result.Requeued = true
	} else if result.Status != "pending" && result.Status != "retry" && result.Status != "processing" {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *InstructionV2Repository) ClaimReviewJob(
	ctx context.Context,
	owner string,
	leaseDuration time.Duration,
) (*instructionV2ClaimedReviewJob, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("instruction audit review lease owner is empty")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var job instructionV2ClaimedReviewJob
	err = tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id FROM instruction_audit_v2_review_jobs
			WHERE (
				(status IN ('pending', 'retry') AND next_attempt_at <= NOW())
				OR (status = 'processing' AND lease_expires_at <= NOW())
			)
			ORDER BY next_attempt_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE instruction_audit_v2_review_jobs job
		SET status = 'processing', lease_owner = $1,
		    lease_expires_at = NOW() + ($2 * INTERVAL '1 millisecond'),
		    updated_at = NOW()
		FROM candidate
		WHERE job.id = candidate.id
		RETURNING job.id, job.sha256, job.content_vault_id, job.selected_field,
		          job.source_event_id, job.status, job.final_result, job.pass_votes,
		          job.reject_votes, job.retry_round, job.next_attempt_at,
		          job.prompt_version, job.review_criteria, job.config_version,
		          job.observe_only, job.sampled, job.lease_owner,
		          job.sample_bytes, job.content_bytes, job.last_error,
		          job.completed_at, job.created_at, job.updated_at`, owner,
		leaseDuration.Milliseconds()).Scan(
		&job.ID, &job.SHA256, &job.ContentVaultID, &job.SelectedField,
		&job.SourceEventID, &job.Status, &job.FinalResult, &job.PassVotes,
		&job.RejectVotes, &job.RetryRound, &job.NextAttemptAt,
		&job.PromptVersion, &job.ReviewCriteria, &job.ConfigVersion, &job.ObserveOnly, &job.Sampled,
		&job.LeaseOwner,
		&job.SampleBytes, &job.ContentBytes, &job.LastError,
		&job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	job.SHA256 = strings.TrimSpace(job.SHA256)
	if err := tx.QueryRowContext(ctx, `
		SELECT raw_ciphertext FROM instruction_audit_v2_content_vault WHERE id = $1`,
		job.ContentVaultID).Scan(&job.Ciphertext); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *InstructionV2Repository) RecordReviewAttempts(
	ctx context.Context,
	jobID int64,
	leaseOwner string,
	attempts []InstructionV2ReviewAttempt,
) (string, bool, error) {
	if strings.TrimSpace(leaseOwner) == "" || len(attempts) == 0 {
		return "", false, errInstructionV2ReviewLeaseLost
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback() }()
	var status, selectedField, promptVersion, currentLeaseOwner string
	var observeOnly bool
	var sourceEventID sql.NullInt64
	var vaultID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT status, selected_field, prompt_version, observe_only, lease_owner,
		       source_event_id, content_vault_id
		FROM instruction_audit_v2_review_jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(
		&status, &selectedField, &promptVersion, &observeOnly, &currentLeaseOwner,
		&sourceEventID, &vaultID,
	); err != nil {
		return "", false, err
	}
	if status != "processing" || currentLeaseOwner != leaseOwner {
		return "", false, errInstructionV2ReviewLeaseLost
	}
	for _, attempt := range attempts {
		var attemptNo int
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(attempt_no), 0) + 1
			FROM instruction_audit_v2_review_attempts
			WHERE job_id = $1 AND node_slot = $2`, jobID, attempt.NodeSlot).Scan(&attemptNo); err != nil {
			return "", false, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_v2_review_attempts
				(job_id, node_id, node_slot, node_name_snapshot, reviewer_model,
				 attempt_no, result, confidence, reason, category, prompt_version,
				 sampled, latency_ms)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			jobID, attempt.NodeID, attempt.NodeSlot, attempt.NodeName, attempt.ReviewerModel,
			attemptNo, attempt.Result, attempt.Confidence, attempt.Reason, attempt.Category,
			promptVersion, attempt.Sampled, attempt.LatencyMS); err != nil {
			return "", false, err
		}
	}
	var passVotes, rejectVotes int
	if err := tx.QueryRowContext(ctx, `
		WITH latest_valid AS (
			SELECT DISTINCT ON (node_slot) node_slot, result
			FROM instruction_audit_v2_review_attempts
			WHERE job_id = $1 AND result IN ('pass', 'reject')
			ORDER BY node_slot, id DESC
		)
		SELECT COUNT(*) FILTER (WHERE result = 'pass'),
		       COUNT(*) FILTER (WHERE result = 'reject')
		FROM latest_valid`, jobID).Scan(&passVotes, &rejectVotes); err != nil {
		return "", false, err
	}
	finalResult := ""
	if passVotes >= 2 {
		finalResult = "pass"
	} else if rejectVotes >= 2 {
		finalResult = "reject"
	}
	if finalResult != "" && !observeOnly {
		var vault instructionV2VaultWrite
		if err := tx.QueryRowContext(ctx, `
			SELECT sha256, observed_field, raw_ciphertext, content_bytes, stored_bytes
			FROM instruction_audit_v2_content_vault WHERE id = $1`, vaultID).Scan(
			&vault.SHA256, &vault.ObservedField, &vault.RawCiphertext,
			&vault.ContentBytes, &vault.StoredBytes,
		); err != nil {
			return "", false, err
		}
		vault.SHA256 = strings.TrimSpace(vault.SHA256)
		var winning InstructionV2ReviewAttempt
		if err := tx.QueryRowContext(ctx, `
			SELECT node_id, node_slot, node_name_snapshot, reviewer_model, result,
			       confidence, reason, category, sampled, latency_ms
			FROM instruction_audit_v2_review_attempts
			WHERE job_id = $1 AND result = $2
			ORDER BY id DESC LIMIT 1`, jobID, finalResult).Scan(
			&winning.NodeID, &winning.NodeSlot, &winning.NodeName,
			&winning.ReviewerModel, &winning.Result, &winning.Confidence,
			&winning.Reason, &winning.Category, &winning.Sampled, &winning.LatencyMS,
		); err != nil {
			return "", false, err
		}
		eventID := int64(0)
		if sourceEventID.Valid {
			eventID = sourceEventID.Int64
		}
		if finalResult == "pass" {
			_, err = upsertInstructionV2TrustedTx(ctx, tx, eventID, instructionV2TrustedWrite{
				Vault: vault, Source: "ai_review", ObservedField: selectedField,
				ReviewerNodeID: winning.NodeID, ReviewerModel: winning.ReviewerModel,
				PromptVersion: promptVersion, Confidence: winning.Confidence,
				ReviewReason: winning.Reason, ReviewCategory: winning.Category,
				GlobalTrust: true,
			}, 0)
		} else {
			_, err = upsertInstructionV2RiskTx(ctx, tx, eventID, instructionV2RiskWrite{
				Vault: vault, Source: "async_ai", ObservedField: selectedField,
				ReviewerNodeID: winning.NodeID, ReviewerModel: winning.ReviewerModel,
				PromptVersion: promptVersion, Confidence: winning.Confidence,
				ReviewReason: winning.Reason, ReviewCategory: winning.Category,
			})
		}
		if err != nil {
			return "", false, err
		}
	}
	if finalResult != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_review_jobs
			SET status = 'completed', final_result = $2, pass_votes = $3,
			    reject_votes = $4, lease_owner = '', lease_expires_at = NULL,
			    completed_at = NOW(), updated_at = NOW()
			WHERE id = $1`, jobID, finalResult, passVotes, rejectVotes); err != nil {
			return "", false, err
		}
	} else if _, err := tx.ExecContext(ctx, `
		UPDATE instruction_audit_v2_review_jobs
		SET pass_votes = $2, reject_votes = $3, updated_at = NOW()
		WHERE id = $1`, jobID, passVotes, rejectVotes); err != nil {
		return "", false, err
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return finalResult, finalResult != "", nil
}

func (r *InstructionV2Repository) ScheduleReviewRetry(
	ctx context.Context,
	jobID int64,
	leaseOwner string,
	schedule []int,
	lastError string,
) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var status, currentLeaseOwner string
	var retryRound int
	if err := tx.QueryRowContext(ctx, `
		SELECT status, retry_round, lease_owner FROM instruction_audit_v2_review_jobs
		WHERE id = $1 FOR UPDATE`, jobID).Scan(&status, &retryRound, &currentLeaseOwner); err != nil {
		return false, err
	}
	if status == "completed" {
		return false, tx.Commit()
	}
	if status != "processing" || currentLeaseOwner != leaseOwner {
		return false, errInstructionV2ReviewLeaseLost
	}
	lastError = strings.TrimSpace(lastError)
	if len(lastError) > 500 {
		lastError = lastError[:500]
	}
	if retryRound >= len(schedule) {
		_, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_review_jobs
			SET status = 'failed', lease_owner = '', lease_expires_at = NULL,
			    last_error = $2, updated_at = NOW()
			WHERE id = $1`, jobID, lastError)
		if err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	delay := schedule[retryRound]
	if delay <= 0 {
		delay = 30
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE instruction_audit_v2_review_jobs
		SET status = 'retry', retry_round = retry_round + 1,
		    next_attempt_at = NOW() + ($2 * INTERVAL '1 second'),
		    lease_owner = '', lease_expires_at = NULL,
		    last_error = $3, updated_at = NOW()
		WHERE id = $1`, jobID, delay, lastError)
	if err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (r *InstructionV2Repository) ListReviewAttempts(ctx context.Context, jobID int64) ([]InstructionV2ReviewAttempt, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, job_id, node_id, node_slot, node_name_snapshot, reviewer_model,
		       attempt_no, result, confidence, reason, category, prompt_version,
		       sampled, latency_ms, created_at
		FROM instruction_audit_v2_review_attempts
		WHERE job_id = $1 ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2ReviewAttempt, 0)
	for rows.Next() {
		var item InstructionV2ReviewAttempt
		if err := rows.Scan(
			&item.ID, &item.JobID, &item.NodeID, &item.NodeSlot, &item.NodeName,
			&item.ReviewerModel, &item.AttemptNo, &item.Result, &item.Confidence,
			&item.Reason, &item.Category, &item.PromptVersion, &item.Sampled,
			&item.LatencyMS, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) ListReviewJobs(
	ctx context.Context,
	page, pageSize int,
	status, query string,
) (InstructionV2ReviewJobPage, error) {
	where := []string{"1 = 1"}
	args := make([]any, 0, 4)
	if status = strings.TrimSpace(status); status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("j.status = $%d", len(args)))
	}
	if query = strings.TrimSpace(query); query != "" {
		args = append(args, "%"+query+"%")
		where = append(where, fmt.Sprintf("j.sha256 ILIKE $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM instruction_audit_v2_review_jobs j WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return InstructionV2ReviewJobPage{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		SELECT j.id, j.sha256, j.content_vault_id, j.selected_field, j.source_event_id,
		       j.status, j.final_result, j.pass_votes, j.reject_votes, j.retry_round,
		       j.next_attempt_at, j.prompt_version, j.review_criteria, j.config_version, j.observe_only,
		       j.sampled, j.sample_bytes, j.content_bytes, j.last_error,
		       j.completed_at, j.created_at, j.updated_at
		FROM instruction_audit_v2_review_jobs j WHERE `+whereSQL+`
		ORDER BY j.created_at DESC, j.id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return InstructionV2ReviewJobPage{}, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2ReviewJob, 0, pageSize)
	for rows.Next() {
		item, scanErr := scanInstructionV2ReviewJob(rows)
		if scanErr != nil {
			return InstructionV2ReviewJobPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return InstructionV2ReviewJobPage{}, err
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return InstructionV2ReviewJobPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *InstructionV2Repository) GetReviewJob(ctx context.Context, id int64) (InstructionV2ReviewJob, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT j.id, j.sha256, j.content_vault_id, j.selected_field, j.source_event_id,
		       j.status, j.final_result, j.pass_votes, j.reject_votes, j.retry_round,
		       j.next_attempt_at, j.prompt_version, j.review_criteria, j.config_version, j.observe_only,
		       j.sampled, j.sample_bytes, j.content_bytes, j.last_error,
		       j.completed_at, j.created_at, j.updated_at
		FROM instruction_audit_v2_review_jobs j WHERE j.id = $1`, id)
	item, err := scanInstructionV2ReviewJob(row)
	if err != nil {
		return InstructionV2ReviewJob{}, err
	}
	item.Attempts, err = r.ListReviewAttempts(ctx, id)
	return item, err
}

type instructionV2ReviewJobScanner interface {
	Scan(...any) error
}

func scanInstructionV2ReviewJob(scanner instructionV2ReviewJobScanner) (InstructionV2ReviewJob, error) {
	var item InstructionV2ReviewJob
	err := scanner.Scan(
		&item.ID, &item.SHA256, &item.ContentVaultID, &item.SelectedField,
		&item.SourceEventID, &item.Status, &item.FinalResult, &item.PassVotes,
		&item.RejectVotes, &item.RetryRound, &item.NextAttemptAt,
		&item.PromptVersion, &item.ReviewCriteria, &item.ConfigVersion, &item.ObserveOnly, &item.Sampled,
		&item.SampleBytes, &item.ContentBytes, &item.LastError, &item.CompletedAt,
		&item.CreatedAt, &item.UpdatedAt,
	)
	item.SHA256 = strings.TrimSpace(item.SHA256)
	return item, err
}

func (r *InstructionV2Repository) ListRiskHashes(
	ctx context.Context,
	page, pageSize int,
	status, query string,
) (InstructionV2RiskHashPage, error) {
	where := []string{"1 = 1"}
	args := make([]any, 0, 4)
	if status = strings.TrimSpace(status); status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("risk.status = $%d", len(args)))
	}
	if query = strings.TrimSpace(query); query != "" {
		args = append(args, "%"+query+"%")
		where = append(where, fmt.Sprintf("(risk.sha256 ILIKE $%d OR risk.review_reason ILIKE $%d OR risk.review_category ILIKE $%d)", len(args), len(args), len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM instruction_audit_v2_risk_hashes risk WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return InstructionV2RiskHashPage{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, instructionV2RiskHashSelect()+`
		WHERE `+whereSQL+`
		ORDER BY risk.created_at DESC, risk.id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return InstructionV2RiskHashPage{}, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2RiskHash, 0, pageSize)
	for rows.Next() {
		item, scanErr := scanInstructionV2RiskHash(rows)
		if scanErr != nil {
			return InstructionV2RiskHashPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return InstructionV2RiskHashPage{}, err
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return InstructionV2RiskHashPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *InstructionV2Repository) GetRiskHash(ctx context.Context, id int64) (InstructionV2RiskHash, []byte, error) {
	row := r.db.QueryRowContext(ctx, instructionV2RiskHashSelect()+` WHERE risk.id = $1`, id)
	item, err := scanInstructionV2RiskHash(row)
	if err != nil {
		return InstructionV2RiskHash{}, nil, err
	}
	var ciphertext []byte
	if err := r.db.QueryRowContext(ctx, `
		SELECT raw_ciphertext FROM instruction_audit_v2_content_vault WHERE id = $1`,
		item.ContentVaultID).Scan(&ciphertext); err != nil {
		return InstructionV2RiskHash{}, nil, err
	}
	return item, ciphertext, nil
}

func instructionV2RiskHashSelect() string {
	return `
		SELECT risk.id, risk.sha256, risk.content_vault_id, risk.observed_field,
		       risk.status, risk.source, risk.source_event_id, risk.reviewer_node_id,
		       risk.reviewer_model, risk.prompt_version, risk.confidence,
		       risk.review_reason, risk.review_category, risk.human_review_status,
		       risk.reviewed_by, risk.reviewed_at, risk.created_by, risk.updated_by,
		       risk.created_at, risk.updated_at
		FROM instruction_audit_v2_risk_hashes risk`
}

type instructionV2RiskHashScanner interface {
	Scan(...any) error
}

func scanInstructionV2RiskHash(scanner instructionV2RiskHashScanner) (InstructionV2RiskHash, error) {
	var item InstructionV2RiskHash
	err := scanner.Scan(
		&item.ID, &item.SHA256, &item.ContentVaultID, &item.ObservedField,
		&item.Status, &item.Source, &item.SourceEventID, &item.ReviewerNodeID,
		&item.ReviewerModel, &item.PromptVersion, &item.Confidence,
		&item.ReviewReason, &item.ReviewCategory, &item.HumanReviewStatus,
		&item.ReviewedBy, &item.ReviewedAt, &item.CreatedBy, &item.UpdatedBy,
		&item.CreatedAt, &item.UpdatedAt,
	)
	item.SHA256 = strings.TrimSpace(item.SHA256)
	return item, err
}

func (r *InstructionV2Repository) SaveManualRiskHash(
	ctx context.Context,
	write instructionV2RiskWrite,
	actorID int64,
) (InstructionV2RiskHash, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2RiskHash{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	riskID, err := upsertInstructionV2RiskTx(ctx, tx, 0, write)
	if err != nil {
		return InstructionV2RiskHash{}, 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE instruction_audit_v2_risk_hashes
		SET created_by = COALESCE(created_by, NULLIF($2, 0)),
		    updated_by = NULLIF($2, 0), updated_at = NOW()
		WHERE id = $1`, riskID, actorID); err != nil {
		return InstructionV2RiskHash{}, 0, err
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return InstructionV2RiskHash{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2RiskHash{}, 0, err
	}
	item, _, err := r.GetRiskHash(ctx, riskID)
	return item, version, err
}

func (r *InstructionV2Repository) UpdateRiskHash(
	ctx context.Context,
	id int64,
	request UpdateInstructionV2RiskHashRequest,
	actorID int64,
) (InstructionV2RiskHash, *InstructionV2Hash, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2RiskHash{}, nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var item InstructionV2RiskHash
	var vault instructionV2VaultWrite
	err = tx.QueryRowContext(ctx, `
		SELECT risk.id, risk.sha256, risk.content_vault_id, risk.observed_field,
		       risk.status, risk.source, risk.source_event_id, risk.reviewer_node_id,
		       risk.reviewer_model, risk.prompt_version, risk.confidence,
		       risk.review_reason, risk.review_category, risk.human_review_status,
		       vault.raw_ciphertext, vault.content_bytes, vault.stored_bytes
		FROM instruction_audit_v2_risk_hashes risk
		JOIN instruction_audit_v2_content_vault vault ON vault.id = risk.content_vault_id
		WHERE risk.id = $1 FOR UPDATE`, id).Scan(
		&item.ID, &item.SHA256, &item.ContentVaultID, &item.ObservedField,
		&item.Status, &item.Source, &item.SourceEventID, &item.ReviewerNodeID,
		&item.ReviewerModel, &item.PromptVersion, &item.Confidence,
		&item.ReviewReason, &item.ReviewCategory, &item.HumanReviewStatus,
		&vault.RawCiphertext, &vault.ContentBytes, &vault.StoredBytes,
	)
	if err != nil {
		return InstructionV2RiskHash{}, nil, 0, err
	}
	item.SHA256 = strings.TrimSpace(item.SHA256)
	vault.SHA256, vault.ObservedField = item.SHA256, item.ObservedField
	var trustedHashID int64
	switch request.Action {
	case "confirm_risk":
		_, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_risk_hashes
			SET status = 'active', human_review_status = 'confirmed_risk',
			    reviewed_by = NULLIF($2, 0), reviewed_at = NOW(),
			    updated_by = NULLIF($2, 0), updated_at = NOW()
			WHERE id = $1`, id, actorID)
	case "confirm_safe":
		if _, err = tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_risk_hashes WHERE id = $1`, id); err == nil {
			confidence := 1.0
			if item.Confidence != nil {
				confidence = *item.Confidence
			}
			trustedHashID, err = upsertInstructionV2TrustedTx(ctx, tx, pointerInt64Value(item.SourceEventID), instructionV2TrustedWrite{
				Vault: vault, Source: "manual", ObservedField: item.ObservedField,
				ReviewerNodeID: item.ReviewerNodeID, ReviewerModel: item.ReviewerModel,
				PromptVersion: item.PromptVersion, Confidence: confidence,
				ReviewReason: "人工复审确认安全", ReviewCategory: "human_approved",
				GlobalTrust: true,
			}, actorID)
		}
	case "disable":
		_, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_risk_hashes
			SET status = 'disabled', updated_by = NULLIF($2, 0), updated_at = NOW()
			WHERE id = $1`, id, actorID)
	case "enable":
		_, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_risk_hashes
			SET status = 'active', updated_by = NULLIF($2, 0), updated_at = NOW()
			WHERE id = $1`, id, actorID)
	default:
		err = errors.New("invalid instruction audit risk action")
	}
	if err != nil {
		return InstructionV2RiskHash{}, nil, 0, err
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return InstructionV2RiskHash{}, nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2RiskHash{}, nil, 0, err
	}
	if trustedHashID > 0 {
		trusted, _, getErr := r.GetHash(ctx, trustedHashID)
		return InstructionV2RiskHash{}, &trusted, version, getErr
	}
	updated, _, getErr := r.GetRiskHash(ctx, id)
	return updated, nil, version, getErr
}

func (r *InstructionV2Repository) DeleteRiskHash(ctx context.Context, id, actorID int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_risk_hashes WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return 0, sql.ErrNoRows
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

func (r *InstructionV2Repository) RetryReviewJob(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_v2_review_jobs
		SET status = 'retry', retry_round = 0, next_attempt_at = NOW(),
		    lease_owner = '', lease_expires_at = NULL, last_error = '',
		    completed_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'failed'`, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *InstructionV2Repository) GetVaultCiphertext(ctx context.Context, id int64) ([]byte, error) {
	var ciphertext []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT raw_ciphertext FROM instruction_audit_v2_content_vault WHERE id = $1`, id).Scan(&ciphertext)
	return ciphertext, err
}
