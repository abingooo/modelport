package securityaudit

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

func (r *InstructionV2Repository) PersistInstructionV2Event(ctx context.Context, write instructionV2PersistEvent) (instructionV2PersistResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return instructionV2PersistResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	eventID, err := insertInstructionV2Event(ctx, tx, write.Event)
	if err != nil {
		return instructionV2PersistResult{}, err
	}
	for _, evidence := range write.Evidence {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_v2_event_evidence
				(event_id, field_name, sha256, storage_kind, ciphertext,
				 content_bytes, stored_bytes, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			eventID, evidence.FieldName, evidence.SHA256, evidence.StorageKind,
			evidence.Ciphertext, evidence.ContentBytes, evidence.StoredBytes, evidence.ExpiresAt,
		); err != nil {
			return instructionV2PersistResult{}, err
		}
	}
	for _, review := range write.Reviews {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_v2_ai_reviews
				(event_id, node_id, node_name_snapshot, reviewer_model, field_name, sha256,
				 result, confidence, reason, category, prompt_version, sampled, cached, latency_ms)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			eventID, review.NodeID, review.NodeName, review.ReviewerModel, review.FieldName,
			review.SHA256, review.Result, review.Confidence, review.Reason, review.Category,
			review.PromptVersion, review.Sampled, review.Cached, review.LatencyMS,
		); err != nil {
			return instructionV2PersistResult{}, err
		}
	}
	result := instructionV2PersistResult{EventID: eventID}
	if write.Trusted != nil {
		hashID, trustedErr := upsertInstructionV2TrustedTx(ctx, tx, eventID, *write.Trusted, 0)
		if trustedErr != nil {
			return instructionV2PersistResult{}, trustedErr
		}
		result.HashID = &hashID
		if _, err := tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_events SET matched_hash_id = $2 WHERE id = $1`, eventID, hashID); err != nil {
			return instructionV2PersistResult{}, err
		}
	}
	if write.Risk != nil {
		riskID, riskErr := upsertInstructionV2RiskTx(ctx, tx, eventID, *write.Risk)
		if riskErr != nil {
			return instructionV2PersistResult{}, riskErr
		}
		result.RiskID = &riskID
	}
	if write.ReviewJob != nil {
		jobID, jobErr := upsertInstructionV2ReviewJobTx(ctx, tx, eventID, *write.ReviewJob)
		if jobErr != nil {
			return instructionV2PersistResult{}, jobErr
		}
		result.JobID = &jobID
		if _, err := tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_events SET review_job_id = $2 WHERE id = $1`, eventID, jobID); err != nil {
			return instructionV2PersistResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return instructionV2PersistResult{}, err
	}
	return result, nil
}

func (r *InstructionV2Repository) PersistInstructionV2Events(ctx context.Context, events []InstructionV2Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, event := range events {
		if _, err := insertInstructionV2Event(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertInstructionV2Event(ctx context.Context, tx *sql.Tx, event InstructionV2Event) (int64, error) {
	var eventID int64
	var instructionsDigest, input1Digest any
	if event.Instructions.SHA256 != "" {
		instructionsDigest = event.Instructions.SHA256
	}
	if event.Input1.SHA256 != "" {
		input1Digest = event.Input1.SHA256
	}
	var selectedDigest any
	if event.SelectedSHA256 != "" {
		selectedDigest = event.SelectedSHA256
	}
	err := tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_v2_events
			(request_id, user_id, user_email_snapshot, api_key_id, api_key_name_snapshot,
			 group_id, group_name_snapshot, scope_id, client_profile_id, client_key_snapshot,
			 client_name_snapshot, client_user_agent, model, endpoint, stage, mode, decision,
			 outcome, reason, instructions_state, instructions_sha256, instructions_bytes,
			 instructions_partial, input1_state, input1_sha256, input1_bytes, input1_partial,
			 matched_hash_id, ai_result, ai_reviewed_field, ai_sampled, audit_latency_ms,
			 ai_latency_ms, body_bytes, config_version, evidence_status,
			 selected_field, selected_sha256, review_job_id)
		VALUES
			($1, NULLIF($2, 0), $3, NULLIF($4, 0), $5,
			 $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
			 $18, $19, $20, $21, $22, $23, $24, $25, $26, $27,
			 $28, $29, $30, $31, $32, $33, $34, $35, $36,
			 $37, $38, $39)
		RETURNING id`,
		event.RequestID, pointerInt64Value(event.UserID), event.UserEmail,
		pointerInt64Value(event.APIKeyID), event.APIKeyName, event.GroupID, event.GroupName,
		event.ScopeID, event.ClientProfileID, event.ClientKey, event.ClientName,
		event.ClientUserAgent, event.Model, event.Endpoint, event.Stage, event.Mode,
		event.Decision, event.Outcome, event.Reason, event.Instructions.State,
		instructionsDigest, event.Instructions.Bytes, event.Instructions.Partial,
		event.Input1.State, input1Digest, event.Input1.Bytes, event.Input1.Partial,
		event.MatchedHashID, event.AIResult, event.AIReviewedField, event.AISampled,
		event.AuditLatencyMS, event.AILatencyMS, event.BodyBytes, event.ConfigVersion,
		event.EvidenceStatus, event.SelectedField, selectedDigest, event.ReviewJobID,
	).Scan(&eventID)
	return eventID, err
}

func (r *InstructionV2Repository) ListEvents(ctx context.Context, page, pageSize int, filter InstructionV2EventFilter) (InstructionV2EventPage, error) {
	where, args := instructionV2EventWhere(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM instruction_audit_v2_events e WHERE `+where, args...).Scan(&total); err != nil {
		return InstructionV2EventPage{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, instructionV2EventSelect()+`
		WHERE `+where+`
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return InstructionV2EventPage{}, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2Event, 0)
	for rows.Next() {
		item, err := scanInstructionV2Event(rows)
		if err != nil {
			return InstructionV2EventPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return InstructionV2EventPage{}, err
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return InstructionV2EventPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *InstructionV2Repository) GetEvent(ctx context.Context, id int64) (InstructionV2Event, error) {
	row := r.db.QueryRowContext(ctx, instructionV2EventSelect()+` WHERE e.id = $1`, id)
	item, err := scanInstructionV2Event(row)
	if err != nil {
		return InstructionV2Event{}, err
	}
	reviews, err := r.ListAIReviews(ctx, id)
	if err != nil {
		return InstructionV2Event{}, err
	}
	item.AIReviews = reviews
	return item, nil
}

func (r *InstructionV2Repository) ListAIReviews(ctx context.Context, eventID int64) ([]InstructionV2AIReview, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, event_id, node_id, node_name_snapshot, reviewer_model, field_name,
		       sha256, result, confidence, reason, category, prompt_version, sampled,
		       cached, latency_ms, created_at
		FROM instruction_audit_v2_ai_reviews WHERE event_id = $1 ORDER BY id`, eventID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2AIReview, 0)
	for rows.Next() {
		var item InstructionV2AIReview
		if err := rows.Scan(
			&item.ID, &item.EventID, &item.NodeID, &item.NodeName, &item.ReviewerModel,
			&item.FieldName, &item.SHA256, &item.Result, &item.Confidence, &item.Reason,
			&item.Category, &item.PromptVersion, &item.Sampled, &item.Cached,
			&item.LatencyMS, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.SHA256 = strings.TrimSpace(item.SHA256)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) Statistics(ctx context.Context, filter InstructionV2EventFilter) (InstructionV2Statistics, error) {
	if filter.From == nil {
		value := time.Now().UTC().Add(-24 * time.Hour)
		filter.From = &value
	}
	if filter.To == nil {
		value := time.Now().UTC()
		filter.To = &value
	}
	where, args := instructionV2EventWhere(filter)
	statistics := InstructionV2Statistics{From: *filter.From, To: *filter.To}
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE outcome = 'hash_pass'),
		       COUNT(*) FILTER (WHERE outcome = 'ai_pass'),
		       COUNT(*) FILTER (WHERE decision = 'block'),
		       COUNT(*) FILTER (WHERE outcome IN ('empty_pass', 'user_allowlist_pass')),
		       COUNT(*) FILTER (WHERE ai_result IN ('reject', 'uncertain', 'error', 'queue_full', 'timeout', 'invalid'))
		FROM instruction_audit_v2_events e WHERE `+where, args...).Scan(
		&statistics.Total, &statistics.HashPass, &statistics.AIPass, &statistics.Blocked,
		&statistics.EmptyOrAllowlist, &statistics.AIFailures,
	)
	if err != nil {
		return InstructionV2Statistics{}, err
	}
	if statistics.Total > 0 {
		statistics.BlockRate = float64(statistics.Blocked) / float64(statistics.Total)
	}
	return statistics, nil
}

func (r *InstructionV2Repository) DeleteEvents(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM security_notification_outbox
		WHERE source_type = 'instruction_audit_v2' AND source_id = ANY($1)`, pq.Array(ids)); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_events WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func instructionV2EventWhere(filter InstructionV2EventFilter) (string, []any) {
	where := []string{"1 = 1"}
	args := make([]any, 0, 10)
	add := func(condition string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if filter.EventID > 0 {
		add("e.id = $%d", filter.EventID)
	}
	if filter.UserID > 0 {
		add("e.user_id = $%d", filter.UserID)
	}
	if len(filter.GroupIDs) > 0 {
		add("e.group_id = ANY($%d)", pq.Array(filter.GroupIDs))
	}
	if len(filter.ClientKeys) > 0 {
		add("e.client_key_snapshot = ANY($%d)", pq.Array(filter.ClientKeys))
	}
	if len(filter.Outcomes) > 0 {
		add("e.outcome = ANY($%d)", pq.Array(filter.Outcomes))
	}
	if len(filter.Reasons) > 0 {
		add("e.reason = ANY($%d)", pq.Array(filter.Reasons))
	}
	if len(filter.AIResults) > 0 {
		add("e.ai_result = ANY($%d)", pq.Array(filter.AIResults))
	}
	if strings.TrimSpace(filter.Model) != "" {
		add("e.model = $%d", strings.TrimSpace(filter.Model))
	}
	if filter.From != nil {
		add("e.created_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		add("e.created_at < $%d", *filter.To)
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		args = append(args, "%"+query+"%")
		position := len(args)
		args = append(args, query)
		exactPosition := len(args)
		where = append(where, fmt.Sprintf(`(
			e.request_id ILIKE $%d OR e.user_email_snapshot ILIKE $%d
			OR e.api_key_name_snapshot ILIKE $%d OR e.group_name_snapshot ILIKE $%d
			OR e.client_name_snapshot ILIKE $%d OR e.model ILIKE $%d
			OR e.client_user_agent ILIKE $%d
			OR CAST(e.id AS TEXT) = $%d
		)`, position, position, position, position, position, position, position, exactPosition))
	}
	return strings.Join(where, " AND "), args
}

func instructionV2EventSelect() string {
	return `
		SELECT e.id, e.request_id, e.user_id, e.user_email_snapshot, e.api_key_id,
		       e.api_key_name_snapshot, e.group_id, e.group_name_snapshot, e.scope_id,
		       e.client_profile_id, e.client_key_snapshot, e.client_name_snapshot,
		       e.client_user_agent, e.model, e.endpoint, e.stage, e.mode, e.decision,
		       e.outcome, e.reason, e.instructions_state, e.instructions_sha256,
		       e.instructions_bytes, e.instructions_partial, e.input1_state,
		       e.input1_sha256, e.input1_bytes, e.input1_partial, e.matched_hash_id,
		       e.ai_result, e.ai_reviewed_field, e.ai_sampled, e.audit_latency_ms,
		       e.ai_latency_ms, e.body_bytes, e.config_version, e.evidence_status,
		       e.selected_field, e.selected_sha256, e.review_job_id,
		       COALESCE((SELECT o.status FROM security_notification_outbox o
		                 WHERE o.source_type = 'instruction_audit_v2' AND o.source_id = e.id
		                   AND o.audience = 'user' ORDER BY o.id DESC LIMIT 1), 'not_requested'),
		       COALESCE((SELECT o.status FROM security_notification_outbox o
		                 WHERE o.source_type = 'instruction_audit_v2' AND o.source_id = e.id
		                   AND o.audience = 'ops' ORDER BY o.id DESC LIMIT 1), 'not_requested'),
		       e.created_at
		FROM instruction_audit_v2_events e`
}

type instructionV2RowScanner interface {
	Scan(...any) error
}

func scanInstructionV2Event(scanner instructionV2RowScanner) (InstructionV2Event, error) {
	var item InstructionV2Event
	var instructionsDigest, input1Digest, selectedDigest sql.NullString
	err := scanner.Scan(
		&item.ID, &item.RequestID, &item.UserID, &item.UserEmail, &item.APIKeyID,
		&item.APIKeyName, &item.GroupID, &item.GroupName, &item.ScopeID,
		&item.ClientProfileID, &item.ClientKey, &item.ClientName, &item.ClientUserAgent,
		&item.Model, &item.Endpoint, &item.Stage, &item.Mode, &item.Decision, &item.Outcome,
		&item.Reason, &item.Instructions.State, &instructionsDigest, &item.Instructions.Bytes,
		&item.Instructions.Partial, &item.Input1.State, &input1Digest, &item.Input1.Bytes,
		&item.Input1.Partial, &item.MatchedHashID, &item.AIResult, &item.AIReviewedField,
		&item.AISampled, &item.AuditLatencyMS, &item.AILatencyMS, &item.BodyBytes,
		&item.ConfigVersion, &item.EvidenceStatus, &item.SelectedField, &selectedDigest,
		&item.ReviewJobID, &item.UserNotificationStatus,
		&item.OpsNotificationStatus, &item.CreatedAt,
	)
	if err != nil {
		return InstructionV2Event{}, err
	}
	if instructionsDigest.Valid {
		item.Instructions.SHA256 = strings.TrimSpace(instructionsDigest.String)
	}
	if input1Digest.Valid {
		item.Input1.SHA256 = strings.TrimSpace(input1Digest.String)
	}
	if selectedDigest.Valid {
		item.SelectedSHA256 = strings.TrimSpace(selectedDigest.String)
	}
	return item, nil
}

func pointerInt64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
