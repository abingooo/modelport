package securityaudit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

type InstructionRepository struct{ db *sql.DB }

func NewInstructionRepository(db *sql.DB) *InstructionRepository {
	return &InstructionRepository{db: db}
}

type instructionPolicyAccumulator struct {
	ruleSets map[int64]struct{}
	hashes   map[[32]byte]instructionPolicyHash
}

func (r *InstructionRepository) LoadSnapshot(ctx context.Context) (*instructionSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var enabled bool
	var version int64
	if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE((SELECT value = 'true' FROM settings WHERE key = $1), FALSE), config_version
			FROM instruction_audit_state WHERE id = 1`, SettingKeyInstructionAuditEnabled).Scan(&enabled, &version); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
			SELECT b.group_id, rs.id, rs.enabled, h.digest, h.valid_from, h.valid_until
			FROM instruction_audit_group_bindings b
			JOIN instruction_audit_rule_sets rs ON rs.id = b.rule_set_id
			LEFT JOIN instruction_audit_rule_set_hashes rsh ON rsh.rule_set_id = rs.id AND rs.enabled = TRUE
			LEFT JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
				AND h.status = 'active'
			WHERE b.enabled = TRUE
			ORDER BY b.group_id, rs.id, h.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accumulators := make(map[int64]*instructionPolicyAccumulator)
	auditedGroups := make(map[int64]struct{})
	for rows.Next() {
		var groupID, ruleSetID int64
		var ruleSetEnabled bool
		var digest sql.NullString
		var validFrom, validUntil sql.NullTime
		if err := rows.Scan(&groupID, &ruleSetID, &ruleSetEnabled, &digest, &validFrom, &validUntil); err != nil {
			return nil, err
		}
		auditedGroups[groupID] = struct{}{}
		acc := accumulators[groupID]
		if acc == nil {
			acc = &instructionPolicyAccumulator{ruleSets: make(map[int64]struct{}), hashes: make(map[[32]byte]instructionPolicyHash)}
			accumulators[groupID] = acc
		}
		if ruleSetEnabled {
			acc.ruleSets[ruleSetID] = struct{}{}
		}
		if digest.Valid {
			decoded, decodeErr := hex.DecodeString(strings.TrimSpace(digest.String))
			if decodeErr != nil || len(decoded) != sha256.Size {
				return nil, fmt.Errorf("invalid instruction audit digest in database")
			}
			var value [32]byte
			copy(value[:], decoded)
			policyHash := instructionPolicyHash{Digest: value}
			if validFrom.Valid {
				policyHash.ValidFrom = validFrom.Time.UTC()
			}
			if validUntil.Valid {
				policyHash.ValidUntil = validUntil.Time.UTC()
			}
			acc.hashes[value] = policyHash
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	policies := make(map[int64]instructionPolicy, len(accumulators))
	for groupID, acc := range accumulators {
		policy := instructionPolicy{
			RuleSetIDs: make([]int64, 0, len(acc.ruleSets)),
			Hashes:     make([]instructionPolicyHash, 0, len(acc.hashes)),
		}
		for id := range acc.ruleSets {
			policy.RuleSetIDs = append(policy.RuleSetIDs, id)
		}
		for _, hash := range acc.hashes {
			policy.Hashes = append(policy.Hashes, hash)
		}
		sort.Slice(policy.RuleSetIDs, func(i, j int) bool { return policy.RuleSetIDs[i] < policy.RuleSetIDs[j] })
		sort.Slice(policy.Hashes, func(i, j int) bool {
			return bytes.Compare(policy.Hashes[i].Digest[:], policy.Hashes[j].Digest[:]) < 0
		})
		policies[groupID] = policy
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &instructionSnapshot{
		Enabled: enabled, ConfigVersion: version, AuditedGroups: auditedGroups,
		Policies: policies, LoadedAt: time.Now().UTC(),
	}, nil
}

func (r *InstructionRepository) GetConfigVersion(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("instruction audit repository unavailable")
	}
	var version int64
	err := r.db.QueryRowContext(ctx, `SELECT config_version FROM instruction_audit_state WHERE id = 1`).Scan(&version)
	return version, err
}

type instructionEnabledUpdateResult struct {
	Version int64
	Before  bool
}

func (r *InstructionRepository) SetEnabled(ctx context.Context, enabled bool) (result instructionEnabledUpdateResult, err error) {
	if r == nil || r.db == nil {
		return result, errors.New("instruction audit repository unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentRaw string
	err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1 FOR UPDATE`, SettingKeyInstructionAuditEnabled).Scan(&currentRaw)
	if errors.Is(err, sql.ErrNoRows) {
		currentRaw = "false"
	} else if err != nil {
		return result, err
	}
	current := currentRaw == "true"
	result.Before = current
	if enabled && !current {
		var effectiveGroups int64
		err = tx.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT b.group_id)
			FROM instruction_audit_group_bindings b
			JOIN instruction_audit_rule_sets rs ON rs.id = b.rule_set_id AND rs.enabled = TRUE
			JOIN instruction_audit_rule_set_hashes rsh ON rsh.rule_set_id = rs.id
			JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
				AND h.status = 'active'
				AND (h.valid_from IS NULL OR h.valid_from <= NOW())
				AND (h.valid_until IS NULL OR h.valid_until > NOW())
			WHERE b.enabled = TRUE`).Scan(&effectiveGroups)
		if err != nil {
			return result, err
		}
		if effectiveGroups == 0 {
			return result, ErrInstructionAuditNoEffectiveGroupRules
		}
	}
	if current == enabled {
		if err = tx.QueryRowContext(ctx, `SELECT config_version FROM instruction_audit_state WHERE id = 1`).Scan(&result.Version); err != nil {
			return result, err
		}
		if err = tx.Commit(); err != nil {
			return result, err
		}
		return result, nil
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		SettingKeyInstructionAuditEnabled, fmt.Sprintf("%t", enabled)); err != nil {
		return result, err
	}
	result.Version, err = bumpInstructionConfigTx(ctx, tx)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (r *InstructionRepository) BumpConfigVersion(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("instruction audit repository unavailable")
	}
	var version int64
	err := r.db.QueryRowContext(ctx, `
		UPDATE instruction_audit_state
		SET config_version = config_version + 1, updated_at = NOW()
		WHERE id = 1
		RETURNING config_version`).Scan(&version)
	return version, err
}

func bumpInstructionConfigTx(ctx context.Context, tx *sql.Tx) (int64, error) {
	var version int64
	err := tx.QueryRowContext(ctx, `
		UPDATE instruction_audit_state
		SET config_version = config_version + 1, updated_at = NOW()
		WHERE id = 1
		RETURNING config_version`).Scan(&version)
	return version, err
}

const instructionHashColumns = `id, digest, name, note, observed_source, client_name, client_version, status, valid_from, valid_until, created_by, created_at, updated_at`
const instructionHashColumnsAliased = `h.id, h.digest, h.name, h.note, h.observed_source, h.client_name, h.client_version, h.status, h.valid_from, h.valid_until, h.created_by, h.created_at, h.updated_at`

type instructionScanner interface{ Scan(...any) error }

func scanInstructionHash(scanner instructionScanner) (InstructionHashEntry, error) {
	var item InstructionHashEntry
	var validFrom, validUntil sql.NullTime
	var createdBy sql.NullInt64
	err := scanner.Scan(
		&item.ID, &item.Digest, &item.Name, &item.Note, &item.ObservedSource,
		&item.ClientName, &item.ClientVersion, &item.Status, &validFrom, &validUntil,
		&createdBy, &item.CreatedAt, &item.UpdatedAt,
	)
	if validFrom.Valid {
		item.ValidFrom = &validFrom.Time
	}
	if validUntil.Valid {
		item.ValidUntil = &validUntil.Time
	}
	if createdBy.Valid {
		item.CreatedBy = &createdBy.Int64
	}
	item.Digest = strings.TrimSpace(item.Digest)
	return item, err
}

func (r *InstructionRepository) ListHashes(ctx context.Context, status string) ([]InstructionHashEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+instructionHashColumns+`
		FROM instruction_audit_hashes
		WHERE ($1 = '' OR status = $1)
		ORDER BY CASE status WHEN 'candidate' THEN 0 WHEN 'active' THEN 1 WHEN 'disabled' THEN 2 ELSE 3 END, created_at DESC, id DESC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]InstructionHashEntry, 0)
	for rows.Next() {
		item, scanErr := scanInstructionHash(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionRepository) GetHash(ctx context.Context, id int64) (*InstructionHashEntry, error) {
	item, err := scanInstructionHash(r.db.QueryRowContext(ctx, `SELECT `+instructionHashColumns+` FROM instruction_audit_hashes WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *InstructionRepository) FindHashByDigest(ctx context.Context, digest string) (*InstructionHashEntry, error) {
	item, err := scanInstructionHash(r.db.QueryRowContext(ctx, `SELECT `+instructionHashColumns+` FROM instruction_audit_hashes WHERE digest = $1`, digest))
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *InstructionRepository) CreateHash(ctx context.Context, req CreateInstructionHashRequest, actorID int64) (*InstructionHashEntry, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	item, err := scanInstructionHash(tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_hashes
			(digest, name, note, observed_source, client_name, client_version, status, valid_from, valid_until, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, 0))
		RETURNING `+instructionHashColumns,
		req.Digest, req.Name, req.Note, req.ObservedSource, req.ClientName, req.ClientVersion,
		req.Status, req.ValidFrom, req.ValidUntil, actorID))
	if err != nil {
		return nil, err
	}
	if _, err = bumpInstructionConfigTx(ctx, tx); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *InstructionRepository) UpdateHash(ctx context.Context, item InstructionHashEntry) (*InstructionHashEntry, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := scanInstructionHash(tx.QueryRowContext(ctx, `
		UPDATE instruction_audit_hashes
		SET name = $2, note = $3, observed_source = $4, client_name = $5, client_version = $6,
			status = $7, valid_from = $8, valid_until = $9, updated_at = NOW()
		WHERE id = $1
		RETURNING `+instructionHashColumns,
		item.ID, item.Name, item.Note, item.ObservedSource, item.ClientName, item.ClientVersion,
		item.Status, item.ValidFrom, item.ValidUntil))
	if err != nil {
		return nil, err
	}
	if _, err = bumpInstructionConfigTx(ctx, tx); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *InstructionRepository) ListRuleSets(ctx context.Context) ([]InstructionRuleSet, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, enabled, version, created_at, updated_at
		FROM instruction_audit_rule_sets ORDER BY enabled DESC, name, id`)
	if err != nil {
		return nil, err
	}
	items := make([]InstructionRuleSet, 0)
	index := make(map[int64]int)
	for rows.Next() {
		var item InstructionRuleSet
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Enabled, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		item.Hashes = []InstructionHashEntry{}
		index[item.ID] = len(items)
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	hashRows, err := r.db.QueryContext(ctx, `
		SELECT rsh.rule_set_id, `+instructionHashColumnsAliased+`
		FROM instruction_audit_rule_set_hashes rsh
		JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
		ORDER BY rsh.rule_set_id, h.name, h.id`)
	if err != nil {
		return nil, err
	}
	defer hashRows.Close()
	for hashRows.Next() {
		var ruleSetID int64
		var hash InstructionHashEntry
		var validFrom, validUntil sql.NullTime
		var createdBy sql.NullInt64
		if err := hashRows.Scan(
			&ruleSetID, &hash.ID, &hash.Digest, &hash.Name, &hash.Note, &hash.ObservedSource,
			&hash.ClientName, &hash.ClientVersion, &hash.Status, &validFrom, &validUntil,
			&createdBy, &hash.CreatedAt, &hash.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if validFrom.Valid {
			hash.ValidFrom = &validFrom.Time
		}
		if validUntil.Valid {
			hash.ValidUntil = &validUntil.Time
		}
		if createdBy.Valid {
			hash.CreatedBy = &createdBy.Int64
		}
		hash.Digest = strings.TrimSpace(hash.Digest)
		if position, ok := index[ruleSetID]; ok {
			items[position].Hashes = append(items[position].Hashes, hash)
		}
	}
	return items, hashRows.Err()
}

func (r *InstructionRepository) SaveRuleSet(ctx context.Context, id int64, req SaveInstructionRuleSetRequest, actorID int64) (*InstructionRuleSet, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var savedID int64
	if id == 0 {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_rule_sets (name, description, enabled, created_by, updated_by)
			VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($4, 0)) RETURNING id`,
			req.Name, req.Description, req.Enabled, actorID).Scan(&savedID)
	} else {
		err = tx.QueryRowContext(ctx, `
			UPDATE instruction_audit_rule_sets
			SET name = $2, description = $3, enabled = $4, version = version + 1,
				updated_by = NULLIF($5, 0), updated_at = NOW()
			WHERE id = $1 RETURNING id`, id, req.Name, req.Description, req.Enabled, actorID).Scan(&savedID)
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM instruction_audit_rule_set_hashes WHERE rule_set_id = $1`, savedID); err != nil {
		return nil, err
	}
	for _, hashID := range uniquePositiveInt64s(req.HashIDs) {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_rule_set_hashes (rule_set_id, hash_id, created_by)
			VALUES ($1, $2, NULLIF($3, 0))`, savedID, hashID, actorID); err != nil {
			return nil, err
		}
	}
	if _, err = bumpInstructionConfigTx(ctx, tx); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	items, err := r.ListRuleSets(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == savedID {
			return &items[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *InstructionRepository) ListGroupBindings(ctx context.Context) ([]InstructionGroupBinding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.id, b.group_id, g.name, g.platform, g.status,
			b.rule_set_id, rs.name, b.enabled,
			(rs.enabled AND EXISTS (
				SELECT 1
				FROM instruction_audit_rule_set_hashes rsh
				JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
				WHERE rsh.rule_set_id = rs.id
					AND h.status = 'active'
					AND (h.valid_from IS NULL OR h.valid_from <= NOW())
					AND (h.valid_until IS NULL OR h.valid_until > NOW())
			)) AS effective,
			b.created_at, b.updated_at
		FROM instruction_audit_group_bindings b
		JOIN instruction_audit_rule_sets rs ON rs.id = b.rule_set_id
		JOIN groups g ON g.id = b.group_id
		ORDER BY b.enabled DESC, g.name, b.group_id, rs.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]InstructionGroupBinding, 0)
	for rows.Next() {
		var item InstructionGroupBinding
		if err := rows.Scan(&item.ID, &item.GroupID, &item.GroupName, &item.Platform, &item.GroupStatus,
			&item.RuleSetID, &item.RuleSetName, &item.Enabled, &item.Effective, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionRepository) ListGroupOptions(ctx context.Context) ([]InstructionGroupOption, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, platform, status
		FROM groups
		WHERE deleted_at IS NULL
		ORDER BY CASE WHEN status = 'active' THEN 0 ELSE 1 END, name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

func (r *InstructionRepository) SaveGroupBindings(ctx context.Context, req SaveInstructionGroupBindingsRequest, actorID int64) ([]InstructionGroupBinding, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	ids := make([]int64, 0, len(req.GroupIDs))
	for _, groupID := range uniquePositiveInt64s(req.GroupIDs) {
		var id int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_group_bindings
				(group_id, rule_set_id, enabled, created_by, updated_by)
			VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($4, 0))
			ON CONFLICT (group_id, rule_set_id)
			DO UPDATE SET enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
			RETURNING id`, groupID, req.RuleSetID, req.Enabled, actorID).Scan(&id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if _, err = bumpInstructionConfigTx(ctx, tx); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	all, err := r.ListGroupBindings(ctx)
	if err != nil {
		return nil, err
	}
	wanted := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	items := make([]InstructionGroupBinding, 0, len(ids))
	for _, item := range all {
		if _, ok := wanted[item.ID]; ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *InstructionRepository) DeleteGroupBinding(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_group_bindings WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	if _, err = bumpInstructionConfigTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *InstructionRepository) ListEvents(ctx context.Context, page, pageSize int, userID int64, model string) (*InstructionEventPage, error) {
	offset := (page - 1) * pageSize
	pattern := "%" + strings.TrimSpace(model) + "%"
	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM instruction_audit_events
		WHERE ($1 = 0 OR user_id = $1) AND ($2 = '%%' OR model ILIKE $2)`, userID, pattern).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.id, e.request_id, e.user_id, e.user_email_snapshot, e.api_key_id,
			e.group_id, e.group_name_snapshot, e.model, e.endpoint, e.stage,
			e.instructions_present, e.instructions_sha256, e.instructions_result,
			e.input1_present, e.input1_sha256, e.input1_result,
			e.decision, e.reason, e.rule_set_ids, e.config_version, e.latency_ms,
			COALESCE(o.status, 'rate_limited'), e.created_at
		FROM instruction_audit_events e
		LEFT JOIN instruction_audit_notification_outbox o ON o.event_id = e.id
		WHERE ($1 = 0 OR e.user_id = $1) AND ($2 = '%%' OR e.model ILIKE $2)
		ORDER BY e.created_at DESC, e.id DESC LIMIT $3 OFFSET $4`, userID, pattern, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]InstructionEvent, 0, pageSize)
	for rows.Next() {
		item, scanErr := scanInstructionEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &InstructionEventPage{
		Items: items, Total: total, Page: page, PageSize: pageSize,
		Pages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}

func scanInstructionEvent(scanner instructionScanner) (InstructionEvent, error) {
	var item InstructionEvent
	var userID, apiKeyID, groupID sql.NullInt64
	var ruleSetJSON []byte
	err := scanner.Scan(
		&item.ID, &item.RequestID, &userID, &item.UserEmailSnapshot, &apiKeyID,
		&groupID, &item.GroupNameSnapshot,
		&item.Model, &item.Endpoint, &item.Stage,
		&item.Instructions.Present, &item.Instructions.SHA256, &item.Instructions.Result,
		&item.Input1.Present, &item.Input1.SHA256, &item.Input1.Result,
		&item.Decision, &item.Reason, &ruleSetJSON, &item.ConfigVersion, &item.LatencyMS,
		&item.NotificationStatus, &item.CreatedAt,
	)
	if userID.Valid {
		item.UserID = &userID.Int64
	}
	if apiKeyID.Valid {
		item.APIKeyID = &apiKeyID.Int64
	}
	if groupID.Valid {
		item.GroupID = &groupID.Int64
	}
	if len(ruleSetJSON) > 0 {
		_ = json.Unmarshal(ruleSetJSON, &item.RuleSetIDs)
	}
	if item.RuleSetIDs == nil {
		item.RuleSetIDs = []int64{}
	}
	return item, err
}

func (r *InstructionRepository) GetEvent(ctx context.Context, id int64) (*InstructionEvent, error) {
	item, err := scanInstructionEvent(r.db.QueryRowContext(ctx, `
		SELECT e.id, e.request_id, e.user_id, e.user_email_snapshot, e.api_key_id,
			e.group_id, e.group_name_snapshot, e.model, e.endpoint, e.stage,
			e.instructions_present, e.instructions_sha256, e.instructions_result,
			e.input1_present, e.input1_sha256, e.input1_result,
			e.decision, e.reason, e.rule_set_ids, e.config_version, e.latency_ms,
			COALESCE(o.status, 'rate_limited'), e.created_at
		FROM instruction_audit_events e
		LEFT JOIN instruction_audit_notification_outbox o ON o.event_id = e.id
		WHERE e.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *InstructionRepository) RecordBlocked(ctx context.Context, req Request, decision *InstructionDecision) error {
	if decision == nil {
		return errors.New("instruction audit decision required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ruleSetIDs := decision.RuleSetIDs
	if ruleSetIDs == nil {
		ruleSetIDs = []int64{}
	}
	ruleSets, err := json.Marshal(ruleSetIDs)
	if err != nil {
		return fmt.Errorf("marshal instruction audit rule set ids: %w", err)
	}
	latencyMS := int(decision.Latency.Milliseconds())
	if latencyMS < 0 {
		latencyMS = 0
	}
	var eventID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_events
			(request_id, user_id, user_email_snapshot, api_key_id, group_id, group_name_snapshot,
			 model, endpoint, stage,
			 instructions_present, instructions_sha256, instructions_result,
			 input1_present, input1_sha256, input1_result, reason, rule_set_ids, config_version, latency_ms)
		VALUES ($1, NULLIF($2, 0), $3, NULLIF($4, 0), NULLIF($5, 0), $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id`,
		req.RequestID, req.UserID, req.UserEmail, req.APIKeyID, instructionGroupID(req.GroupID), req.GroupName,
		req.Model, req.Endpoint, req.Stage,
		decision.Instructions.Present, decision.Instructions.SHA256, decision.Instructions.Result,
		decision.Input1.Present, decision.Input1.SHA256, decision.Input1.Result,
		decision.Reason, ruleSets, decision.ConfigVersion, latencyMS).Scan(&eventID)
	if err != nil {
		return err
	}
	bucket := time.Now().UTC().Truncate(15 * time.Minute).Format(time.RFC3339)
	dedupRaw := fmt.Sprintf("%d\x00%d\x00%s\x00%s", req.UserID, instructionGroupID(req.GroupID), decision.Reason, bucket)
	dedupSum := sha256.Sum256([]byte(dedupRaw))
	_, err = tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_notification_outbox (event_id, dedup_key)
			VALUES ($1, $2) ON CONFLICT (dedup_key) DO NOTHING`, eventID, hex.EncodeToString(dedupSum[:]))
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

type instructionOutboxItem struct {
	ID               int64
	EventID          int64
	Attempts         int
	MaxAttempts      int
	SentRecipientIDs []int64
}

type instructionAdminRecipient struct {
	ID       int64
	Email    string
	Username string
}

func (r *InstructionRepository) ReclaimStaleOutbox(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_notification_outbox
		SET status = 'retry', claimed_at = NULL, available_at = NOW(), updated_at = NOW(), last_error = 'stale claim recovered'
		WHERE status = 'processing' AND claimed_at < NOW() - INTERVAL '5 minutes'`)
	return err
}

func (r *InstructionRepository) ClaimOutbox(ctx context.Context) (*instructionOutboxItem, error) {
	var item instructionOutboxItem
	var sentRecipientIDs pq.Int64Array
	err := r.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id FROM instruction_audit_notification_outbox
			WHERE status IN ('pending', 'retry') AND available_at <= NOW()
			ORDER BY available_at, id
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE instruction_audit_notification_outbox o
		SET status = 'processing', attempts = attempts + 1, claimed_at = NOW(), updated_at = NOW()
		FROM candidate c WHERE o.id = c.id
			RETURNING o.id, o.event_id, o.attempts, o.max_attempts, o.sent_recipient_ids`).Scan(
		&item.ID, &item.EventID, &item.Attempts, &item.MaxAttempts, &sentRecipientIDs)
	if err != nil {
		return nil, err
	}
	item.SentRecipientIDs = append([]int64(nil), sentRecipientIDs...)
	return &item, nil
}

func (r *InstructionRepository) ListAdminRecipients(ctx context.Context) ([]instructionAdminRecipient, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, email, COALESCE(username, '') FROM users
		WHERE role = 'admin' AND status = 'active' AND deleted_at IS NULL AND btrim(email) <> ''
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]instructionAdminRecipient, 0)
	for rows.Next() {
		var item instructionAdminRecipient
		if err := rows.Scan(&item.ID, &item.Email, &item.Username); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionRepository) MarkOutboxSent(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_notification_outbox
		SET status = 'sent', claimed_at = NULL, last_error = '', updated_at = NOW()
		WHERE id = $1`, id)
	return err
}

func (r *InstructionRepository) MarkOutboxRecipientSent(ctx context.Context, id, recipientID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE instruction_audit_notification_outbox
		SET sent_recipient_ids = CASE
				WHEN $2 = ANY(sent_recipient_ids) THEN sent_recipient_ids
				ELSE array_append(sent_recipient_ids, $2)
			END,
			claimed_at = NOW(), updated_at = NOW()
		WHERE id = $1`, id, recipientID)
	return err
}

func (r *InstructionRepository) MarkOutboxFailed(ctx context.Context, item instructionOutboxItem, sendErr error, delay time.Duration) error {
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
		UPDATE instruction_audit_notification_outbox
		SET status = $2, claimed_at = NULL, available_at = NOW() + ($3 * INTERVAL '1 second'),
			last_error = $4, updated_at = NOW()
		WHERE id = $1`, item.ID, status, int(delay.Seconds()), message)
	return err
}

func (r *InstructionRepository) OverviewCounts(ctx context.Context) (hashes, activeHashes, ruleSets, auditedGroups, effectiveGroups, pendingEmails int64, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM instruction_audit_hashes),
			(SELECT COUNT(*) FROM instruction_audit_hashes WHERE status = 'active' AND (valid_from IS NULL OR valid_from <= NOW()) AND (valid_until IS NULL OR valid_until > NOW())),
			(SELECT COUNT(*) FROM instruction_audit_rule_sets),
			(SELECT COUNT(DISTINCT b.group_id)
			 FROM instruction_audit_group_bindings b
			 WHERE b.enabled = TRUE),
			(SELECT COUNT(DISTINCT b.group_id)
			 FROM instruction_audit_group_bindings b
			 JOIN instruction_audit_rule_sets rs ON rs.id = b.rule_set_id AND rs.enabled = TRUE
			 JOIN instruction_audit_rule_set_hashes rsh ON rsh.rule_set_id = rs.id
			 JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
				AND h.status = 'active'
				AND (h.valid_from IS NULL OR h.valid_from <= NOW())
				AND (h.valid_until IS NULL OR h.valid_until > NOW())
			 WHERE b.enabled = TRUE),
			(SELECT COUNT(*) FROM instruction_audit_notification_outbox WHERE status IN ('pending', 'processing', 'retry'))`).Scan(
		&hashes, &activeHashes, &ruleSets, &auditedGroups, &effectiveGroups, &pendingEmails)
	return
}

func instructionGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
	}
	return *groupID
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
