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
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type InstructionRepository struct{ db *sql.DB }

var errInstructionAuditAllowedUserNotFound = errors.New("instruction audit allowed user not found")

func NewInstructionRepository(db *sql.DB) *InstructionRepository {
	return &InstructionRepository{db: db}
}

type instructionPolicyAccumulator struct {
	ruleSets         map[int64]struct{}
	hashes           map[[32]byte]instructionPolicyHash
	allowedUsers     map[int64]struct{}
	allowEmptyFields bool
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
			SELECT b.group_id, b.client_types, rs.id, rs.enabled, rs.allow_empty_fields,
				ARRAY(
					SELECT rsu.user_id
					FROM instruction_audit_rule_set_users rsu
					WHERE rsu.rule_set_id = rs.id
					ORDER BY rsu.user_id
				),
				h.digest, h.valid_from, h.valid_until
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
	defer func() { _ = rows.Close() }()

	accumulators := make(map[int64]*instructionPolicyAccumulator)
	clientAccumulators := make(map[instructionPolicyScope]*instructionPolicyAccumulator)
	auditedGroups := make(map[int64]struct{})
	auditedClientScopes := make(map[instructionPolicyScope]struct{})
	for rows.Next() {
		var groupID, ruleSetID int64
		var clientTypes pq.StringArray
		var ruleSetEnabled bool
		var allowEmptyFields bool
		var allowedUserIDs pq.Int64Array
		var digest sql.NullString
		var validFrom, validUntil sql.NullTime
		if err := rows.Scan(
			&groupID, &clientTypes, &ruleSetID, &ruleSetEnabled, &allowEmptyFields,
			&allowedUserIDs, &digest, &validFrom, &validUntil,
		); err != nil {
			return nil, err
		}
		normalizedClientTypes, normalizeErr := normalizeInstructionClientTypes([]string(clientTypes))
		if normalizeErr != nil || len(normalizedClientTypes) == 0 {
			return nil, fmt.Errorf("invalid instruction audit client scope in database")
		}
		targets := make([]*instructionPolicyAccumulator, 0, len(normalizedClientTypes))
		for _, clientType := range normalizedClientTypes {
			if clientType == InstructionClientAll {
				auditedGroups[groupID] = struct{}{}
				acc := accumulators[groupID]
				if acc == nil {
					acc = newInstructionPolicyAccumulator()
					accumulators[groupID] = acc
				}
				targets = append(targets, acc)
				continue
			}
			scope := instructionPolicyScope{GroupID: groupID, ClientType: clientType}
			auditedClientScopes[scope] = struct{}{}
			acc := clientAccumulators[scope]
			if acc == nil {
				acc = newInstructionPolicyAccumulator()
				clientAccumulators[scope] = acc
			}
			targets = append(targets, acc)
		}
		var policyHash *instructionPolicyHash
		if digest.Valid {
			decoded, decodeErr := hex.DecodeString(strings.TrimSpace(digest.String))
			if decodeErr != nil || len(decoded) != sha256.Size {
				return nil, fmt.Errorf("invalid instruction audit digest in database")
			}
			var value [32]byte
			copy(value[:], decoded)
			valueHash := instructionPolicyHash{Digest: value}
			if validFrom.Valid {
				valueHash.ValidFrom = validFrom.Time.UTC()
			}
			if validUntil.Valid {
				valueHash.ValidUntil = validUntil.Time.UTC()
			}
			policyHash = &valueHash
		}
		for _, acc := range targets {
			if ruleSetEnabled {
				acc.ruleSets[ruleSetID] = struct{}{}
				acc.allowEmptyFields = acc.allowEmptyFields || allowEmptyFields
				for _, userID := range allowedUserIDs {
					if userID > 0 {
						acc.allowedUsers[userID] = struct{}{}
					}
				}
			}
			if policyHash != nil {
				acc.hashes[policyHash.Digest] = *policyHash
			}
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
		policies[groupID] = buildInstructionPolicy(acc)
	}
	clientPolicies := make(map[instructionPolicyScope]instructionPolicy, len(clientAccumulators))
	for scope, acc := range clientAccumulators {
		clientPolicies[scope] = buildInstructionPolicy(acc)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &instructionSnapshot{
		Enabled: enabled, ConfigVersion: version, AuditedGroups: auditedGroups,
		Policies: policies, AuditedClientScopes: auditedClientScopes,
		ClientPolicies: clientPolicies, LoadedAt: time.Now().UTC(),
	}, nil
}

func newInstructionPolicyAccumulator() *instructionPolicyAccumulator {
	return &instructionPolicyAccumulator{
		ruleSets:     make(map[int64]struct{}),
		hashes:       make(map[[32]byte]instructionPolicyHash),
		allowedUsers: make(map[int64]struct{}),
	}
}

func buildInstructionPolicy(acc *instructionPolicyAccumulator) instructionPolicy {
	policy := instructionPolicy{}
	if acc == nil {
		return policy
	}
	policy.RuleSetIDs = make([]int64, 0, len(acc.ruleSets))
	policy.Hashes = make([]instructionPolicyHash, 0, len(acc.hashes))
	policy.AllowedUsers = make(map[int64]struct{}, len(acc.allowedUsers))
	policy.AllowEmptyFields = acc.allowEmptyFields
	for id := range acc.ruleSets {
		policy.RuleSetIDs = append(policy.RuleSetIDs, id)
	}
	for _, hash := range acc.hashes {
		policy.Hashes = append(policy.Hashes, hash)
	}
	for userID := range acc.allowedUsers {
		policy.AllowedUsers[userID] = struct{}{}
	}
	sort.Slice(policy.RuleSetIDs, func(i, j int) bool { return policy.RuleSetIDs[i] < policy.RuleSetIDs[j] })
	sort.Slice(policy.Hashes, func(i, j int) bool {
		return bytes.Compare(policy.Hashes[i].Digest[:], policy.Hashes[j].Digest[:]) < 0
	})
	return policy
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
			WHERE b.enabled = TRUE
				AND (
					rs.allow_empty_fields
					OR EXISTS (
						SELECT 1 FROM instruction_audit_rule_set_users rsu
						WHERE rsu.rule_set_id = rs.id
					)
					OR EXISTS (
						SELECT 1
						FROM instruction_audit_rule_set_hashes rsh
						JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
						WHERE rsh.rule_set_id = rs.id
							AND h.status = 'active'
							AND (h.valid_from IS NULL OR h.valid_from <= NOW())
							AND (h.valid_until IS NULL OR h.valid_until > NOW())
					)
				)`).Scan(&effectiveGroups)
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
	defer func() { _ = rows.Close() }()
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

func (r *InstructionRepository) DeleteHash(ctx context.Context, id int64) (version, references int64, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var lockedID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM instruction_audit_hashes WHERE id = $1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
		return 0, 0, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM instruction_audit_rule_set_hashes WHERE hash_id = $1`, id).Scan(&references); err != nil {
		return 0, 0, err
	}
	if references > 0 {
		return 0, references, nil
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM instruction_audit_hashes WHERE id = $1`, id); err != nil {
		return 0, 0, err
	}
	if version, err = bumpInstructionConfigTx(ctx, tx); err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return version, 0, nil
}

func (r *InstructionRepository) ListRuleSets(ctx context.Context) ([]InstructionRuleSet, error) {
	rows, err := r.db.QueryContext(ctx, `
			SELECT id, name, description, enabled, allow_empty_fields, version, created_at, updated_at
			FROM instruction_audit_rule_sets ORDER BY enabled DESC, name, id`)
	if err != nil {
		return nil, err
	}
	items := make([]InstructionRuleSet, 0)
	index := make(map[int64]int)
	for rows.Next() {
		var item InstructionRuleSet
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.Enabled, &item.AllowEmptyFields,
			&item.Version, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			_ = rows.Close()
			return nil, err
		}
		item.Hashes = []InstructionHashEntry{}
		item.AllowedUsers = []InstructionRuleSetUser{}
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
	if err := hashRows.Err(); err != nil {
		_ = hashRows.Close()
		return nil, err
	}
	if err := hashRows.Close(); err != nil {
		return nil, err
	}

	userRows, err := r.db.QueryContext(ctx, `
		SELECT rsu.rule_set_id, u.id, u.email, u.deleted_at IS NOT NULL
		FROM instruction_audit_rule_set_users rsu
		JOIN users u ON u.id = rsu.user_id
		ORDER BY rsu.rule_set_id, u.email, u.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = userRows.Close() }()
	for userRows.Next() {
		var ruleSetID int64
		var user InstructionRuleSetUser
		if err := userRows.Scan(&ruleSetID, &user.ID, &user.Email, &user.Deleted); err != nil {
			return nil, err
		}
		if position, ok := index[ruleSetID]; ok {
			items[position].AllowedUsers = append(items[position].AllowedUsers, user)
		}
	}
	return items, userRows.Err()
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
				INSERT INTO instruction_audit_rule_sets
					(name, description, enabled, allow_empty_fields, created_by, updated_by)
				VALUES ($1, $2, $3, $4, NULLIF($5, 0), NULLIF($5, 0)) RETURNING id`,
			req.Name, req.Description, req.Enabled, req.AllowEmptyFields, actorID).Scan(&savedID)
	} else {
		err = tx.QueryRowContext(ctx, `
				UPDATE instruction_audit_rule_sets
				SET name = $2, description = $3, enabled = $4, allow_empty_fields = $5,
					version = version + 1, updated_by = NULLIF($6, 0), updated_at = NOW()
				WHERE id = $1 RETURNING id`, id, req.Name, req.Description, req.Enabled,
			req.AllowEmptyFields, actorID).Scan(&savedID)
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
	if _, err = tx.ExecContext(ctx, `DELETE FROM instruction_audit_rule_set_users WHERE rule_set_id = $1`, savedID); err != nil {
		return nil, err
	}
	for _, userID := range uniquePositiveInt64s(req.AllowedUserIDs) {
		result, insertErr := tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_rule_set_users (rule_set_id, user_id, created_by)
			SELECT $1, u.id, NULLIF($3, 0)
			FROM users u
			WHERE u.id = $2`, savedID, userID, actorID)
		if insertErr != nil {
			return nil, insertErr
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return nil, affectedErr
		}
		if affected == 0 {
			return nil, errInstructionAuditAllowedUserNotFound
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

func (r *InstructionRepository) DeleteRuleSet(ctx context.Context, id int64) (version, references int64, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var lockedID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM instruction_audit_rule_sets WHERE id = $1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
		return 0, 0, err
	}
	if err = tx.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM instruction_audit_group_bindings WHERE rule_set_id = $1) +
			(SELECT COUNT(*) FROM instruction_audit_bindings WHERE rule_set_id = $1)`, id).Scan(&references); err != nil {
		return 0, 0, err
	}
	if references > 0 {
		return 0, references, nil
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM instruction_audit_rule_sets WHERE id = $1`, id); err != nil {
		return 0, 0, err
	}
	if version, err = bumpInstructionConfigTx(ctx, tx); err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return version, 0, nil
}

func (r *InstructionRepository) AddHashesToRuleSet(
	ctx context.Context,
	ruleSetID int64,
	hashes []CreateInstructionHashRequest,
	actorID int64,
) (*AddInstructionEventToRuleSetResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var lockedRuleSetID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM instruction_audit_rule_sets WHERE id = $1 FOR UPDATE`, ruleSetID).Scan(&lockedRuleSetID); err != nil {
		return nil, err
	}
	result := &AddInstructionEventToRuleSetResult{RuleSetID: ruleSetID, HashIDs: make([]int64, 0, len(hashes))}
	seenDigests := make(map[string]struct{}, len(hashes))
	for _, item := range hashes {
		if _, exists := seenDigests[item.Digest]; exists {
			continue
		}
		seenDigests[item.Digest] = struct{}{}
		var hashID int64
		var status string
		var validFrom, validUntil sql.NullTime
		err = tx.QueryRowContext(ctx, `
				SELECT id, status, valid_from, valid_until
				FROM instruction_audit_hashes WHERE digest = $1 FOR UPDATE`, item.Digest).
			Scan(&hashID, &status, &validFrom, &validUntil)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO instruction_audit_hashes
					(digest, name, note, observed_source, client_name, client_version, status, created_by)
				VALUES ($1, $2, $3, $4, $5, $6, 'active', NULLIF($7, 0))
				RETURNING id`, item.Digest, item.Name, item.Note, item.ObservedSource,
				item.ClientName, item.ClientVersion, actorID).Scan(&hashID)
			if err != nil {
				return nil, err
			}
			result.CreatedHashes++
		} else if err != nil {
			return nil, err
		} else if status != "active" || validFrom.Valid || validUntil.Valid {
			if _, err = tx.ExecContext(ctx, `
				UPDATE instruction_audit_hashes
				SET status = 'active', valid_from = NULL, valid_until = NULL, updated_at = NOW()
				WHERE id = $1`, hashID); err != nil {
				return nil, err
			}
			result.ActivatedHashes++
		}
		insertResult, insertErr := tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_rule_set_hashes (rule_set_id, hash_id, created_by)
			VALUES ($1, $2, NULLIF($3, 0))
			ON CONFLICT (rule_set_id, hash_id) DO NOTHING`, ruleSetID, hashID, actorID)
		if insertErr != nil {
			return nil, insertErr
		}
		if affected, _ := insertResult.RowsAffected(); affected > 0 {
			result.AttachedHashes++
		}
		result.HashIDs = append(result.HashIDs, hashID)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE instruction_audit_rule_sets
		SET version = version + 1, updated_by = NULLIF($2, 0), updated_at = NOW()
		WHERE id = $1`, ruleSetID, actorID); err != nil {
		return nil, err
	}
	if result.ConfigVersion, err = bumpInstructionConfigTx(ctx, tx); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *InstructionRepository) ListGroupBindings(ctx context.Context) ([]InstructionGroupBinding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.id, b.group_id, g.name, g.platform, g.status,
			b.rule_set_id, rs.name, b.client_types, b.enabled,
			(rs.enabled AND (
				rs.allow_empty_fields
				OR EXISTS (
					SELECT 1 FROM instruction_audit_rule_set_users rsu
					WHERE rsu.rule_set_id = rs.id
				)
				OR EXISTS (
					SELECT 1
					FROM instruction_audit_rule_set_hashes rsh
					JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
					WHERE rsh.rule_set_id = rs.id
						AND h.status = 'active'
						AND (h.valid_from IS NULL OR h.valid_from <= NOW())
						AND (h.valid_until IS NULL OR h.valid_until > NOW())
				)
			)) AS effective,
			b.created_at, b.updated_at
		FROM instruction_audit_group_bindings b
		JOIN instruction_audit_rule_sets rs ON rs.id = b.rule_set_id
		JOIN groups g ON g.id = b.group_id
		ORDER BY b.enabled DESC, g.name, b.group_id, rs.name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionGroupBinding, 0)
	for rows.Next() {
		var item InstructionGroupBinding
		var clientTypes pq.StringArray
		if err := rows.Scan(&item.ID, &item.GroupID, &item.GroupName, &item.Platform, &item.GroupStatus,
			&item.RuleSetID, &item.RuleSetName, &clientTypes, &item.Enabled, &item.Effective, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ClientTypes = append([]string(nil), clientTypes...)
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

func (r *InstructionRepository) SaveGroupBindings(ctx context.Context, req SaveInstructionGroupBindingsRequest, actorID int64) ([]InstructionGroupBinding, error) {
	if req.ClientTypes != nil {
		clientTypes, err := normalizeInstructionClientTypes(req.ClientTypes)
		if err != nil {
			return nil, err
		}
		req.ClientTypes = clientTypes
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var clientTypes any
	if req.ClientTypes != nil {
		clientTypes = pq.Array(req.ClientTypes)
	}
	ids := make([]int64, 0, len(req.GroupIDs))
	for _, groupID := range uniquePositiveInt64s(req.GroupIDs) {
		var id int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_group_bindings
				(group_id, rule_set_id, client_types, enabled, created_by, updated_by)
			VALUES ($1, $2, COALESCE($3::TEXT[], ARRAY['all']::TEXT[]), $4, NULLIF($5, 0), NULLIF($5, 0))
			ON CONFLICT (group_id, rule_set_id)
			DO UPDATE SET client_types = COALESCE($3::TEXT[], instruction_audit_group_bindings.client_types),
				enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
			RETURNING id`, groupID, req.RuleSetID, clientTypes, req.Enabled, actorID).Scan(&id)
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

const instructionEventFilterSQL = `
	WHERE ($1 = '' OR
		e.user_email_snapshot ILIKE $2 OR e.request_id ILIKE $2 OR e.model ILIKE $2 OR
		e.client_type ILIKE $2 OR e.client_user_agent ILIKE $2 OR
		COALESCE(e.user_id::TEXT, '') = $1 OR COALESCE(e.api_key_id::TEXT, '') = $1)
	  AND ($3::TIMESTAMPTZ IS NULL OR e.created_at >= $3)
	  AND ($4::TIMESTAMPTZ IS NULL OR e.created_at < $4)
	  AND (cardinality($5::BIGINT[]) = 0 OR e.group_id = ANY($5::BIGINT[]))
	  AND (cardinality($6::TEXT[]) = 0 OR e.reason = ANY($6::TEXT[]))
	  AND (cardinality($7::TEXT[]) = 0 OR e.instructions_result = ANY($7::TEXT[]))
	  AND (cardinality($8::TEXT[]) = 0 OR e.input1_result = ANY($8::TEXT[]))
	  AND (cardinality($9::TEXT[]) = 0 OR COALESCE(un.status, 'no_recipient') = ANY($9::TEXT[]))
	  AND (cardinality($10::TEXT[]) = 0 OR COALESCE(op.status, 'no_recipient') = ANY($10::TEXT[]))
	  AND ($11 = 0 OR e.user_id = $11)
	  AND ($12 = '%%' OR e.model ILIKE $12)
	  AND (cardinality($13::TEXT[]) = 0 OR e.client_type = ANY($13::TEXT[]))
	  AND ($14 = 0 OR e.id = $14)`

func instructionEventFilterArgs(filter InstructionEventFilter) []any {
	filter = canonicalInstructionEventFilter(filter)
	query := strings.TrimSpace(filter.Query)
	return []any{
		query, "%" + query + "%", filter.From, filter.To, pq.Array(filter.GroupIDs), pq.Array(filter.Reasons),
		pq.Array(filter.InstructionResults), pq.Array(filter.Input1Results),
		pq.Array(filter.UserNotifications), pq.Array(filter.OpsNotifications), filter.UserID,
		"%" + strings.TrimSpace(filter.Model) + "%", pq.Array(filter.ClientTypes), filter.EventID,
	}
}

func (r *InstructionRepository) ListEvents(ctx context.Context, page, pageSize int, filter InstructionEventFilter) (*InstructionEventPage, error) {
	offset := (page - 1) * pageSize
	args := instructionEventFilterArgs(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM instruction_audit_events e
		LEFT JOIN security_notification_outbox un
			ON un.source_type = 'instruction_audit' AND un.source_id = e.id AND un.audience = 'user'
		LEFT JOIN security_notification_outbox op
			ON op.source_type = 'instruction_audit' AND op.source_id = e.id AND op.audience = 'ops'
		`+instructionEventFilterSQL, args...).Scan(&total); err != nil {
		return nil, err
	}
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.id, e.request_id, e.user_id, e.user_email_snapshot, e.api_key_id,
			e.group_id, e.group_name_snapshot, e.client_type, e.client_user_agent,
			e.model, e.endpoint, e.stage,
			e.instructions_present, e.instructions_sha256, e.instructions_result,
			e.input1_present, e.input1_sha256, e.input1_result,
			e.decision, e.reason, e.rule_set_ids, e.config_version, e.latency_ms,
			e.evidence_status, e.evidence_expires_at,
			COALESCE(un.status, 'no_recipient'), COALESCE(op.status, 'no_recipient'), e.created_at
		FROM instruction_audit_events e
		LEFT JOIN security_notification_outbox un
			ON un.source_type = 'instruction_audit' AND un.source_id = e.id AND un.audience = 'user'
		LEFT JOIN security_notification_outbox op
			ON op.source_type = 'instruction_audit' AND op.source_id = e.id AND op.audience = 'ops'
		`+instructionEventFilterSQL+`
		ORDER BY e.created_at DESC, e.id DESC LIMIT $15 OFFSET $16`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

func (r *InstructionRepository) DeleteEventsByIDs(ctx context.Context, ids []int64) (*InstructionDeleteResult, error) {
	ids = uniquePositiveInt64s(ids)
	if len(ids) == 0 {
		return &InstructionDeleteResult{}, nil
	}
	if len(ids) > 500 {
		return nil, errors.New("instruction audit delete batch exceeds 500 events")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `
		DELETE FROM security_notification_outbox
		WHERE source_type = 'instruction_audit' AND source_id = ANY($1)`, pq.Array(ids)); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `DELETE FROM instruction_audit_events WHERE id = ANY($1) RETURNING id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	deletedIDs, err := scanInstructionEventIDs(rows)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &InstructionDeleteResult{DeletedEvents: int64(len(deletedIDs))}, nil
}

func (r *InstructionRepository) PreviewDeleteEvents(ctx context.Context, filter InstructionEventFilter) (*InstructionDeletePreview, error) {
	if err := validateInstructionDeleteFilter(filter); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	args := instructionEventFilterArgs(filter)
	var count, maxID int64
	if err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(MAX(e.id), 0)
		FROM instruction_audit_events e
		LEFT JOIN security_notification_outbox un
			ON un.source_type = 'instruction_audit' AND un.source_id = e.id AND un.audience = 'user'
		LEFT JOIN security_notification_outbox op
			ON op.source_type = 'instruction_audit' AND op.source_id = e.id AND op.audience = 'ops'
		`+instructionEventFilterSQL, args...).Scan(&count, &maxID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	canonical := canonicalInstructionEventFilter(filter)
	return &InstructionDeletePreview{
		MatchedCount: count, FilterSummary: canonical, SnapshotMaxID: maxID,
		FilterHash: instructionEventFilterHash(canonical, maxID),
	}, nil
}

func (r *InstructionRepository) DeleteEventsByFilter(
	ctx context.Context,
	filter InstructionEventFilter,
	snapshotMaxID int64,
	batchSize int,
) (*InstructionDeleteResult, error) {
	if err := validateInstructionDeleteFilter(filter); err != nil {
		return nil, err
	}
	if snapshotMaxID <= 0 {
		return &InstructionDeleteResult{}, nil
	}
	if batchSize < 1 || batchSize > 1000 {
		batchSize = 200
	}
	total := &InstructionDeleteResult{}
	for {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		args := instructionEventFilterArgs(filter)
		args = append(args, snapshotMaxID, batchSize)
		rows, err := tx.QueryContext(ctx, `
			SELECT e.id
			FROM instruction_audit_events e
			LEFT JOIN security_notification_outbox un
				ON un.source_type = 'instruction_audit' AND un.source_id = e.id AND un.audience = 'user'
			LEFT JOIN security_notification_outbox op
				ON op.source_type = 'instruction_audit' AND op.source_id = e.id AND op.audience = 'ops'
			`+instructionEventFilterSQL+`
			AND e.id <= $15
			ORDER BY e.id LIMIT $16 FOR UPDATE OF e SKIP LOCKED`, args...)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		ids, err := scanInstructionEventIDs(rows)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if len(ids) > 0 {
			if _, err = tx.ExecContext(ctx, `
				DELETE FROM security_notification_outbox
				WHERE source_type = 'instruction_audit' AND source_id = ANY($1)`, pq.Array(ids)); err != nil {
				_ = tx.Rollback()
				return nil, err
			}
			result, deleteErr := tx.ExecContext(ctx, `DELETE FROM instruction_audit_events WHERE id = ANY($1)`, pq.Array(ids))
			if deleteErr != nil {
				_ = tx.Rollback()
				return nil, deleteErr
			}
			deleted, _ := result.RowsAffected()
			total.DeletedEvents += deleted
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		if len(ids) < batchSize {
			break
		}
	}
	return total, nil
}

func validateInstructionDeleteFilter(filter InstructionEventFilter) error {
	if filter.From == nil || filter.To == nil || !filter.From.Before(*filter.To) {
		return errors.New("instruction audit filter delete requires a valid explicit time range")
	}
	return nil
}

func canonicalInstructionEventFilter(filter InstructionEventFilter) InstructionEventFilter {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Model = strings.TrimSpace(filter.Model)
	filter.GroupIDs = uniquePositiveInt64s(filter.GroupIDs)
	sort.Slice(filter.GroupIDs, func(i, j int) bool { return filter.GroupIDs[i] < filter.GroupIDs[j] })
	filter.ClientTypes = canonicalInstructionStrings(filter.ClientTypes)
	filter.Reasons = canonicalInstructionStrings(filter.Reasons)
	filter.InstructionResults = canonicalInstructionStrings(filter.InstructionResults)
	filter.Input1Results = canonicalInstructionStrings(filter.Input1Results)
	filter.UserNotifications = canonicalInstructionStrings(filter.UserNotifications)
	filter.OpsNotifications = canonicalInstructionStrings(filter.OpsNotifications)
	if filter.From != nil {
		value := filter.From.UTC()
		filter.From = &value
	}
	if filter.To != nil {
		value := filter.To.UTC()
		filter.To = &value
	}
	return filter
}

func canonicalInstructionStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func instructionEventFilterHash(filter InstructionEventFilter, snapshotMaxID int64) string {
	payload := struct {
		Filter        InstructionEventFilter `json:"filter"`
		SnapshotMaxID int64                  `json:"snapshot_max_id"`
	}{canonicalInstructionEventFilter(filter), snapshotMaxID}
	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func scanInstructionEventIDs(rows *sql.Rows) ([]int64, error) {
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanInstructionEvent(scanner instructionScanner) (InstructionEvent, error) {
	var item InstructionEvent
	var userID, apiKeyID, groupID sql.NullInt64
	var evidenceExpiresAt sql.NullTime
	var ruleSetJSON []byte
	err := scanner.Scan(
		&item.ID, &item.RequestID, &userID, &item.UserEmailSnapshot, &apiKeyID,
		&groupID, &item.GroupNameSnapshot, &item.ClientType, &item.ClientUserAgent,
		&item.Model, &item.Endpoint, &item.Stage,
		&item.Instructions.Present, &item.Instructions.SHA256, &item.Instructions.Result,
		&item.Input1.Present, &item.Input1.SHA256, &item.Input1.Result,
		&item.Decision, &item.Reason, &ruleSetJSON, &item.ConfigVersion, &item.LatencyMS,
		&item.EvidenceStatus, &evidenceExpiresAt,
		&item.UserNotificationStatus, &item.OpsNotificationStatus, &item.CreatedAt,
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
	if evidenceExpiresAt.Valid {
		item.EvidenceExpiresAt = &evidenceExpiresAt.Time
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
			e.group_id, e.group_name_snapshot, e.client_type, e.client_user_agent,
			e.model, e.endpoint, e.stage,
			e.instructions_present, e.instructions_sha256, e.instructions_result,
			e.input1_present, e.input1_sha256, e.input1_result,
			e.decision, e.reason, e.rule_set_ids, e.config_version, e.latency_ms,
			e.evidence_status, e.evidence_expires_at,
			COALESCE(un.status, 'no_recipient'), COALESCE(op.status, 'no_recipient'), e.created_at
		FROM instruction_audit_events e
		LEFT JOIN security_notification_outbox un
			ON un.source_type = 'instruction_audit' AND un.source_id = e.id AND un.audience = 'user'
		LEFT JOIN security_notification_outbox op
			ON op.source_type = 'instruction_audit' AND op.source_id = e.id AND op.audience = 'ops'
		WHERE e.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *InstructionRepository) RecordBlocked(
	ctx context.Context,
	req Request,
	decision *InstructionDecision,
	evidenceStatus string,
	evidenceExpiresAt *time.Time,
	evidence []InstructionEvidence,
) (int64, error) {
	if decision == nil {
		return 0, errors.New("instruction audit decision required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	ruleSetIDs := decision.RuleSetIDs
	if ruleSetIDs == nil {
		ruleSetIDs = []int64{}
	}
	ruleSets, err := json.Marshal(ruleSetIDs)
	if err != nil {
		return 0, fmt.Errorf("marshal instruction audit rule set ids: %w", err)
	}
	latencyMS := int(decision.Latency.Milliseconds())
	if latencyMS < 0 {
		latencyMS = 0
	}
	clientType := strings.ToLower(strings.TrimSpace(req.InstructionClientType))
	if !validInstructionDetectedClientType(clientType) {
		clientType = ClassifyInstructionClient(req.UserAgent, req.TrustedInternalClient)
	}
	clientUserAgent := instructionUserAgentSnapshot(req.UserAgent)
	var eventID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO instruction_audit_events
			(request_id, user_id, user_email_snapshot, api_key_id, group_id, group_name_snapshot,
			 client_type, client_user_agent, model, endpoint, stage,
			 instructions_present, instructions_sha256, instructions_result,
			 input1_present, input1_sha256, input1_result, reason, rule_set_ids, config_version, latency_ms,
			 evidence_status, evidence_expires_at)
		VALUES ($1, NULLIF($2, 0), $3, NULLIF($4, 0), NULLIF($5, 0), $6, $7, LEFT($8, 512), $9, $10, $11,
			$12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		RETURNING id`,
		req.RequestID, req.UserID, req.UserEmail, req.APIKeyID, instructionGroupID(req.GroupID), req.GroupName,
		clientType, clientUserAgent, req.Model, req.Endpoint, req.Stage,
		decision.Instructions.Present, decision.Instructions.SHA256, decision.Instructions.Result,
		decision.Input1.Present, decision.Input1.SHA256, decision.Input1.Result,
		decision.Reason, ruleSets, decision.ConfigVersion, latencyMS,
		evidenceStatus, evidenceExpiresAt).Scan(&eventID)
	if err != nil {
		return 0, err
	}
	for _, item := range evidence {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO instruction_audit_evidence
				(event_id, source, digest, ciphertext, key_version, plaintext_bytes, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			eventID, item.Source, item.Digest, item.Ciphertext, item.KeyVersion,
			item.PlaintextBytes, item.ExpiresAt); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return eventID, nil
}

func (r *InstructionRepository) GetEvidenceRetentionDays(ctx context.Context) (int, error) {
	var raw string
	if err := r.db.QueryRowContext(ctx, `
		SELECT value FROM settings WHERE key = $1`, SettingKeyInstructionEvidenceRetentionDays).Scan(&raw); err != nil {
		return 0, err
	}
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid instruction evidence retention setting: %w", err)
	}
	return days, nil
}

func (r *InstructionRepository) SetEvidenceRetentionDays(ctx context.Context, days int) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		SettingKeyInstructionEvidenceRetentionDays, strconv.Itoa(days))
	return err
}

func (r *InstructionRepository) ListEvidence(ctx context.Context, eventID int64) ([]InstructionEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT source, digest, ciphertext, key_version, plaintext_bytes, expires_at
		FROM instruction_audit_evidence WHERE event_id = $1 ORDER BY source`, eventID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionEvidence, 0, 2)
	for rows.Next() {
		var item InstructionEvidence
		if err := rows.Scan(&item.Source, &item.Digest, &item.Ciphertext, &item.KeyVersion, &item.PlaintextBytes, &item.ExpiresAt); err != nil {
			return nil, err
		}
		item.Digest = strings.TrimSpace(item.Digest)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionRepository) RecordEvidenceAccess(ctx context.Context, eventID int64, access InstructionEvidenceAccess) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO instruction_audit_evidence_access_logs
			(event_id, actor_id, action, source, request_id, client_ip, user_agent, succeeded, error_code)
		VALUES ($1, NULLIF($2, 0), $3, LEFT($4, 48), LEFT($5, 128), LEFT($6, 64), LEFT($7, 512), $8, LEFT($9, 64))`,
		eventID, access.ActorID, access.Action, access.Source, access.RequestID,
		access.ClientIP, access.UserAgent, access.Succeeded, access.ErrorCode)
	return err
}

func (r *InstructionRepository) CountEvidenceAccesses(ctx context.Context, eventID int64) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM instruction_audit_evidence_access_logs WHERE event_id = $1`, eventID).Scan(&count)
	return count, err
}

func (r *InstructionRepository) HasSuccessfulEvidenceReveal(ctx context.Context, eventID, actorID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM instruction_audit_evidence_access_logs
			WHERE event_id = $1 AND actor_id = $2 AND action = 'reveal' AND succeeded = TRUE
		)`, eventID, actorID).Scan(&exists)
	return exists, err
}

func (r *InstructionRepository) ExpireEvidence(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		WITH expired AS (
			DELETE FROM instruction_audit_evidence
			WHERE expires_at <= NOW()
			RETURNING event_id
		), affected AS (
			SELECT DISTINCT event_id FROM expired
		)
		UPDATE instruction_audit_events e
		SET evidence_status = 'expired'
		FROM affected a
		WHERE e.id = a.event_id
		  AND NOT EXISTS (SELECT 1 FROM instruction_audit_evidence x WHERE x.event_id = e.id)`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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
			 WHERE b.enabled = TRUE
				AND (
					rs.allow_empty_fields
					OR EXISTS (
						SELECT 1 FROM instruction_audit_rule_set_users rsu
						WHERE rsu.rule_set_id = rs.id
					)
					OR EXISTS (
						SELECT 1
						FROM instruction_audit_rule_set_hashes rsh
						JOIN instruction_audit_hashes h ON h.id = rsh.hash_id
						WHERE rsh.rule_set_id = rs.id
							AND h.status = 'active'
							AND (h.valid_from IS NULL OR h.valid_from <= NOW())
							AND (h.valid_until IS NULL OR h.valid_until > NOW())
					)
				)),
			(SELECT COUNT(*) FROM security_notification_outbox
			 WHERE source_type = 'instruction_audit' AND status IN ('pending', 'processing', 'retry'))`).Scan(
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
