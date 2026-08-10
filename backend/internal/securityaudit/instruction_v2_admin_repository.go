package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

func (r *InstructionV2Repository) ListScopes(ctx context.Context) ([]InstructionV2Scope, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.group_id, g.name, g.platform, g.status, s.client_profile_id,
		       COALESCE(p.profile_key, ''), COALESCE(p.name, '全部客户端'), s.enabled,
		       (s.enabled AND g.deleted_at IS NULL AND g.status = 'active'
		        AND (s.client_profile_id IS NULL OR p.enabled)) AS effective,
		       s.created_by, s.updated_by, s.created_at, s.updated_at
		FROM instruction_audit_v2_scopes s
		JOIN groups g ON g.id = s.group_id
		LEFT JOIN instruction_audit_v2_client_profiles p ON p.id = s.client_profile_id
		ORDER BY g.name, CASE WHEN s.client_profile_id IS NULL THEN 0 ELSE 1 END,
		         COALESCE(p.priority, 0), s.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2Scope, 0)
	for rows.Next() {
		var item InstructionV2Scope
		var profileID sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.GroupID, &item.GroupName, &item.GroupPlatform, &item.GroupStatus,
			&profileID, &item.ClientProfileKey, &item.ClientProfileName, &item.Enabled,
			&item.Effective, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if profileID.Valid {
			value := profileID.Int64
			item.ClientProfileID = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) SaveScope(ctx context.Context, id int64, request SaveInstructionV2ScopeRequest, actorID int64) (InstructionV2Scope, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2Scope{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var scopeID int64
	if id > 0 {
		err = tx.QueryRowContext(ctx, `
			UPDATE instruction_audit_v2_scopes
			SET group_id = $2, client_profile_id = $3, enabled = $4,
			    updated_by = NULLIF($5, 0), updated_at = NOW()
			WHERE id = $1 RETURNING id`, id, request.GroupID, request.ClientProfileID, request.Enabled, actorID).Scan(&scopeID)
	} else if request.ClientProfileID == nil {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_v2_scopes
				(group_id, client_profile_id, enabled, created_by, updated_by)
			VALUES ($1, NULL, $2, NULLIF($3, 0), NULLIF($3, 0))
			ON CONFLICT (group_id) WHERE client_profile_id IS NULL
			DO UPDATE SET enabled = EXCLUDED.enabled, updated_by = NULLIF($3, 0), updated_at = NOW()
			RETURNING id`, request.GroupID, request.Enabled, actorID).Scan(&scopeID)
	} else {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_v2_scopes
				(group_id, client_profile_id, enabled, created_by, updated_by)
			VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($4, 0))
			ON CONFLICT (group_id, client_profile_id) WHERE client_profile_id IS NOT NULL
			DO UPDATE SET enabled = EXCLUDED.enabled, updated_by = NULLIF($4, 0), updated_at = NOW()
			RETURNING id`, request.GroupID, request.ClientProfileID, request.Enabled, actorID).Scan(&scopeID)
	}
	if err != nil {
		return InstructionV2Scope{}, 0, err
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return InstructionV2Scope{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2Scope{}, 0, err
	}
	items, err := r.ListScopes(ctx)
	if err != nil {
		return InstructionV2Scope{}, 0, err
	}
	for _, item := range items {
		if item.ID == scopeID {
			return item, version, nil
		}
	}
	return InstructionV2Scope{}, 0, sql.ErrNoRows
}

func (r *InstructionV2Repository) DeleteScope(ctx context.Context, id, actorID int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_scopes WHERE id = $1`, id)
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

func (r *InstructionV2Repository) ListGroupOptions(ctx context.Context) ([]InstructionGroupOption, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, platform, status FROM groups
		WHERE deleted_at IS NULL ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionGroupOption, 0)
	for rows.Next() {
		var item InstructionGroupOption
		if err := rows.Scan(&item.ID, &item.Name, &item.Platform, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) ListUserAllowlist(ctx context.Context) ([]InstructionV2UserAllowlistEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id, a.user_id, u.email, u.username, a.note, a.enabled,
		       a.created_by, a.updated_by, a.created_at, a.updated_at
		FROM instruction_audit_v2_user_allowlist a
		JOIN users u ON u.id = a.user_id
		ORDER BY u.email, a.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2UserAllowlistEntry, 0)
	for rows.Next() {
		var item InstructionV2UserAllowlistEntry
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Email, &item.Username, &item.Note, &item.Enabled,
			&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) ListUserOptions(ctx context.Context, query string) ([]InstructionV2UserOption, error) {
	query = strings.TrimSpace(query)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, email, username, status
		FROM users
		WHERE deleted_at IS NULL
		  AND ($1 = '' OR email ILIKE '%' || $1 || '%' OR username ILIKE '%' || $1 || '%'
		       OR CAST(id AS TEXT) = $1)
		ORDER BY CASE WHEN email = $1 THEN 0 ELSE 1 END, email, id
		LIMIT 50`, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2UserOption, 0, 50)
	for rows.Next() {
		var item InstructionV2UserOption
		if err := rows.Scan(&item.ID, &item.Email, &item.Username, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) SaveUserAllowlist(ctx context.Context, request SaveInstructionV2UserAllowlistRequest, actorID int64) (InstructionV2UserAllowlistEntry, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2UserAllowlistEntry{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_v2_user_allowlist
			(user_id, note, enabled, created_by, updated_by)
		VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($4, 0))
		ON CONFLICT (user_id) DO UPDATE
		SET note = EXCLUDED.note, enabled = EXCLUDED.enabled,
		    updated_by = NULLIF($4, 0), updated_at = NOW()
		RETURNING id`, request.UserID, request.Note, request.Enabled, actorID).Scan(&id)
	if err != nil {
		return InstructionV2UserAllowlistEntry{}, 0, err
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return InstructionV2UserAllowlistEntry{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2UserAllowlistEntry{}, 0, err
	}
	items, err := r.ListUserAllowlist(ctx)
	if err != nil {
		return InstructionV2UserAllowlistEntry{}, 0, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, version, nil
		}
	}
	return InstructionV2UserAllowlistEntry{}, 0, sql.ErrNoRows
}

func (r *InstructionV2Repository) DeleteUserAllowlist(ctx context.Context, id, actorID int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_user_allowlist WHERE id = $1`, id)
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

func (r *InstructionV2Repository) ListHashes(ctx context.Context, page, pageSize int, status, query string) (InstructionV2HashPage, error) {
	where := []string{"1 = 1"}
	args := make([]any, 0, 4)
	if status = strings.TrimSpace(status); status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("h.status = $%d", len(args)))
	}
	if query = strings.TrimSpace(query); query != "" {
		args = append(args, "%"+query+"%")
		where = append(where, fmt.Sprintf("(h.sha256 ILIKE $%d OR h.name ILIKE $%d OR h.note ILIKE $%d)", len(args), len(args), len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM instruction_audit_v2_hashes h WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return InstructionV2HashPage{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		SELECT h.id, h.sha256, h.name, h.note, h.status, h.source, h.observed_field,
		       h.hash_algorithm, h.normalization_version, h.content_bytes, h.raw_storage,
		       h.stored_bytes, h.ai_sampled, h.source_event_id, h.reviewer_node_id,
		       h.reviewer_model, h.prompt_version, h.confidence, h.review_reason,
		       h.review_category, h.candidate_expires_at, h.created_by, h.updated_by,
		       h.created_at, h.updated_at, h.global_trust, h.content_vault_id
		FROM instruction_audit_v2_hashes h WHERE `+whereSQL+`
		ORDER BY h.created_at DESC, h.id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return InstructionV2HashPage{}, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2Hash, 0, pageSize)
	for rows.Next() {
		var item InstructionV2Hash
		if err := rows.Scan(
			&item.ID, &item.SHA256, &item.Name, &item.Note, &item.Status, &item.Source,
			&item.ObservedField, &item.HashAlgorithm, &item.NormalizationVersion, &item.ContentBytes,
			&item.RawStorage, &item.StoredBytes, &item.AISampled, &item.SourceEventID,
			&item.ReviewerNodeID, &item.ReviewerModel, &item.PromptVersion, &item.Confidence,
			&item.ReviewReason, &item.ReviewCategory, &item.CandidateExpiresAt, &item.CreatedBy,
			&item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &item.GlobalTrust,
			&item.ContentVaultID,
		); err != nil {
			return InstructionV2HashPage{}, err
		}
		item.SHA256 = strings.TrimSpace(item.SHA256)
		ensureInstructionV2HashCollections(&item)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return InstructionV2HashPage{}, err
	}
	if len(items) > 0 {
		byID := make(map[int64]*InstructionV2Hash, len(items))
		for index := range items {
			byID[items[index].ID] = &items[index]
		}
		ids := make([]int64, 0, len(byID))
		for id := range byID {
			ids = append(ids, id)
		}
		scopeRows, err := r.db.QueryContext(ctx, `
			SELECT hs.hash_id, s.id, s.group_id, g.name, s.client_profile_id,
			       COALESCE(p.profile_key, ''), COALESCE(p.name, '全部客户端'),
			       hs.status, hs.source, hs.candidate_expires_at, hs.created_at, hs.updated_at
			FROM instruction_audit_v2_hash_scopes hs
			JOIN instruction_audit_v2_scopes s ON s.id = hs.scope_id
			JOIN groups g ON g.id = s.group_id
			LEFT JOIN instruction_audit_v2_client_profiles p ON p.id = s.client_profile_id
			WHERE hs.hash_id = ANY($1) ORDER BY hs.hash_id, g.name, s.id`, pq.Array(ids))
		if err != nil {
			return InstructionV2HashPage{}, err
		}
		if err := scanInstructionV2HashScopes(scopeRows, byID); err != nil {
			_ = scopeRows.Close()
			return InstructionV2HashPage{}, err
		}
		if err := scopeRows.Close(); err != nil {
			return InstructionV2HashPage{}, err
		}
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return InstructionV2HashPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *InstructionV2Repository) GetHash(ctx context.Context, id int64) (InstructionV2Hash, []byte, error) {
	var item InstructionV2Hash
	var ciphertext []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT h.id, h.sha256, h.name, h.note, h.status, h.source, h.observed_field,
		       h.hash_algorithm, h.normalization_version,
		       CASE WHEN vault.id IS NULL THEN h.content_bytes ELSE vault.content_bytes END,
		       CASE WHEN vault.id IS NULL THEN h.raw_storage ELSE 'full' END,
		       COALESCE(vault.raw_ciphertext, h.raw_ciphertext),
		       CASE WHEN vault.id IS NULL THEN h.stored_bytes ELSE vault.stored_bytes END,
		       h.ai_sampled, h.source_event_id, h.reviewer_node_id, h.reviewer_model,
		       h.prompt_version, h.confidence, h.review_reason, h.review_category,
		       h.candidate_expires_at, h.created_by, h.updated_by, h.created_at,
		       h.updated_at, h.global_trust, h.content_vault_id
		FROM instruction_audit_v2_hashes h
		LEFT JOIN instruction_audit_v2_content_vault vault ON vault.id = h.content_vault_id
		WHERE h.id = $1`, id).Scan(
		&item.ID, &item.SHA256, &item.Name, &item.Note, &item.Status, &item.Source,
		&item.ObservedField, &item.HashAlgorithm, &item.NormalizationVersion, &item.ContentBytes,
		&item.RawStorage, &ciphertext, &item.StoredBytes, &item.AISampled, &item.SourceEventID,
		&item.ReviewerNodeID, &item.ReviewerModel, &item.PromptVersion, &item.Confidence,
		&item.ReviewReason, &item.ReviewCategory, &item.CandidateExpiresAt, &item.CreatedBy,
		&item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &item.GlobalTrust,
		&item.ContentVaultID,
	)
	if err != nil {
		return InstructionV2Hash{}, nil, err
	}
	item.SHA256 = strings.TrimSpace(item.SHA256)
	ensureInstructionV2HashCollections(&item)
	byID := map[int64]*InstructionV2Hash{item.ID: &item}
	rows, err := r.db.QueryContext(ctx, `
		SELECT hs.hash_id, s.id, s.group_id, g.name, s.client_profile_id,
		       COALESCE(p.profile_key, ''), COALESCE(p.name, '全部客户端'),
		       hs.status, hs.source, hs.candidate_expires_at, hs.created_at, hs.updated_at
		FROM instruction_audit_v2_hash_scopes hs
		JOIN instruction_audit_v2_scopes s ON s.id = hs.scope_id
		JOIN groups g ON g.id = s.group_id
		LEFT JOIN instruction_audit_v2_client_profiles p ON p.id = s.client_profile_id
		WHERE hs.hash_id = $1 ORDER BY g.name, s.id`, item.ID)
	if err != nil {
		return InstructionV2Hash{}, nil, err
	}
	if err := scanInstructionV2HashScopes(rows, byID); err != nil {
		_ = rows.Close()
		return InstructionV2Hash{}, nil, err
	}
	if err := rows.Close(); err != nil {
		return InstructionV2Hash{}, nil, err
	}
	return item, ciphertext, nil
}

func ensureInstructionV2HashCollections(item *InstructionV2Hash) {
	if item == nil {
		return
	}
	if item.ScopeIDs == nil {
		item.ScopeIDs = make([]int64, 0)
	}
	if item.Scopes == nil {
		item.Scopes = make([]InstructionV2HashScope, 0)
	}
}

func (r *InstructionV2Repository) SaveManualHash(ctx context.Context, write instructionV2ManualHashWrite, actorID int64) (InstructionV2Hash, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2Hash{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	hashID, err := saveInstructionV2ManualHashTx(ctx, tx, write, actorID)
	if err != nil {
		return InstructionV2Hash{}, 0, err
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return InstructionV2Hash{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2Hash{}, 0, err
	}
	item, _, err := r.GetHash(ctx, hashID)
	return item, version, err
}

func (r *InstructionV2Repository) SaveManualHashes(
	ctx context.Context,
	writes []instructionV2ManualHashWrite,
	actorID int64,
) ([]InstructionV2Hash, int64, error) {
	if len(writes) == 0 {
		return []InstructionV2Hash{}, 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	ids := make([]int64, 0, len(writes))
	for _, write := range writes {
		hashID, saveErr := saveInstructionV2ManualHashTx(ctx, tx, write, actorID)
		if saveErr != nil {
			return nil, 0, saveErr
		}
		ids = append(ids, hashID)
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	items := make([]InstructionV2Hash, 0, len(ids))
	for _, hashID := range ids {
		item, _, getErr := r.GetHash(ctx, hashID)
		if getErr != nil {
			return nil, 0, getErr
		}
		items = append(items, item)
	}
	return items, version, nil
}

func saveInstructionV2ManualHashTx(
	ctx context.Context,
	tx *sql.Tx,
	write instructionV2ManualHashWrite,
	actorID int64,
) (int64, error) {
	var activeRisk bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM instruction_audit_v2_risk_hashes
			WHERE sha256 = $1 AND status = 'active'
		)`, write.SHA256).Scan(&activeRisk); err != nil {
		return 0, err
	}
	if activeRisk {
		return 0, errors.New("instruction audit risk hash takes precedence")
	}
	var vaultID any
	if len(write.RawCiphertext) > 0 {
		id, err := upsertInstructionV2VaultTx(ctx, tx, instructionV2VaultWrite{
			SHA256: write.SHA256, ObservedField: write.ObservedField,
			RawCiphertext: write.RawCiphertext, ContentBytes: write.ContentBytes,
			StoredBytes: write.StoredBytes,
		})
		if err != nil {
			return 0, err
		}
		vaultID = id
	}
	var hashID int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_v2_hashes
			(sha256, name, note, status, source, observed_field, content_bytes,
			 raw_storage, raw_ciphertext, stored_bytes, candidate_expires_at,
			 global_trust, content_vault_id, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'unavailable', NULL, 0, NULL,
		        $8, $9, NULLIF($10, 0), NULLIF($10, 0))
		ON CONFLICT (sha256) DO UPDATE
		SET name = CASE WHEN EXCLUDED.name = '' THEN instruction_audit_v2_hashes.name ELSE EXCLUDED.name END,
		    note = CASE WHEN EXCLUDED.note = '' THEN instruction_audit_v2_hashes.note ELSE EXCLUDED.note END,
		    status = EXCLUDED.status,
		    observed_field = CASE WHEN EXCLUDED.observed_field = ''
		        THEN instruction_audit_v2_hashes.observed_field ELSE EXCLUDED.observed_field END,
		    content_bytes = GREATEST(instruction_audit_v2_hashes.content_bytes, EXCLUDED.content_bytes),
		    global_trust = EXCLUDED.global_trust,
		    content_vault_id = COALESCE(EXCLUDED.content_vault_id,
		        instruction_audit_v2_hashes.content_vault_id),
		    candidate_expires_at = NULL,
		    updated_by = NULLIF($10, 0), updated_at = NOW()
		WHERE instruction_audit_v2_hashes.status <> 'revoked'
		RETURNING id`, write.SHA256, write.Name, write.Note, write.Status, write.Source,
		write.ObservedField, write.ContentBytes, write.GlobalTrust, vaultID, actorID).Scan(&hashID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errInstructionV2RevokedHash
	}
	if err != nil {
		return 0, err
	}
	for _, scopeID := range write.ScopeIDs {
		if write.GlobalTrust {
			break
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_v2_hash_scopes
				(hash_id, scope_id, status, source, candidate_expires_at, created_by, updated_by)
			VALUES ($1, $2, $3, $4, NULL, NULLIF($5, 0), NULLIF($5, 0))
			ON CONFLICT (hash_id, scope_id) DO UPDATE
			SET status = EXCLUDED.status, source = EXCLUDED.source,
			    candidate_expires_at = NULL,
			    updated_by = NULLIF($5, 0), updated_at = NOW()`,
			hashID, scopeID, write.Status, write.Source, actorID)
		if err != nil {
			return 0, err
		}
	}
	return hashID, nil
}

func (r *InstructionV2Repository) UpdateHash(ctx context.Context, id int64, request UpdateInstructionV2HashRequest, actorID int64, _ int) (InstructionV2Hash, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2Hash{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var currentStatus, name, note string
	var globalTrust bool
	err = tx.QueryRowContext(ctx, `
		SELECT status, name, note, global_trust
		FROM instruction_audit_v2_hashes WHERE id = $1 FOR UPDATE`, id).Scan(
		&currentStatus, &name, &note, &globalTrust,
	)
	if err != nil {
		return InstructionV2Hash{}, 0, err
	}
	if request.Name != nil {
		name = strings.TrimSpace(*request.Name)
	}
	if request.Note != nil {
		note = strings.TrimSpace(*request.Note)
	}
	nextStatus := currentStatus
	if request.Status != nil {
		nextStatus = strings.TrimSpace(*request.Status)
		if currentStatus == "revoked" && nextStatus != "revoked" {
			return InstructionV2Hash{}, 0, errInstructionV2RevokedHash
		}
	}
	if request.GlobalTrust != nil {
		globalTrust = *request.GlobalTrust
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE instruction_audit_v2_hashes
		SET name = $2, note = $3, status = $4, global_trust = $5,
		    candidate_expires_at = NULL,
		    updated_by = NULLIF($6, 0), updated_at = NOW()
		WHERE id = $1`, id, name, note, nextStatus, globalTrust, actorID)
	if err != nil {
		return InstructionV2Hash{}, 0, err
	}
	if globalTrust {
		if _, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_hash_scopes WHERE hash_id = $1`, id); err != nil {
			return InstructionV2Hash{}, 0, err
		}
	} else if request.SetScopes {
		if _, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_hash_scopes WHERE hash_id = $1`, id); err != nil {
			return InstructionV2Hash{}, 0, err
		}
		for _, scopeID := range request.ScopeIDs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO instruction_audit_v2_hash_scopes
					(hash_id, scope_id, status, source, candidate_expires_at, created_by, updated_by)
				VALUES ($1, $2, $3, 'manual', NULL, NULLIF($4, 0), NULLIF($4, 0))`,
				id, scopeID, nextStatus, actorID); err != nil {
				return InstructionV2Hash{}, 0, err
			}
		}
	} else if request.Status != nil {
		_, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_v2_hash_scopes
			SET status = $2, candidate_expires_at = NULL,
			    updated_by = NULLIF($3, 0), updated_at = NOW()
			WHERE hash_id = $1`, id, nextStatus, actorID)
		if err != nil {
			return InstructionV2Hash{}, 0, err
		}
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return InstructionV2Hash{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2Hash{}, 0, err
	}
	item, _, err := r.GetHash(ctx, id)
	return item, version, err
}

func (r *InstructionV2Repository) DeleteHash(ctx context.Context, id, actorID int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_hashes WHERE id = $1`, id)
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

func (r *InstructionV2Repository) GetEventEvidence(ctx context.Context, eventID int64) ([]instructionV2EvidenceWrite, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT field_name, sha256, storage_kind, ciphertext, content_bytes, stored_bytes, expires_at
		FROM instruction_audit_v2_event_evidence WHERE event_id = $1
		UNION ALL
		SELECT event.selected_field, vault.sha256, 'vault', vault.raw_ciphertext,
		       vault.content_bytes, vault.stored_bytes, NOW() + INTERVAL '100 years'
		FROM instruction_audit_v2_events event
		JOIN instruction_audit_v2_content_vault vault
		  ON vault.sha256 = event.selected_sha256
		WHERE event.id = $1
		  AND event.selected_field <> ''
		  AND NOT EXISTS (
		      SELECT 1 FROM instruction_audit_v2_event_evidence evidence
		      WHERE evidence.event_id = event.id
		  )
		ORDER BY field_name`, eventID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]instructionV2EvidenceWrite, 0, 2)
	for rows.Next() {
		var item instructionV2EvidenceWrite
		if err := rows.Scan(&item.FieldName, &item.SHA256, &item.StorageKind, &item.Ciphertext, &item.ContentBytes, &item.StoredBytes, &item.ExpiresAt); err != nil {
			return nil, err
		}
		item.SHA256 = strings.TrimSpace(item.SHA256)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) RecordRawAccess(ctx context.Context, resourceType string, resourceID int64, access InstructionV2RawAccess, succeeded bool, errorCode string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO instruction_audit_v2_raw_access_logs
			(resource_type, resource_id, field_name, action, actor_id, request_id,
			 client_ip, user_agent, succeeded, error_code)
		VALUES ($1, $2, $3, $4, NULLIF($5, 0), $6, $7, $8, $9, $10)`,
		resourceType, resourceID, access.FieldName, access.Action, access.ActorID,
		access.RequestID, access.ClientIP, access.UserAgent, succeeded, errorCode)
	return err
}

func (r *InstructionV2Repository) Cleanup(ctx context.Context, config InstructionV2Config) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_event_evidence WHERE expires_at <= NOW()`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM security_notification_outbox o
		WHERE o.source_type = 'instruction_audit_v2'
		  AND EXISTS (
			SELECT 1 FROM instruction_audit_v2_events e
			WHERE e.id = o.source_id
			  AND e.created_at < NOW() - ($1 * INTERVAL '1 day')
		  )`, config.EventRetentionDays); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM instruction_audit_v2_events
		WHERE created_at < NOW() - ($1 * INTERVAL '1 day')`, config.EventRetentionDays); err != nil {
		return err
	}
	return tx.Commit()
}
