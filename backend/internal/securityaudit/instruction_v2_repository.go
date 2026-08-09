package securityaudit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type InstructionV2Repository struct {
	db *sql.DB
}

func NewInstructionV2Repository(db *sql.DB) *InstructionV2Repository {
	return &InstructionV2Repository{db: db}
}

func (r *InstructionV2Repository) LoadSnapshot(ctx context.Context, decryptor service.SecretEncryptor) (*instructionV2Snapshot, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("instruction audit v2 repository unavailable")
	}
	config, err := r.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	profiles, err := r.ListClientProfiles(ctx)
	if err != nil {
		return nil, err
	}
	compiledProfiles, profilesByKey, err := normalizeInstructionV2ClientProfiles(profiles)
	if err != nil {
		return nil, err
	}
	snapshot := &instructionV2Snapshot{
		Config: config, Profiles: compiledProfiles, ProfilesByKey: profilesByKey,
		ScopesByGroup: make(map[int64][]instructionV2ScopeRuntime),
		Hashes:        make(map[string]instructionV2HashRuntime), AllowedUsers: make(map[int64]struct{}),
		LoadedAt: time.Now().UTC(),
	}
	if config.RiskControlEnabled {
		snapshot.Config.EffectiveMode = config.Mode
	} else {
		snapshot.Config.EffectiveMode = InstructionV2ModeOff
	}
	snapshot.PromptVersion = instructionV2PromptVersion(config)
	snapshot.GlobalSemaphore = make(chan struct{}, config.AIGlobalConcurrency)

	if err := r.loadRuntimeScopes(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := r.loadRuntimeHashes(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := r.loadRuntimeAllowlist(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := r.loadRuntimeAINodes(ctx, snapshot, decryptor); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *InstructionV2Repository) getConfig(ctx context.Context) (InstructionV2Config, error) {
	var config InstructionV2Config
	var riskControl string
	err := r.db.QueryRowContext(ctx, `
		SELECT c.mode, c.review_criteria, c.confidence_threshold, c.ai_input_max_chars,
		       c.ai_global_concurrency, c.ai_queue_wait_ms, c.ai_total_timeout_ms,
		       c.ai_cache_ttl_seconds, c.event_retention_days, c.evidence_retention_days,
		       c.candidate_retention_days, c.raw_full_max_bytes, c.config_version,
		       c.updated_by, c.updated_at,
		       COALESCE((SELECT value FROM settings WHERE key = $1), 'false')
		FROM instruction_audit_v2_config c WHERE c.id = 1`, SettingKeyRiskControl).Scan(
		&config.Mode, &config.ReviewCriteria, &config.ConfidenceThreshold, &config.AIInputMaxChars,
		&config.AIGlobalConcurrency, &config.AIQueueWaitMS, &config.AITotalTimeoutMS,
		&config.AICacheTTLSeconds, &config.EventRetentionDays, &config.EvidenceRetentionDays,
		&config.CandidateRetentionDays, &config.RawFullMaxBytes, &config.ConfigVersion,
		&config.UpdatedBy, &config.UpdatedAt, &riskControl,
	)
	if err != nil {
		return InstructionV2Config{}, err
	}
	config.RiskControlEnabled, _ = strconv.ParseBool(strings.TrimSpace(riskControl))
	config.EffectiveMode = InstructionV2ModeOff
	if config.RiskControlEnabled {
		config.EffectiveMode = config.Mode
	}
	return config, nil
}

func (r *InstructionV2Repository) GetConfig(ctx context.Context) (InstructionV2Config, error) {
	config, err := r.getConfig(ctx)
	if err != nil {
		return InstructionV2Config{}, err
	}
	err = r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM instruction_audit_v2_scopes s
			 JOIN groups g ON g.id = s.group_id AND g.deleted_at IS NULL AND g.status = 'active'
			 LEFT JOIN instruction_audit_v2_client_profiles p ON p.id = s.client_profile_id
			 WHERE s.enabled AND (s.client_profile_id IS NULL OR p.enabled)),
			(SELECT COUNT(DISTINCT h.id) FROM instruction_audit_v2_hashes h
			 JOIN instruction_audit_v2_hash_scopes hs ON hs.hash_id = h.id
			 WHERE h.status = 'active' AND hs.status = 'active'),
			(SELECT COUNT(*) FROM instruction_audit_v2_ai_nodes WHERE enabled)
	`).Scan(&config.ActiveScopeCount, &config.ActiveHashCount, &config.EnabledAINodeCount)
	return config, err
}

func (r *InstructionV2Repository) UpdateConfig(ctx context.Context, request UpdateInstructionV2ConfigRequest, actorID int64) (InstructionV2Config, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2Config{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var nextVersion int64
	err = tx.QueryRowContext(ctx, `
		UPDATE instruction_audit_v2_config
		SET mode = $2, review_criteria = $3, confidence_threshold = $4,
		    ai_input_max_chars = $5, ai_global_concurrency = $6, ai_queue_wait_ms = $7,
		    ai_total_timeout_ms = $8, ai_cache_ttl_seconds = $9,
		    event_retention_days = $10, evidence_retention_days = $11,
		    candidate_retention_days = $12, raw_full_max_bytes = $13,
		    config_version = config_version + 1, updated_by = NULLIF($14, 0), updated_at = NOW()
		WHERE id = 1 AND config_version = $1
		RETURNING config_version`,
		request.ExpectedConfigVersion, request.Mode, request.ReviewCriteria, request.ConfidenceThreshold,
		request.AIInputMaxChars, request.AIGlobalConcurrency, request.AIQueueWaitMS,
		request.AITotalTimeoutMS, request.AICacheTTLSeconds, request.EventRetentionDays,
		request.EvidenceRetentionDays, request.CandidateRetentionDays, request.RawFullMaxBytes, actorID,
	).Scan(&nextVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return InstructionV2Config{}, errInstructionV2ConfigConflict
	}
	if err != nil {
		return InstructionV2Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2Config{}, err
	}
	return r.GetConfig(ctx)
}

func (r *InstructionV2Repository) loadRuntimeScopes(ctx context.Context, snapshot *instructionV2Snapshot) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.group_id, s.client_profile_id, COALESCE(p.profile_key, '')
		FROM instruction_audit_v2_scopes s
		JOIN groups g ON g.id = s.group_id AND g.deleted_at IS NULL AND g.status = 'active'
		LEFT JOIN instruction_audit_v2_client_profiles p ON p.id = s.client_profile_id
		WHERE s.enabled AND (s.client_profile_id IS NULL OR p.enabled)
		ORDER BY s.group_id, CASE WHEN s.client_profile_id IS NULL THEN 1 ELSE 0 END, s.id`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var scope instructionV2ScopeRuntime
		var profileID sql.NullInt64
		if err := rows.Scan(&scope.ID, &scope.GroupID, &profileID, &scope.ClientProfileKey); err != nil {
			return err
		}
		if profileID.Valid {
			value := profileID.Int64
			scope.ClientProfileID = &value
		}
		snapshot.ScopesByGroup[scope.GroupID] = append(snapshot.ScopesByGroup[scope.GroupID], scope)
	}
	return rows.Err()
}

func (r *InstructionV2Repository) loadRuntimeHashes(ctx context.Context, snapshot *instructionV2Snapshot) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT h.id, h.sha256, hs.scope_id
		FROM instruction_audit_v2_hashes h
		JOIN instruction_audit_v2_hash_scopes hs ON hs.hash_id = h.id AND hs.status = 'active'
		JOIN instruction_audit_v2_scopes s ON s.id = hs.scope_id AND s.enabled
		WHERE h.status = 'active'
		ORDER BY h.id, hs.scope_id`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var hashID, scopeID int64
		var digest string
		if err := rows.Scan(&hashID, &digest, &scopeID); err != nil {
			return err
		}
		digest = strings.TrimSpace(digest)
		hash := snapshot.Hashes[digest]
		if hash.ScopeIDs == nil {
			hash = instructionV2HashRuntime{ID: hashID, SHA256: digest, ScopeIDs: make(map[int64]struct{})}
		}
		hash.ScopeIDs[scopeID] = struct{}{}
		snapshot.Hashes[digest] = hash
	}
	return rows.Err()
}

func (r *InstructionV2Repository) loadRuntimeAllowlist(ctx context.Context, snapshot *instructionV2Snapshot) error {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id FROM instruction_audit_v2_user_allowlist WHERE enabled`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return err
		}
		snapshot.AllowedUsers[userID] = struct{}{}
	}
	return rows.Err()
}

func (r *InstructionV2Repository) loadRuntimeAINodes(ctx context.Context, snapshot *instructionV2Snapshot, decryptor service.SecretEncryptor) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, base_url, model, api_key_ciphertext, priority, enabled,
		       timeout_ms, max_concurrency, created_by, updated_by, created_at, updated_at
		FROM instruction_audit_v2_ai_nodes WHERE enabled ORDER BY priority, id`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var node instructionV2AINodeRuntime
		var ciphertext string
		if err := rows.Scan(
			&node.ID, &node.Name, &node.BaseURL, &node.Model, &ciphertext, &node.Priority,
			&node.Enabled, &node.TimeoutMS, &node.MaxConcurrency, &node.CreatedBy, &node.UpdatedBy,
			&node.CreatedAt, &node.UpdatedAt,
		); err != nil {
			return err
		}
		if strings.TrimSpace(ciphertext) == "" || decryptor == nil {
			continue
		}
		apiKey, decryptErr := decryptor.Decrypt(ciphertext)
		if decryptErr != nil || strings.TrimSpace(apiKey) == "" {
			continue
		}
		node.APIKey = apiKey
		node.HasAPIKey = true
		node.APIKeyStatus = "configured"
		node.semaphore = make(chan struct{}, node.MaxConcurrency)
		snapshot.AINodes = append(snapshot.AINodes, &node)
	}
	return rows.Err()
}

func instructionV2PromptVersion(config InstructionV2Config) string {
	material := strings.Join([]string{
		InstructionV2PromptWrapperVersion,
		config.ReviewCriteria,
		strconv.FormatFloat(config.ConfidenceThreshold, 'f', -1, 64),
		strconv.FormatInt(config.ConfigVersion, 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return InstructionV2PromptWrapperVersion + "-" + hex.EncodeToString(digest[:8])
}

func (r *InstructionV2Repository) bumpConfigVersion(ctx context.Context, tx *sql.Tx, actorID int64) (int64, error) {
	var version int64
	err := tx.QueryRowContext(ctx, `
		UPDATE instruction_audit_v2_config
		SET config_version = config_version + 1, updated_by = NULLIF($1, 0), updated_at = NOW()
		WHERE id = 1 RETURNING config_version`, actorID).Scan(&version)
	return version, err
}

func (r *InstructionV2Repository) ListAINodes(ctx context.Context) ([]InstructionV2AINode, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, base_url, model, priority, enabled, timeout_ms, max_concurrency,
		       api_key_ciphertext <> '', created_by, updated_by, created_at, updated_at
		FROM instruction_audit_v2_ai_nodes ORDER BY priority, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2AINode, 0)
	for rows.Next() {
		var item InstructionV2AINode
		if err := rows.Scan(
			&item.ID, &item.Name, &item.BaseURL, &item.Model, &item.Priority, &item.Enabled,
			&item.TimeoutMS, &item.MaxConcurrency, &item.HasAPIKey, &item.CreatedBy, &item.UpdatedBy,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if item.HasAPIKey {
			item.APIKeyStatus = "configured"
		} else {
			item.APIKeyStatus = "missing"
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) SaveAINode(ctx context.Context, id int64, request SaveInstructionV2AINodeRequest, ciphertext string, actorID int64) (InstructionV2AINode, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2AINode{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var item InstructionV2AINode
	if id == 0 {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_v2_ai_nodes
				(name, base_url, model, api_key_ciphertext, priority, enabled, timeout_ms,
				 max_concurrency, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, 0), NULLIF($9, 0))
			RETURNING id, created_at, updated_at`,
			request.Name, request.BaseURL, request.Model, ciphertext, request.Priority, request.Enabled,
			request.TimeoutMS, request.MaxConcurrency, actorID,
		).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	} else {
		query := `
			UPDATE instruction_audit_v2_ai_nodes
			SET name = $2, base_url = $3, model = $4, priority = $5, enabled = $6,
			    timeout_ms = $7, max_concurrency = $8, updated_by = NULLIF($9, 0), updated_at = NOW()`
		args := []any{id, request.Name, request.BaseURL, request.Model, request.Priority, request.Enabled, request.TimeoutMS, request.MaxConcurrency, actorID}
		if request.ClearAPIKey || ciphertext != "" {
			query += ", api_key_ciphertext = $10"
			args = append(args, ciphertext)
		}
		query += " WHERE id = $1 RETURNING id, created_at, updated_at"
		err = tx.QueryRowContext(ctx, query, args...).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	}
	if err != nil {
		return InstructionV2AINode{}, 0, err
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return InstructionV2AINode{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2AINode{}, 0, err
	}
	item.Name, item.BaseURL, item.Model = request.Name, request.BaseURL, request.Model
	item.Priority, item.Enabled, item.TimeoutMS, item.MaxConcurrency = request.Priority, request.Enabled, request.TimeoutMS, request.MaxConcurrency
	item.HasAPIKey = ciphertext != "" || (!request.ClearAPIKey && id > 0)
	if item.HasAPIKey {
		item.APIKeyStatus = "configured"
	} else {
		item.APIKeyStatus = "missing"
	}
	return item, version, nil
}

func (r *InstructionV2Repository) DeleteAINode(ctx context.Context, id, actorID int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_ai_nodes WHERE id = $1`, id)
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

func (r *InstructionV2Repository) ListClientProfiles(ctx context.Context) ([]InstructionV2ClientProfile, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, profile_key, name, description, matchers, priority, enabled,
		       built_in, immutable_internal, created_at, updated_at
		FROM instruction_audit_v2_client_profiles ORDER BY priority, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]InstructionV2ClientProfile, 0)
	for rows.Next() {
		var item InstructionV2ClientProfile
		var matchers []byte
		if err := rows.Scan(
			&item.ID, &item.ProfileKey, &item.Name, &item.Description, &matchers, &item.Priority,
			&item.Enabled, &item.BuiltIn, &item.ImmutableInternal, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(matchers, &item.Matchers); err != nil {
			return nil, fmt.Errorf("decode instruction audit client profile %d: %w", item.ID, err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InstructionV2Repository) SaveClientProfile(ctx context.Context, id int64, request SaveInstructionV2ClientProfileRequest, actorID int64) (InstructionV2ClientProfile, int64, error) {
	matchers, err := json.Marshal(request.Matchers)
	if err != nil {
		return InstructionV2ClientProfile{}, 0, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InstructionV2ClientProfile{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var item InstructionV2ClientProfile
	if id == 0 {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO instruction_audit_v2_client_profiles
				(profile_key, name, description, matchers, priority, enabled, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, 0), NULLIF($7, 0))
			RETURNING id, created_at, updated_at`,
			request.ProfileKey, request.Name, request.Description, matchers, request.Priority, request.Enabled, actorID,
		).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	} else {
		var builtIn, immutable bool
		var existingKey string
		if err = tx.QueryRowContext(ctx, `
			SELECT profile_key, built_in, immutable_internal
			FROM instruction_audit_v2_client_profiles WHERE id = $1 FOR UPDATE`, id).Scan(&existingKey, &builtIn, &immutable); err != nil {
			return InstructionV2ClientProfile{}, 0, err
		}
		if immutable {
			return InstructionV2ClientProfile{}, 0, errInstructionV2ImmutableProfile
		}
		if builtIn {
			request.ProfileKey = existingKey
		}
		err = tx.QueryRowContext(ctx, `
			UPDATE instruction_audit_v2_client_profiles
			SET profile_key = $2, name = $3, description = $4, matchers = $5,
			    priority = $6, enabled = $7, updated_by = NULLIF($8, 0), updated_at = NOW()
			WHERE id = $1 RETURNING id, built_in, immutable_internal, created_at, updated_at`,
			id, request.ProfileKey, request.Name, request.Description, matchers,
			request.Priority, request.Enabled, actorID,
		).Scan(&item.ID, &item.BuiltIn, &item.ImmutableInternal, &item.CreatedAt, &item.UpdatedAt)
	}
	if err != nil {
		return InstructionV2ClientProfile{}, 0, err
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return InstructionV2ClientProfile{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return InstructionV2ClientProfile{}, 0, err
	}
	item.ProfileKey, item.Name, item.Description, item.Matchers = request.ProfileKey, request.Name, request.Description, request.Matchers
	item.Priority, item.Enabled = request.Priority, request.Enabled
	return item, version, nil
}

func (r *InstructionV2Repository) DeleteClientProfile(ctx context.Context, id, actorID int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var builtIn bool
	if err := tx.QueryRowContext(ctx, `SELECT built_in FROM instruction_audit_v2_client_profiles WHERE id = $1 FOR UPDATE`, id).Scan(&builtIn); err != nil {
		return 0, err
	}
	if builtIn {
		return 0, errInstructionV2BuiltInProfile
	}
	var referenced bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM instruction_audit_v2_scopes WHERE client_profile_id = $1
		)`, id).Scan(&referenced); err != nil {
		return 0, err
	}
	if referenced {
		return 0, errInstructionV2ProfileInUse
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM instruction_audit_v2_client_profiles WHERE id = $1`, id); err != nil {
		return 0, err
	}
	version, err := r.bumpConfigVersion(ctx, tx, actorID)
	if err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

func scanInstructionV2HashScopes(rows *sql.Rows, hashes map[int64]*InstructionV2Hash) error {
	for rows.Next() {
		var hashID int64
		var scope InstructionV2HashScope
		var profileID sql.NullInt64
		var expiresAt sql.NullTime
		if err := rows.Scan(
			&hashID, &scope.ScopeID, &scope.GroupID, &scope.GroupName, &profileID,
			&scope.ClientProfileKey, &scope.ClientProfileName, &scope.Status, &scope.Source,
			&expiresAt, &scope.CreatedAt, &scope.UpdatedAt,
		); err != nil {
			return err
		}
		if profileID.Valid {
			value := profileID.Int64
			scope.ClientProfileID = &value
		}
		if expiresAt.Valid {
			value := expiresAt.Time
			scope.CandidateExpiresAt = &value
		}
		if hash := hashes[hashID]; hash != nil {
			hash.ScopeIDs = append(hash.ScopeIDs, scope.ScopeID)
			hash.Scopes = append(hash.Scopes, scope)
		}
	}
	return rows.Err()
}
