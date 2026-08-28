package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	modelportservice "github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var (
	errInstructionAIAutomaticHashUnavailable = errors.New("instruction audit automatic hash is unavailable")
	errInstructionAIScopeNotManaged          = errors.New("instruction audit scope is not system managed")
	errInstructionAIScopeRevoked             = errors.New("instruction audit scope is revoked")
)

type instructionAIReviewAttempt struct {
	ReviewedSource string
	ReviewedSHA256 string
	Result         string
	ApprovedSource string
	Confidence     float64
	Reason         string
	ReviewerModel  string
	PromptVersion  string
	LatencyMS      int
}

type instructionAIOutcomeCommit struct {
	Request             Request
	Decision            *InstructionDecision
	EvidenceStatus      string
	EvidenceExpiresAt   *time.Time
	Evidence            []InstructionEvidence
	Attempts            []instructionAIReviewAttempt
	FinalAttempt        int
	ApprovedRaw         *instructionHashRawStorage
	ApprovedField       InstructionFieldResult
	AutomaticUntil      time.Time
	NotificationIntents []modelportservice.SecurityNotificationAudienceInput
}

type instructionAIOutcomeCommitResult struct {
	EventID       int64
	AIReviewID    int64
	AutomaticHash *InstructionHashEntry
	ConfigVersion int64
}

func (r *InstructionRepository) CommitAIOutcome(
	ctx context.Context,
	commit instructionAIOutcomeCommit,
) (*instructionAIOutcomeCommitResult, error) {
	if r == nil || r.db == nil || commit.Decision == nil || len(commit.Attempts) == 0 {
		return nil, errors.New("instruction audit AI outcome is invalid")
	}
	if commit.FinalAttempt < 0 || commit.FinalAttempt >= len(commit.Attempts) {
		return nil, errors.New("instruction audit AI final attempt is invalid")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	result := &instructionAIOutcomeCommitResult{}
	var automaticHashID, systemRuleSetID int64
	if commit.Decision.FinalOutcome == InstructionOutcomeAIPass {
		if commit.ApprovedRaw == nil || commit.ApprovedField.Plaintext == "" || commit.ApprovedField.SHA256 == "" {
			return nil, errors.New("instruction audit AI approved content is unavailable")
		}
		groupID := instructionGroupID(commit.Request.GroupID)
		clientType := strings.ToLower(strings.TrimSpace(commit.Request.InstructionClientType))
		if groupID <= 0 || !validInstructionDetectedClientType(clientType) {
			return nil, errors.New("instruction audit AI scope is invalid")
		}
		automaticHashID, err = upsertInstructionAutomaticHashTx(
			ctx, tx, commit.ApprovedField.SHA256,
			commit.Attempts[commit.FinalAttempt].ApprovedSource,
			commit.ApprovedRaw, commit.AutomaticUntil,
		)
		if err != nil {
			return nil, err
		}
		systemRuleSetID, err = ensureInstructionAIScopeTx(
			ctx, tx, groupID, clientType, automaticHashID, commit.AutomaticUntil,
		)
		if err != nil {
			return nil, err
		}
		commit.Decision.RuleSetIDs = appendUniqueInstructionID(commit.Decision.RuleSetIDs, systemRuleSetID)
		version, versionErr := bumpInstructionConfigTx(ctx, tx)
		if versionErr != nil {
			return nil, versionErr
		}
		commit.Decision.ConfigVersion = version
		result.ConfigVersion = version
	}

	reviewIDs := make([]int64, 0, len(commit.Attempts))
	for index, attempt := range commit.Attempts {
		var automaticHash any
		if automaticHashID > 0 && index == commit.FinalAttempt && attempt.Result == "pass" {
			automaticHash = automaticHashID
		}
		var reviewID int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_ai_reviews
				(request_id, user_id, group_id, client_type, model,
				 reviewed_source, reviewed_sha256, result, approved_source, confidence,
				 review_reason, reviewer_model, prompt_version, latency_ms, automatic_hash_id)
			VALUES ($1, NULLIF($2, 0), NULLIF($3, 0), $4, $5,
				$6, $7, $8, NULLIF($9, ''), $10, $11, $12, $13, $14, $15)
			RETURNING id`,
			commit.Request.RequestID, commit.Request.UserID, instructionGroupID(commit.Request.GroupID),
			commit.Request.InstructionClientType, commit.Request.Model,
			attempt.ReviewedSource, attempt.ReviewedSHA256, attempt.Result,
			attempt.ApprovedSource, attempt.Confidence, attempt.Reason,
			attempt.ReviewerModel, attempt.PromptVersion, attempt.LatencyMS, automaticHash,
		).Scan(&reviewID)
		if err != nil {
			return nil, err
		}
		reviewIDs = append(reviewIDs, reviewID)
	}
	result.AIReviewID = reviewIDs[commit.FinalAttempt]
	commit.Decision.AIReviewID = &result.AIReviewID
	eventID, err := recordInstructionEventTx(
		ctx, tx, commit.Request, commit.Decision, commit.EvidenceStatus,
		commit.EvidenceExpiresAt, commit.Evidence,
	)
	if err != nil {
		return nil, err
	}
	result.EventID = eventID
	if err = insertInstructionNotificationIntentsTx(ctx, tx, eventID, commit.NotificationIntents); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE instruction_audit_ai_reviews SET event_id = $1 WHERE id = ANY($2)`,
		eventID, pq.Array(reviewIDs)); err != nil {
		return nil, err
	}
	if automaticHashID > 0 {
		attempt := commit.Attempts[commit.FinalAttempt]
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_hash_sources
				(hash_id, source_type, field_name, event_id, ai_review_id,
				 reviewer_model, prompt_version, confidence, review_reason)
			VALUES ($1, 'ai_review', $2, $3, $4, $5, $6, $7, $8)`,
			automaticHashID, attempt.ApprovedSource, eventID, result.AIReviewID,
			attempt.ReviewerModel, attempt.PromptVersion, attempt.Confidence, attempt.Reason,
		); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if automaticHashID > 0 {
		if item, getErr := r.GetHash(ctx, automaticHashID); getErr == nil {
			result.AutomaticHash = item
		} else {
			result.AutomaticHash = &InstructionHashEntry{
				ID: automaticHashID, Digest: commit.ApprovedField.SHA256,
				ObservedSource: commit.Attempts[commit.FinalAttempt].ApprovedSource,
				Status:         "active", RawStatus: "stored",
			}
		}
	}
	return result, nil
}

func upsertInstructionAutomaticHashTx(
	ctx context.Context,
	tx *sql.Tx,
	digest string,
	fieldName string,
	raw *instructionHashRawStorage,
	validUntil time.Time,
) (int64, error) {
	if !instructionDigestPattern.MatchString(digest) || raw == nil || raw.Status != "stored" || len(raw.Ciphertext) == 0 {
		return 0, errors.New("instruction audit automatic hash payload is invalid")
	}
	var hashID int64
	err := tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_hashes
				(digest, name, note, observed_source, client_name, status, valid_from, valid_until)
			VALUES ($1, $2, 'AI 二审自动生成的精确范围临时规则', $3, 'ai_review', 'active', NOW(), NULL)
			ON CONFLICT (digest) DO NOTHING
			RETURNING id`, digest, "AI 临时规则 "+digest[:12], fieldName).Scan(&hashID)
	created := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if !created {
		var status string
		var existingFrom, existingUntil sql.NullTime
		if err = tx.QueryRowContext(ctx, `
				SELECT h.id, h.status, h.valid_from, h.valid_until
				FROM instruction_audit_hashes h
				WHERE h.digest = $1 FOR UPDATE`, digest).Scan(
			&hashID, &status, &existingFrom, &existingUntil,
		); err != nil {
			return 0, err
		}
		status = strings.TrimSpace(status)
		now := time.Now().UTC()
		if status != "active" || (existingFrom.Valid && now.Before(existingFrom.Time)) ||
			(existingUntil.Valid && !now.Before(existingUntil.Time)) {
			return 0, errInstructionAIAutomaticHashUnavailable
		}
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO instruction_audit_hash_raw_contents
			(hash_id, ciphertext, raw_content_status, content_bytes, hash_algorithm,
			 normalization_version, encryption_key_version, raw_expires_at)
		VALUES ($1, $2, 'stored', $3, $4, $5, $6, $7)
		ON CONFLICT (hash_id) DO UPDATE SET
			ciphertext = EXCLUDED.ciphertext,
			raw_content_status = EXCLUDED.raw_content_status,
			content_bytes = EXCLUDED.content_bytes,
			hash_algorithm = EXCLUDED.hash_algorithm,
			normalization_version = EXCLUDED.normalization_version,
			encryption_key_version = EXCLUDED.encryption_key_version,
			raw_expires_at = EXCLUDED.raw_expires_at,
			updated_at = NOW()
		WHERE instruction_audit_hash_raw_contents.raw_content_status <> 'stored'
		   OR instruction_audit_hash_raw_contents.raw_expires_at <= NOW()`,
		hashID, raw.Ciphertext, raw.ContentBytes, raw.HashAlgorithm,
		raw.Normalization, raw.KeyVersion, raw.ExpiresAt); err != nil {
		return 0, err
	}
	return hashID, nil
}

func ensureInstructionAIScopeTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID int64,
	clientType string,
	hashID int64,
	validUntil time.Time,
) (int64, error) {
	systemKey := fmt.Sprintf("ai:%d:%s", groupID, clientType)
	name := fmt.Sprintf("AI 临时规则 G%d %s", groupID, clientType)
	var ruleSetID int64
	err := tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_rule_sets
				(name, description, enabled, system_managed, system_key)
			VALUES ($1, 'AI 二审自动维护的精确分组与客户端范围', TRUE, TRUE, $2)
			ON CONFLICT (system_key) WHERE system_managed = TRUE AND system_key <> ''
			DO NOTHING
			RETURNING id`, name, systemKey).Scan(&ruleSetID)
	if errors.Is(err, sql.ErrNoRows) {
		var enabled, systemManaged bool
		err = tx.QueryRowContext(ctx, `
			SELECT id, enabled, system_managed
			FROM instruction_audit_rule_sets
			WHERE system_key = $1 AND system_managed = TRUE
			FOR UPDATE`, systemKey).Scan(&ruleSetID, &enabled, &systemManaged)
		if err == nil && (!enabled || !systemManaged) {
			return 0, errInstructionAIAutomaticHashUnavailable
		}
	}
	if err != nil {
		return 0, err
	}
	insertResult, err := tx.ExecContext(ctx, `
		INSERT INTO instruction_audit_rule_set_hashes
			(rule_set_id, hash_id, source_type, status, valid_until)
		VALUES ($1, $2, 'ai_review', 'active', $3)
		ON CONFLICT (rule_set_id, hash_id) DO NOTHING`, ruleSetID, hashID, validUntil)
	if err != nil {
		return 0, err
	}
	if inserted, rowsErr := insertResult.RowsAffected(); rowsErr != nil {
		return 0, rowsErr
	} else if inserted == 0 {
		var sourceType, status string
		var existingUntil sql.NullTime
		if err = tx.QueryRowContext(ctx, `
			SELECT source_type, status, valid_until
			FROM instruction_audit_rule_set_hashes
			WHERE rule_set_id = $1 AND hash_id = $2
			FOR UPDATE`, ruleSetID, hashID).Scan(&sourceType, &status, &existingUntil); err != nil {
			return 0, err
		}
		if status != "active" {
			return 0, errInstructionAIAutomaticHashUnavailable
		}
		if sourceType == "ai_review" && (!existingUntil.Valid || existingUntil.Time.Before(validUntil)) {
			if _, err = tx.ExecContext(ctx, `
				UPDATE instruction_audit_rule_set_hashes
				SET valid_until = $3, updated_at = NOW()
				WHERE rule_set_id = $1 AND hash_id = $2`, ruleSetID, hashID, validUntil); err != nil {
				return 0, err
			}
		}
	}
	bindingResult, err := tx.ExecContext(ctx, `
		INSERT INTO instruction_audit_group_bindings
			(group_id, rule_set_id, client_types, enabled)
		VALUES ($1, $2, $3, TRUE)
		ON CONFLICT (group_id, rule_set_id)
		DO NOTHING`, groupID, ruleSetID, pq.Array([]string{clientType}))
	if err != nil {
		return 0, err
	}
	if inserted, rowsErr := bindingResult.RowsAffected(); rowsErr != nil {
		return 0, rowsErr
	} else if inserted == 0 {
		var enabled bool
		var clientTypes pq.StringArray
		if err = tx.QueryRowContext(ctx, `
			SELECT enabled, client_types
			FROM instruction_audit_group_bindings
			WHERE group_id = $1 AND rule_set_id = $2
			FOR UPDATE`, groupID, ruleSetID).Scan(&enabled, &clientTypes); err != nil {
			return 0, err
		}
		if !enabled || len(clientTypes) != 1 || strings.ToLower(strings.TrimSpace(clientTypes[0])) != clientType {
			return 0, errInstructionAIAutomaticHashUnavailable
		}
	}
	return ruleSetID, nil
}

func appendUniqueInstructionID(values []int64, value int64) []int64 {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	result := append(append([]int64(nil), values...), value)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func (r *InstructionRepository) ListAIReviewsForEvent(ctx context.Context, eventID int64) ([]InstructionAIReview, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, event_id, request_id, user_id, group_id, client_type, model,
			reviewed_source, reviewed_sha256, result, COALESCE(approved_source, ''),
			confidence, review_reason, reviewer_model, prompt_version, latency_ms,
			automatic_hash_id, created_at
		FROM instruction_audit_ai_reviews
		WHERE event_id = $1 ORDER BY created_at, id`, eventID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionAIReview, 0, 2)
	for rows.Next() {
		item, scanErr := scanInstructionAIReview(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionRepository) HasSuccessfulHashRawReveal(ctx context.Context, hashID, actorID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM instruction_audit_sensitive_access_logs
			WHERE resource_type = 'hash_raw' AND resource_id = $1
			  AND actor_id = $2 AND action = 'reveal' AND succeeded = TRUE
		)`, hashID, actorID).Scan(&exists)
	return exists, err
}

func (r *InstructionRepository) UpdateHashStatus(
	ctx context.Context,
	hashID int64,
	status string,
	access InstructionSensitiveAccess,
) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var lockedID int64
	if err = tx.QueryRowContext(ctx, `
		SELECT id FROM instruction_audit_hashes WHERE id = $1 FOR UPDATE`, hashID).Scan(&lockedID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE instruction_audit_hashes
		SET status = $2::VARCHAR,
			valid_from = CASE WHEN $2::VARCHAR = 'active' THEN COALESCE(valid_from, NOW()) ELSE valid_from END,
			valid_until = CASE WHEN $2::VARCHAR = 'active' THEN NULL ELSE valid_until END,
			updated_at = NOW()
		WHERE id = $1`, hashID, status); err != nil {
		return 0, err
	}
	version, err := bumpInstructionConfigTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO instruction_audit_sensitive_access_logs
			(resource_type, resource_id, actor_id, action, request_id, client_ip,
			 user_agent, succeeded, error_code)
		VALUES ('ai_hash', $1, NULLIF($2, 0), $3, LEFT($4, 128), LEFT($5, 64),
			LEFT($6, 512), TRUE, '')`, hashID, access.ActorID, access.Action,
		access.RequestID, access.ClientIP, access.UserAgent); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return version, nil
}

func (r *InstructionRepository) UpdateHashScope(
	ctx context.Context,
	hashID int64,
	ruleSetID int64,
	action string,
	access InstructionSensitiveAccess,
) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var sourceType, status string
	var systemManaged bool
	if err = tx.QueryRowContext(ctx, `
		SELECT rsh.source_type, rsh.status, rs.system_managed
		FROM instruction_audit_rule_set_hashes rsh
		JOIN instruction_audit_rule_sets rs ON rs.id = rsh.rule_set_id
		WHERE rsh.hash_id = $1 AND rsh.rule_set_id = $2
		FOR UPDATE OF rsh`, hashID, ruleSetID).Scan(&sourceType, &status, &systemManaged); err != nil {
		return 0, err
	}
	if !systemManaged {
		return 0, errInstructionAIScopeNotManaged
	}
	if status == "revoked" && action != "revoke" {
		return 0, errInstructionAIScopeRevoked
	}
	switch action {
	case "promote":
		_, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_rule_set_hashes
			SET source_type = 'manual', status = 'active', valid_until = NULL,
				updated_by = NULLIF($3, 0), updated_at = NOW()
			WHERE hash_id = $1 AND rule_set_id = $2`, hashID, ruleSetID, access.ActorID)
	case "disable":
		_, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_rule_set_hashes
			SET status = 'disabled', updated_by = NULLIF($3, 0), updated_at = NOW()
			WHERE hash_id = $1 AND rule_set_id = $2`, hashID, ruleSetID, access.ActorID)
	case "revoke":
		_, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_rule_set_hashes
			SET status = 'revoked', updated_by = NULLIF($3, 0), updated_at = NOW()
			WHERE hash_id = $1 AND rule_set_id = $2`, hashID, ruleSetID, access.ActorID)
	default:
		return 0, errors.New("instruction audit scope action is invalid")
	}
	if err != nil {
		return 0, err
	}
	version, err := bumpInstructionConfigTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO instruction_audit_sensitive_access_logs
			(resource_type, resource_id, scope_rule_set_id, actor_id, action, request_id,
			 client_ip, user_agent, succeeded, error_code)
		VALUES ('ai_scope', $1, $2, NULLIF($3, 0), $4, LEFT($5, 128), LEFT($6, 64),
			LEFT($7, 512), TRUE, '')`, hashID, ruleSetID, access.ActorID, action,
		access.RequestID, access.ClientIP, access.UserAgent); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return version, nil
}

func scanInstructionAIReview(scanner instructionScanner) (InstructionAIReview, error) {
	var item InstructionAIReview
	var eventID, userID, groupID, hashID sql.NullInt64
	err := scanner.Scan(
		&item.ID, &eventID, &item.RequestID, &userID, &groupID, &item.ClientType,
		&item.Model, &item.ReviewedSource, &item.ReviewedSHA256, &item.Result,
		&item.ApprovedSource, &item.Confidence, &item.Reason, &item.ReviewerModel,
		&item.PromptVersion, &item.LatencyMS, &hashID, &item.CreatedAt,
	)
	if eventID.Valid {
		item.EventID = &eventID.Int64
	}
	if userID.Valid {
		item.UserID = &userID.Int64
	}
	if groupID.Valid {
		item.GroupID = &groupID.Int64
	}
	if hashID.Valid {
		item.AutomaticHashID = &hashID.Int64
	}
	return item, err
}
