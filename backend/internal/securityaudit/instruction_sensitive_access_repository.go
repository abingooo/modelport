package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const instructionSensitiveGrantLockKey int64 = 0x4d505347

var (
	errInstructionSensitiveGrantNotFound    = errors.New("instruction sensitive access grant not found")
	errInstructionSensitiveTargetNotAdmin   = errors.New("instruction sensitive access target is not an administrator")
	errInstructionSensitiveTargetInactive   = errors.New("instruction sensitive access target is inactive")
	errInstructionSensitiveTargetTotpNeeded = errors.New("instruction sensitive access target requires TOTP")
	errInstructionSensitiveLastHolder       = errors.New("instruction sensitive access last holder")
)

const instructionSensitiveGrantColumns = `
	g.id, u.id, u.email, u.username, u.status, u.totp_enabled,
	g.granted_by, g.grant_source, g.grant_reason, g.granted_at`

func (r *InstructionRepository) GetActiveInstructionSensitiveGrant(
	ctx context.Context,
	userID int64,
) (*InstructionSensitiveGrant, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	return scanInstructionSensitiveGrant(r.db.QueryRowContext(ctx, `
		SELECT `+instructionSensitiveGrantColumns+`
		FROM instruction_audit_sensitive_access_grants g
		JOIN users u ON u.id = g.subject_user_id
		WHERE g.subject_user_id = $1
		  AND g.revoked_at IS NULL
		  AND u.role = 'admin'
		  AND u.status = 'active'
		  AND u.deleted_at IS NULL
		ORDER BY g.id DESC
		LIMIT 1`, userID))
}

func (r *InstructionRepository) GetActiveInstructionSensitiveGrantByID(
	ctx context.Context,
	userID int64,
	grantID int64,
) (*InstructionSensitiveGrant, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	return scanInstructionSensitiveGrant(r.db.QueryRowContext(ctx, `
		SELECT `+instructionSensitiveGrantColumns+`
		FROM instruction_audit_sensitive_access_grants g
		JOIN users u ON u.id = g.subject_user_id
		WHERE g.id = $1
		  AND g.subject_user_id = $2
		  AND g.revoked_at IS NULL
		  AND u.role = 'admin'
		  AND u.status = 'active'
		  AND u.deleted_at IS NULL`, grantID, userID))
}

func (r *InstructionRepository) ListActiveInstructionSensitiveGrants(
	ctx context.Context,
) ([]InstructionSensitiveGrant, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+instructionSensitiveGrantColumns+`
		FROM instruction_audit_sensitive_access_grants g
		JOIN users u ON u.id = g.subject_user_id
		WHERE g.revoked_at IS NULL
		  AND u.role = 'admin'
		  AND u.deleted_at IS NULL
		ORDER BY g.granted_at ASC, g.id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionSensitiveGrant, 0)
	for rows.Next() {
		item, scanErr := scanInstructionSensitiveGrant(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *InstructionRepository) GrantInstructionSensitiveAccess(
	ctx context.Context,
	actorID int64,
	actorGrantID int64,
	targetUserID int64,
	reason string,
) (*InstructionSensitiveGrant, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, instructionSensitiveGrantLockKey); err != nil {
		return nil, err
	}
	if _, err = getActiveInstructionSensitiveGrantTx(ctx, tx, actorID, actorGrantID); err != nil {
		return nil, err
	}

	var email, status string
	var totpEnabled bool
	err = tx.QueryRowContext(ctx, `
		SELECT email, status, totp_enabled
		FROM users
		WHERE id = $1 AND role = 'admin' AND deleted_at IS NULL
		FOR UPDATE`, targetUserID).Scan(&email, &status, &totpEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errInstructionSensitiveTargetNotAdmin
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, errInstructionSensitiveTargetInactive
	}
	if !totpEnabled {
		return nil, errInstructionSensitiveTargetTotpNeeded
	}

	existing, findErr := getActiveInstructionSensitiveGrantForUserTx(ctx, tx, targetUserID)
	if findErr == nil {
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if !errors.Is(findErr, sql.ErrNoRows) {
		return nil, findErr
	}

	var grantID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_sensitive_access_grants
			(subject_user_id, subject_email_snapshot, granted_by, grant_source, grant_reason)
		VALUES ($1, $2, $3, 'manual', $4)
		RETURNING id`, targetUserID, email, actorID, strings.TrimSpace(reason)).Scan(&grantID)
	if err != nil {
		return nil, err
	}
	item, err := getActiveInstructionSensitiveGrantTx(ctx, tx, targetUserID, grantID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *InstructionRepository) RevokeInstructionSensitiveAccess(
	ctx context.Context,
	actorID int64,
	actorGrantID int64,
	targetUserID int64,
	reason string,
) (*InstructionSensitiveGrant, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, instructionSensitiveGrantLockKey); err != nil {
		return nil, err
	}
	if _, err = getActiveInstructionSensitiveGrantTx(ctx, tx, actorID, actorGrantID); err != nil {
		return nil, err
	}
	target, err := getActiveInstructionSensitiveGrantForUserTx(ctx, tx, targetUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errInstructionSensitiveGrantNotFound
	}
	if err != nil {
		return nil, err
	}

	var effectiveHolders int64
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM instruction_audit_sensitive_access_grants g
		JOIN users u ON u.id = g.subject_user_id
		WHERE g.revoked_at IS NULL
		  AND u.role = 'admin'
		  AND u.status = 'active'
		  AND u.totp_enabled = TRUE
		  AND u.deleted_at IS NULL`).Scan(&effectiveHolders)
	if err != nil {
		return nil, err
	}
	if target.UserStatus == "active" && effectiveHolders <= 1 {
		return nil, errInstructionSensitiveLastHolder
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE instruction_audit_sensitive_access_grants
		SET revoked_by = $1, revoke_source = 'manual', revoke_reason = $2, revoked_at = NOW()
		WHERE id = $3 AND revoked_at IS NULL`, actorID, strings.TrimSpace(reason), target.ID)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, errInstructionSensitiveGrantNotFound
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	target.Effective = false
	return target, nil
}

func getActiveInstructionSensitiveGrantForUserTx(
	ctx context.Context,
	tx *sql.Tx,
	userID int64,
) (*InstructionSensitiveGrant, error) {
	return scanInstructionSensitiveGrant(tx.QueryRowContext(ctx, `
		SELECT `+instructionSensitiveGrantColumns+`
		FROM instruction_audit_sensitive_access_grants g
		JOIN users u ON u.id = g.subject_user_id
		WHERE g.subject_user_id = $1
		  AND g.revoked_at IS NULL
		  AND u.role = 'admin'
		  AND u.deleted_at IS NULL
		ORDER BY g.id DESC
		LIMIT 1
		FOR UPDATE OF g`, userID))
}

func getActiveInstructionSensitiveGrantTx(
	ctx context.Context,
	tx *sql.Tx,
	userID int64,
	grantID int64,
) (*InstructionSensitiveGrant, error) {
	return scanInstructionSensitiveGrant(tx.QueryRowContext(ctx, `
		SELECT `+instructionSensitiveGrantColumns+`
		FROM instruction_audit_sensitive_access_grants g
		JOIN users u ON u.id = g.subject_user_id
		WHERE g.id = $1
		  AND g.subject_user_id = $2
		  AND g.revoked_at IS NULL
		  AND u.role = 'admin'
		  AND u.status = 'active'
		  AND u.deleted_at IS NULL
		FOR UPDATE OF g`, grantID, userID))
}

func scanInstructionSensitiveGrant(scanner instructionScanner) (*InstructionSensitiveGrant, error) {
	var item InstructionSensitiveGrant
	var grantedBy sql.NullInt64
	err := scanner.Scan(
		&item.ID, &item.UserID, &item.Email, &item.Username, &item.UserStatus,
		&item.TotpEnabled, &grantedBy, &item.GrantSource, &item.GrantReason, &item.GrantedAt,
	)
	if grantedBy.Valid {
		value := grantedBy.Int64
		item.GrantedBy = &value
	}
	item.GrantedAt = item.GrantedAt.UTC()
	item.Effective = item.UserStatus == "active" && item.TotpEnabled
	return &item, err
}

func instructionSensitiveAuditAuthorization(ctx context.Context) (any, string, string) {
	authorization, ok := instructionSensitiveAuthorizationFromContext(ctx)
	if !ok {
		return nil, "legacy", "legacy"
	}
	return authorization.GrantID, authorization.AuthMethod, authorization.AuthorizationResult
}

func instructionSensitiveGrantValidAt(grant *InstructionSensitiveGrant, now time.Time) bool {
	return grant != nil && grant.ID > 0 && grant.UserID > 0 && grant.Effective && grant.TotpEnabled && !grant.GrantedAt.After(now)
}
