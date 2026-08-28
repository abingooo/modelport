package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const instructionTranslationJobColumns = `
	id, resource_type, resource_id, field_name, target_language, provider, status,
	error_code, chunk_count, completed_chunks, attempts, max_attempts, claim_version,
	result_bytes, redaction_count, provider_latency_ms, requested_by, authorized_grant_id,
	processing_started_at, expires_at, created_at, updated_at`

func (r *InstructionRepository) CreateTranslationJob(
	ctx context.Context,
	request InstructionTranslationRequest,
	actorID int64,
	expiresAt time.Time,
	access InstructionSensitiveAccess,
) (*InstructionTranslationJob, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	grantID, authMethod, authorizationResult := instructionSensitiveAuditAuthorization(ctx)
	if grantID == nil || authMethod != "jwt" || authorizationResult != "granted" {
		return nil, errors.New("instruction audit sensitive authorization is required")
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_translation_jobs
			(resource_type, resource_id, field_name, target_language, provider,
			 requested_by, authorized_grant_id, expires_at, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, 0), $7, $8, NOW())
		RETURNING `+instructionTranslationJobColumns,
		request.ResourceType, request.ResourceID, request.FieldName,
		request.TargetLanguage, request.Provider, actorID, grantID, expiresAt.UTC())
	job, err := scanInstructionTranslationJob(row)
	if err != nil {
		return nil, err
	}
	access.ResourceType = "translation"
	access.ResourceID = job.ID
	access.ActorID = actorID
	access.Action = "translate"
	access.Succeeded = true
	access.ErrorCode = ""
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO instruction_audit_sensitive_access_logs
			(resource_type, resource_id, actor_id, action, request_id, client_ip,
			 user_agent, succeeded, error_code, grant_id, auth_method, authorization_result)
		VALUES ('translation', $1, NULLIF($2, 0), 'translate', LEFT($3, 128),
			LEFT($4, 64), LEFT($5, 512), TRUE, '', $6, LEFT($7, 24), LEFT($8, 24))`,
		job.ID, actorID, access.RequestID, access.ClientIP, access.UserAgent,
		grantID, authMethod, authorizationResult); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return job, nil
}

func (r *InstructionRepository) GetTranslationJob(ctx context.Context, id int64) (*InstructionTranslationJob, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	return scanInstructionTranslationJob(r.db.QueryRowContext(ctx, `
		SELECT `+instructionTranslationJobColumns+`
		FROM instruction_audit_translation_jobs WHERE id = $1`, id))
}

func (r *InstructionRepository) ValidateInstructionTranslationGrant(
	ctx context.Context,
	job *InstructionTranslationJob,
) error {
	if job == nil || job.RequestedBy == nil || job.AuthorizedGrantID == nil {
		return sql.ErrNoRows
	}
	_, err := r.GetActiveInstructionSensitiveGrantByID(ctx, *job.RequestedBy, *job.AuthorizedGrantID)
	return err
}

func (r *InstructionRepository) ClaimTranslationJob(ctx context.Context, now time.Time) (*InstructionTranslationJob, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, errors.New("instruction audit repository unavailable")
	}
	row := r.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id AS candidate_id FROM instruction_audit_translation_jobs
			WHERE status IN ('pending', 'retry')
			  AND next_attempt_at <= $1
			  AND expires_at > $1
			  AND attempts < max_attempts
			ORDER BY next_attempt_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE instruction_audit_translation_jobs AS j
		SET status = 'processing', attempts = j.attempts + 1,
			claim_version = j.claim_version + 1, processing_started_at = $1,
			error_code = '', updated_at = $1
		FROM candidate
		WHERE j.id = candidate.candidate_id
		RETURNING `+instructionTranslationJobColumns, now.UTC())
	job, err := scanInstructionTranslationJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return job, err == nil, err
}

func (r *InstructionRepository) SetTranslationJobProgress(
	ctx context.Context,
	jobID int64,
	claimVersion int64,
	chunkCount int,
	completedChunks int,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_translation_jobs
		SET chunk_count = $3, completed_chunks = $4, updated_at = NOW()
		WHERE id = $1 AND status = 'processing' AND claim_version = $2`,
		jobID, claimVersion, chunkCount, completedChunks)
	return requireInstructionTranslationLease(result, err)
}

func (r *InstructionRepository) CompleteTranslationJob(
	ctx context.Context,
	jobID int64,
	claimVersion int64,
	status string,
	completedChunks int,
	resultBytes int,
	redactionCount int,
	providerLatencyMS int,
	errorCode string,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_translation_jobs
		SET status = $3, completed_chunks = $4, result_bytes = $5,
			redaction_count = $6, provider_latency_ms = $7,
			error_code = LEFT($8, 64), processing_started_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'processing' AND claim_version = $2`,
		jobID, claimVersion, status, completedChunks, resultBytes,
		redactionCount, providerLatencyMS, errorCode)
	return requireInstructionTranslationLease(result, err)
}

func (r *InstructionRepository) RetryTranslationJob(
	ctx context.Context,
	jobID int64,
	claimVersion int64,
	nextAttempt time.Time,
	errorCode string,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_translation_jobs
		SET status = 'retry', next_attempt_at = $3, error_code = LEFT($4, 64),
			processing_started_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'processing' AND claim_version = $2
		  AND attempts < max_attempts`, jobID, claimVersion, nextAttempt.UTC(), errorCode)
	return requireInstructionTranslationLease(result, err)
}

func (r *InstructionRepository) ReclaimTranslationJobs(ctx context.Context, staleBefore time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_translation_jobs
		SET status = CASE WHEN attempts < max_attempts THEN 'retry' ELSE 'failed' END,
			next_attempt_at = NOW(), error_code = 'worker_interrupted',
			processing_started_at = NULL, updated_at = NOW()
		WHERE status = 'processing' AND processing_started_at < $1`, staleBefore.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *InstructionRepository) ExpireTranslationJobs(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_translation_jobs
		SET status = 'expired', processing_started_at = NULL, updated_at = NOW()
		WHERE expires_at <= $1 AND status <> 'expired'`, now.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *InstructionRepository) MarkTranslationJobExpired(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_translation_jobs
		SET status = 'expired', processing_started_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status <> 'expired'`, id)
	return err
}

func (r *InstructionRepository) TranslationQueueCounts(ctx context.Context) (pending, processing, failed int64, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('pending', 'retry')),
			COUNT(*) FILTER (WHERE status = 'processing'),
			COUNT(*) FILTER (WHERE status IN ('failed', 'partial'))
		FROM instruction_audit_translation_jobs
		WHERE expires_at > NOW()`).Scan(&pending, &processing, &failed)
	return
}

func requireInstructionTranslationLease(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("instruction audit translation lease lost")
	}
	return nil
}

func scanInstructionTranslationJob(scanner instructionScanner) (*InstructionTranslationJob, error) {
	var job InstructionTranslationJob
	var requestedBy sql.NullInt64
	var authorizedGrantID sql.NullInt64
	var processingStartedAt sql.NullTime
	err := scanner.Scan(
		&job.ID, &job.ResourceType, &job.ResourceID, &job.FieldName,
		&job.TargetLanguage, &job.Provider, &job.Status, &job.ErrorCode,
		&job.ChunkCount, &job.CompletedChunks, &job.Attempts, &job.MaxAttempts,
		&job.ClaimVersion, &job.ResultBytes, &job.RedactionCount,
		&job.ProviderLatencyMS, &requestedBy, &authorizedGrantID, &processingStartedAt,
		&job.ExpiresAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if requestedBy.Valid {
		job.RequestedBy = &requestedBy.Int64
	}
	if authorizedGrantID.Valid {
		job.AuthorizedGrantID = &authorizedGrantID.Int64
	}
	if processingStartedAt.Valid {
		value := processingStartedAt.Time.UTC()
		job.ProcessingStartedAt = &value
	}
	return &job, err
}
