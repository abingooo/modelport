package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type securityNotificationRepository struct {
	db *sql.DB
}

func NewSecurityNotificationRepository(db *sql.DB) service.SecurityNotificationRepository {
	return &securityNotificationRepository{db: db}
}

func (r *securityNotificationRepository) Enqueue(ctx context.Context, input service.SecurityNotificationAudienceInput) error {
	variableValues := input.Variables
	if variableValues == nil {
		variableValues = map[string]string{}
	}
	variables, err := json.Marshal(variableValues)
	if err != nil {
		return fmt.Errorf("marshal security notification variables: %w", err)
	}
	status := "pending"
	dedupKey := any(input.DedupKey)
	if len(input.Recipients) == 0 {
		status = "no_recipient"
		dedupKey = nil
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO security_notification_outbox
			(source_type, source_id, audience, user_id, recipients, template_event, variables, dedup_key, status)
		VALUES ($1, $2, $3, NULLIF($4, 0), $5, $6, $7, $8, $9)
		ON CONFLICT DO NOTHING`,
		input.SourceType, input.SourceID, input.Audience, input.UserID, pq.Array(input.Recipients),
		input.TemplateEvent, variables, dedupKey, status)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted > 0 || status == "no_recipient" {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO security_notification_outbox
			(source_type, source_id, audience, user_id, recipients, template_event, variables, status)
		VALUES ($1, $2, $3, NULLIF($4, 0), $5, $6, $7, 'suppressed')
		ON CONFLICT (source_type, source_id, audience) DO NOTHING`,
		input.SourceType, input.SourceID, input.Audience, input.UserID, pq.Array(input.Recipients),
		input.TemplateEvent, variables)
	return err
}

func (r *securityNotificationRepository) ReclaimStale(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE security_notification_outbox
		SET status = 'retry', claimed_at = NULL, available_at = NOW(), updated_at = NOW(),
			last_error = 'stale claim recovered'
		WHERE status = 'processing' AND claimed_at < NOW() - INTERVAL '5 minutes'`)
	return err
}

func (r *securityNotificationRepository) Claim(ctx context.Context) (*service.SecurityNotificationOutboxItem, error) {
	var item service.SecurityNotificationOutboxItem
	var userID sql.NullInt64
	var recipients, sentHashes pq.StringArray
	var variables []byte
	err := r.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id FROM security_notification_outbox
			WHERE status IN ('pending', 'retry') AND available_at <= NOW()
			ORDER BY available_at, id FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE security_notification_outbox o
		SET status = 'processing', attempts = attempts + 1, claimed_at = NOW(), updated_at = NOW()
		FROM candidate c WHERE o.id = c.id
		RETURNING o.id, o.source_type, o.source_id, o.audience, o.user_id,
			o.recipients, o.sent_recipient_hashes, o.template_event, o.variables,
			o.attempts, o.max_attempts`).Scan(
		&item.ID, &item.SourceType, &item.SourceID, &item.Audience, &userID,
		&recipients, &sentHashes, &item.TemplateEvent, &variables,
		&item.Attempts, &item.MaxAttempts)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		item.UserID = userID.Int64
	}
	item.Recipients = append([]string(nil), recipients...)
	item.SentRecipientHashes = append([]string(nil), sentHashes...)
	if err := json.Unmarshal(variables, &item.Variables); err != nil {
		return nil, fmt.Errorf("decode security notification variables: %w", err)
	}
	return &item, nil
}

func (r *securityNotificationRepository) MarkRecipientSent(ctx context.Context, id int64, recipientHash string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE security_notification_outbox
		SET sent_recipient_hashes = CASE
				WHEN $2 = ANY(sent_recipient_hashes) THEN sent_recipient_hashes
				ELSE array_append(sent_recipient_hashes, $2)
			END,
			claimed_at = NOW(), updated_at = NOW()
		WHERE id = $1`, id, recipientHash)
	return err
}

func (r *securityNotificationRepository) MarkSent(ctx context.Context, item service.SecurityNotificationOutboxItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `
		UPDATE security_notification_outbox
		SET status = 'sent', claimed_at = NULL, last_error = '', updated_at = NOW()
		WHERE id = $1`, item.ID); err != nil {
		return err
	}
	if item.SourceType == service.SecurityNotificationSourceCyberPolicy && item.Audience == "user" {
		if _, err = tx.ExecContext(ctx, `
			UPDATE content_moderation_logs SET email_sent = TRUE WHERE id = $1`, item.SourceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *securityNotificationRepository) MarkFailed(ctx context.Context, item service.SecurityNotificationOutboxItem, sendErr error, delay time.Duration) error {
	status := "retry"
	if item.Attempts >= item.MaxAttempts {
		status = "failed"
	}
	message := "notification failed"
	if sendErr != nil {
		message = sendErr.Error()
	}
	if len(message) > 512 {
		message = message[:512]
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE security_notification_outbox
		SET status = $2, claimed_at = NULL, available_at = NOW() + ($3 * INTERVAL '1 second'),
			last_error = $4, updated_at = NOW()
		WHERE id = $1`, item.ID, status, int(delay.Seconds()), message)
	return err
}
