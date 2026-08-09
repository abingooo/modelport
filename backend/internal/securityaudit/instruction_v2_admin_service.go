package securityaudit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const instructionV2MaximumManualRawBytes = 16 << 20

func (s *InstructionV2Service) AdminConfig(ctx context.Context) (InstructionV2Config, error) {
	if s == nil || s.repository == nil {
		return InstructionV2Config{}, instructionV2UnavailableError()
	}
	config, err := s.repository.GetConfig(ctx)
	if err != nil {
		return InstructionV2Config{}, err
	}
	config.GatewayHTTPMaxBodyBytes = s.httpMaxBody
	config.GatewayWSMaxBodyBytes = s.wsMaxBody
	config.EvidenceEncryptionReady = s.evidenceCipher != nil && s.evidenceCipher.Available()
	config.AsyncQueueDepth = len(s.asyncQueue)
	config.AsyncQueueCapacity = cap(s.asyncQueue)
	s.stateMu.RLock()
	config.LastConfigLoadError = s.lastLoadError
	s.stateMu.RUnlock()
	if snapshot := s.snapshot.Load(); snapshot != nil {
		loadedAt := snapshot.LoadedAt
		config.LastConfigLoadedAt = &loadedAt
	}
	return config, nil
}

func (s *InstructionV2Service) UpdateAdminConfig(
	ctx context.Context,
	request UpdateInstructionV2ConfigRequest,
	actorID int64,
) (InstructionV2Config, error) {
	if s == nil || s.repository == nil {
		return InstructionV2Config{}, instructionV2UnavailableError()
	}
	if err := validateInstructionV2ConfigRequest(request); err != nil {
		return InstructionV2Config{}, err
	}
	current, err := s.repository.GetConfig(ctx)
	if err != nil {
		return InstructionV2Config{}, err
	}
	if request.Mode != InstructionV2ModeOff {
		if current.ActiveScopeCount == 0 {
			return InstructionV2Config{}, infraerrors.BadRequest(
				"instruction_audit_v2_scope_required", "启用指令审核前至少需要一个有效审核范围",
			)
		}
		if current.EnabledAINodeCount == 0 {
			return InstructionV2Config{}, infraerrors.BadRequest(
				"instruction_audit_v2_ai_node_required", "启用指令审核前至少需要一个可用 AI 节点",
			)
		}
		if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
			return InstructionV2Config{}, infraerrors.ServiceUnavailable(
				"instruction_audit_v2_encryption_unavailable", "原文加密密钥未配置，无法启用指令审核",
			)
		}
	}
	config, err := s.repository.UpdateConfig(ctx, request, actorID)
	if errors.Is(err, errInstructionV2ConfigConflict) {
		return InstructionV2Config{}, infraerrors.Conflict(
			"instruction_audit_v2_config_conflict", "配置已被其他管理员修改，请刷新后重试",
		)
	}
	if err != nil {
		return InstructionV2Config{}, err
	}
	s.refreshAfterMutation(ctx, config.ConfigVersion)
	return s.AdminConfig(ctx)
}

func validateInstructionV2ConfigRequest(request UpdateInstructionV2ConfigRequest) error {
	if request.ExpectedConfigVersion <= 0 {
		return infraerrors.BadRequest("instruction_audit_v2_invalid_version", "配置版本无效")
	}
	if request.Mode != InstructionV2ModeOff && request.Mode != InstructionV2ModeObserve && request.Mode != InstructionV2ModeEnforce {
		return infraerrors.BadRequest("instruction_audit_v2_invalid_mode", "审核模式无效")
	}
	request.ReviewCriteria = strings.TrimSpace(request.ReviewCriteria)
	if len([]rune(request.ReviewCriteria)) > 10000 || request.ConfidenceThreshold < 0.5 || request.ConfidenceThreshold > 1 ||
		request.AIInputMaxChars < 1000 || request.AIInputMaxChars > 1000000 ||
		request.AIGlobalConcurrency < 1 || request.AIGlobalConcurrency > 512 ||
		request.AIQueueWaitMS < 0 || request.AIQueueWaitMS > 30000 ||
		request.AITotalTimeoutMS < 1000 || request.AITotalTimeoutMS > 30000 ||
		request.AICacheTTLSeconds < 0 || request.AICacheTTLSeconds > 86400 ||
		request.EventRetentionDays < 1 || request.EventRetentionDays > 3650 ||
		request.EvidenceRetentionDays < 1 || request.EvidenceRetentionDays > 365 ||
		request.CandidateRetentionDays < 1 || request.CandidateRetentionDays > 365 ||
		request.RawFullMaxBytes < 65536 || request.RawFullMaxBytes > instructionV2MaximumManualRawBytes {
		return infraerrors.BadRequest("instruction_audit_v2_invalid_config", "指令审核配置超出允许范围")
	}
	return nil
}

func (s *InstructionV2Service) ListAdminAINodes(ctx context.Context) ([]InstructionV2AINode, error) {
	return s.repository.ListAINodes(ctx)
}

func (s *InstructionV2Service) SaveAdminAINode(
	ctx context.Context,
	id int64,
	request SaveInstructionV2AINodeRequest,
	actorID int64,
) (InstructionV2AINode, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Model = strings.TrimSpace(request.Model)
	request.APIKey = strings.TrimSpace(request.APIKey)
	normalizedURL, err := NormalizeBaseURL(request.BaseURL)
	if err != nil {
		return InstructionV2AINode{}, err
	}
	request.BaseURL = normalizedURL
	if request.Name == "" || len([]rune(request.Name)) > 120 || request.Model == "" || len(request.Model) > 255 ||
		request.Priority < 0 || request.Priority > 100000 || request.TimeoutMS < 100 || request.TimeoutMS > 30000 ||
		request.MaxConcurrency < 1 || request.MaxConcurrency > 256 {
		return InstructionV2AINode{}, infraerrors.BadRequest(
			"instruction_audit_v2_invalid_ai_node", "AI 审核节点配置无效",
		)
	}
	existingHasKey := false
	if id > 0 {
		items, listErr := s.repository.ListAINodes(ctx)
		if listErr != nil {
			return InstructionV2AINode{}, listErr
		}
		found := false
		for _, item := range items {
			if item.ID == id {
				existingHasKey, found = item.HasAPIKey, true
				break
			}
		}
		if !found {
			return InstructionV2AINode{}, instructionV2NotFoundError("AI 节点")
		}
	}
	hasKey := existingHasKey
	if request.ClearAPIKey {
		hasKey = false
	}
	if request.APIKey != "" {
		hasKey = true
	}
	if request.Enabled && !hasKey {
		return InstructionV2AINode{}, infraerrors.BadRequest(
			"instruction_audit_v2_ai_key_required", "启用 AI 节点前必须配置 API Key",
		)
	}
	ciphertext := ""
	if request.APIKey != "" {
		if s.secretEncryptor == nil {
			return InstructionV2AINode{}, infraerrors.ServiceUnavailable(
				"instruction_audit_v2_secret_unavailable", "AI 节点密钥加密服务不可用",
			)
		}
		ciphertext, err = s.secretEncryptor.Encrypt(request.APIKey)
		if err != nil {
			return InstructionV2AINode{}, infraerrors.ServiceUnavailable(
				"instruction_audit_v2_secret_encrypt_failed", "AI 节点密钥加密失败",
			)
		}
	}
	_, version, err := s.repository.SaveAINode(ctx, id, request, ciphertext, actorID)
	if err != nil {
		return InstructionV2AINode{}, mapInstructionV2RepositoryError(err, "AI 节点")
	}
	s.refreshAfterMutation(ctx, version)
	items, err := s.repository.ListAINodes(ctx)
	if err != nil {
		return InstructionV2AINode{}, err
	}
	for _, item := range items {
		if (id > 0 && item.ID == id) || (id == 0 && item.Name == request.Name && item.Model == request.Model) {
			return item, nil
		}
	}
	return InstructionV2AINode{}, instructionV2NotFoundError("AI 节点")
}

func (s *InstructionV2Service) DeleteAdminAINode(ctx context.Context, id, actorID int64) error {
	version, err := s.repository.DeleteAINode(ctx, id, actorID)
	if err != nil {
		return mapInstructionV2RepositoryError(err, "AI 节点")
	}
	s.refreshAfterMutation(ctx, version)
	return nil
}

func (s *InstructionV2Service) TestAdminAINode(ctx context.Context, id int64) (InstructionV2AINodeTestResult, error) {
	result, latency, err := s.TestAINode(ctx, id)
	if err != nil {
		return InstructionV2AINodeTestResult{}, mapInstructionV2RepositoryError(err, "AI 节点")
	}
	return InstructionV2AINodeTestResult{
		Result: result.Result, Confidence: result.Confidence, Reason: result.Reason,
		Category: result.Category, LatencyMS: int(latency.Milliseconds()),
	}, nil
}

func (s *InstructionV2Service) ListAdminClientProfiles(ctx context.Context) ([]InstructionV2ClientProfile, error) {
	return s.repository.ListClientProfiles(ctx)
}

func (s *InstructionV2Service) SaveAdminClientProfile(
	ctx context.Context,
	id int64,
	request SaveInstructionV2ClientProfileRequest,
	actorID int64,
) (InstructionV2ClientProfile, error) {
	var err error
	if id > 0 {
		profiles, listErr := s.repository.ListClientProfiles(ctx)
		if listErr != nil {
			return InstructionV2ClientProfile{}, listErr
		}
		var existing *InstructionV2ClientProfile
		for index := range profiles {
			if profiles[index].ID == id {
				existing = &profiles[index]
				break
			}
		}
		if existing == nil {
			return InstructionV2ClientProfile{}, instructionV2NotFoundError("客户端规则")
		}
		if existing.ImmutableInternal {
			return InstructionV2ClientProfile{}, infraerrors.Conflict(
				"instruction_audit_v2_immutable_client", "可信内部客户端规则不可修改",
			)
		}
		if existing.BuiltIn {
			request.ProfileKey = existing.ProfileKey
			request.Name = strings.TrimSpace(request.Name)
			request.Description = strings.TrimSpace(request.Description)
			candidate := InstructionV2ClientProfile{
				ProfileKey: request.ProfileKey, Name: request.Name, Description: request.Description,
				Matchers: request.Matchers, Priority: request.Priority, Enabled: request.Enabled,
			}
			if _, err := compileInstructionV2ClientProfile(candidate); err != nil || len(request.Name) > 120 || len(request.Description) > 500 {
				return InstructionV2ClientProfile{}, infraerrors.BadRequest(
					"instruction_audit_v2_invalid_client", "客户端识别规则无效",
				)
			}
			if (request.ProfileKey == InstructionClientOther || request.ProfileKey == InstructionClientUnknown) && !request.Enabled {
				return InstructionV2ClientProfile{}, infraerrors.BadRequest(
					"instruction_audit_v2_required_client", "其他和未知客户端规则必须保持启用",
				)
			}
		} else {
			request, err = normalizeInstructionV2ClientProfileRequest(request)
			if err != nil {
				return InstructionV2ClientProfile{}, infraerrors.BadRequest(
					"instruction_audit_v2_invalid_client", "客户端识别规则无效",
				)
			}
		}
	} else {
		request, err = normalizeInstructionV2ClientProfileRequest(request)
		if err != nil {
			return InstructionV2ClientProfile{}, infraerrors.BadRequest(
				"instruction_audit_v2_invalid_client", "客户端识别规则无效",
			)
		}
	}
	item, version, err := s.repository.SaveClientProfile(ctx, id, request, actorID)
	if err != nil {
		return InstructionV2ClientProfile{}, mapInstructionV2RepositoryError(err, "客户端规则")
	}
	s.refreshAfterMutation(ctx, version)
	return item, nil
}

func (s *InstructionV2Service) DeleteAdminClientProfile(ctx context.Context, id, actorID int64) error {
	version, err := s.repository.DeleteClientProfile(ctx, id, actorID)
	if err != nil {
		return mapInstructionV2RepositoryError(err, "客户端规则")
	}
	s.refreshAfterMutation(ctx, version)
	return nil
}

func (s *InstructionV2Service) ListAdminScopes(ctx context.Context) ([]InstructionV2Scope, error) {
	return s.repository.ListScopes(ctx)
}

func (s *InstructionV2Service) ListAdminGroups(ctx context.Context) ([]InstructionGroupOption, error) {
	return s.repository.ListGroupOptions(ctx)
}

func (s *InstructionV2Service) SaveAdminScope(
	ctx context.Context,
	id int64,
	request SaveInstructionV2ScopeRequest,
	actorID int64,
) (InstructionV2Scope, error) {
	if request.GroupID <= 0 {
		return InstructionV2Scope{}, infraerrors.BadRequest("instruction_audit_v2_invalid_group", "下游分组无效")
	}
	item, version, err := s.repository.SaveScope(ctx, id, request, actorID)
	if err != nil {
		return InstructionV2Scope{}, mapInstructionV2RepositoryError(err, "审核范围")
	}
	s.refreshAfterMutation(ctx, version)
	return item, nil
}

func (s *InstructionV2Service) DeleteAdminScope(ctx context.Context, id, actorID int64) error {
	version, err := s.repository.DeleteScope(ctx, id, actorID)
	if err != nil {
		return mapInstructionV2RepositoryError(err, "审核范围")
	}
	s.refreshAfterMutation(ctx, version)
	return nil
}

func (s *InstructionV2Service) ListAdminUserAllowlist(ctx context.Context) ([]InstructionV2UserAllowlistEntry, error) {
	return s.repository.ListUserAllowlist(ctx)
}

func (s *InstructionV2Service) ListAdminUserOptions(ctx context.Context, query string) ([]InstructionV2UserOption, error) {
	return s.repository.ListUserOptions(ctx, query)
}

func (s *InstructionV2Service) SaveAdminUserAllowlist(
	ctx context.Context,
	request SaveInstructionV2UserAllowlistRequest,
	actorID int64,
) (InstructionV2UserAllowlistEntry, error) {
	request.Note = strings.TrimSpace(request.Note)
	if request.UserID <= 0 || len([]rune(request.Note)) > 500 {
		return InstructionV2UserAllowlistEntry{}, infraerrors.BadRequest(
			"instruction_audit_v2_invalid_user_allowlist", "用户白名单配置无效",
		)
	}
	item, version, err := s.repository.SaveUserAllowlist(ctx, request, actorID)
	if err != nil {
		return InstructionV2UserAllowlistEntry{}, mapInstructionV2RepositoryError(err, "用户白名单")
	}
	s.refreshAfterMutation(ctx, version)
	return item, nil
}

func (s *InstructionV2Service) DeleteAdminUserAllowlist(ctx context.Context, id, actorID int64) error {
	version, err := s.repository.DeleteUserAllowlist(ctx, id, actorID)
	if err != nil {
		return mapInstructionV2RepositoryError(err, "用户白名单")
	}
	s.refreshAfterMutation(ctx, version)
	return nil
}

func (s *InstructionV2Service) ListAdminHashes(
	ctx context.Context,
	page, pageSize int,
	status, query string,
) (InstructionV2HashPage, error) {
	page, pageSize = normalizeInstructionV2Page(page, pageSize)
	status = strings.TrimSpace(status)
	if status != "" && !validInstructionV2HashStatus(status) {
		return InstructionV2HashPage{}, infraerrors.BadRequest("instruction_audit_v2_invalid_hash_status", "可信指令状态无效")
	}
	return s.repository.ListHashes(ctx, page, pageSize, status, query)
}

func (s *InstructionV2Service) GetAdminHash(ctx context.Context, id int64) (InstructionV2Hash, error) {
	item, _, err := s.repository.GetHash(ctx, id)
	if err != nil {
		return InstructionV2Hash{}, mapInstructionV2RepositoryError(err, "可信指令")
	}
	return item, nil
}

func (s *InstructionV2Service) CreateAdminHash(
	ctx context.Context,
	request SaveInstructionV2HashRequest,
	actorID int64,
) (InstructionV2Hash, error) {
	write, err := s.prepareInstructionV2ManualHash(ctx, request)
	if err != nil {
		return InstructionV2Hash{}, err
	}
	item, version, err := s.repository.SaveManualHash(ctx, write, actorID)
	if err != nil {
		return InstructionV2Hash{}, mapInstructionV2RepositoryError(err, "可信指令")
	}
	s.refreshAfterMutation(ctx, version)
	return item, nil
}

func (s *InstructionV2Service) prepareInstructionV2ManualHash(
	ctx context.Context,
	request SaveInstructionV2HashRequest,
) (instructionV2ManualHashWrite, error) {
	request.SHA256 = strings.ToLower(strings.TrimSpace(request.SHA256))
	request.Name = strings.TrimSpace(request.Name)
	request.Note = strings.TrimSpace(request.Note)
	request.Source = strings.TrimSpace(request.Source)
	request.Status = strings.TrimSpace(request.Status)
	if request.Source == "" {
		request.Source = "manual"
	}
	if request.Status == "" {
		request.Status = "active"
	}
	if (request.Source != "manual" && request.Source != "import") || !validInstructionV2HashStatus(request.Status) ||
		len([]rune(request.Name)) > 160 || len([]rune(request.Note)) > 1000 {
		return instructionV2ManualHashWrite{}, infraerrors.BadRequest(
			"instruction_audit_v2_invalid_hash", "可信指令配置无效",
		)
	}
	scopeIDs, err := normalizeInstructionV2IDs(request.ScopeIDs, 200)
	if err != nil || len(scopeIDs) == 0 {
		return instructionV2ManualHashWrite{}, infraerrors.BadRequest(
			"instruction_audit_v2_scope_required", "可信指令至少需要一个审核范围",
		)
	}
	write := instructionV2ManualHashWrite{
		Name: request.Name, Note: request.Note, Status: request.Status, Source: request.Source,
		RawStorage: "unavailable", ScopeIDs: scopeIDs,
	}
	if request.RawContent != "" {
		if len([]byte(request.RawContent)) > instructionV2MaximumManualRawBytes {
			return instructionV2ManualHashWrite{}, infraerrors.BadRequest(
				"instruction_audit_v2_raw_too_large", "手工录入的指令原文不能超过 16 MiB",
			)
		}
		field := newInstructionV2TextField(request.RawContent, false)
		if request.SHA256 != "" && request.SHA256 != field.SHA256 {
			return instructionV2ManualHashWrite{}, infraerrors.BadRequest(
				"instruction_audit_v2_digest_mismatch", "原文与 SHA-256 不一致",
			)
		}
		config, configErr := s.AdminConfig(ctx)
		if configErr != nil {
			return instructionV2ManualHashWrite{}, configErr
		}
		plaintext := request.RawContent
		write.RawStorage = "full"
		if field.Bytes > int64(config.RawFullMaxBytes) {
			prepared := prepareInstructionV2AISample(field, config.AIInputMaxChars)
			plaintext = prepared.AISample
			write.RawStorage = "sample"
		}
		if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
			return instructionV2ManualHashWrite{}, infraerrors.ServiceUnavailable(
				"instruction_audit_v2_encryption_unavailable", "原文加密服务不可用",
			)
		}
		ciphertext, encryptErr := s.evidenceCipher.EncryptHashRaw(field.SHA256, plaintext)
		if encryptErr != nil {
			return instructionV2ManualHashWrite{}, infraerrors.ServiceUnavailable(
				"instruction_audit_v2_encrypt_failed", "可信指令原文加密失败",
			)
		}
		write.SHA256, write.ContentBytes = field.SHA256, field.Bytes
		write.RawCiphertext, write.StoredBytes = ciphertext, len([]byte(plaintext))
	} else {
		if !instructionDigestPattern.MatchString(request.SHA256) {
			return instructionV2ManualHashWrite{}, infraerrors.BadRequest(
				"instruction_audit_v2_invalid_digest", "请输入 64 位小写 SHA-256 或指令原文",
			)
		}
		write.SHA256 = request.SHA256
	}
	if request.Status == "candidate" {
		config, configErr := s.AdminConfig(ctx)
		if configErr != nil {
			return instructionV2ManualHashWrite{}, configErr
		}
		expiresAt := time.Now().UTC().Add(time.Duration(config.CandidateRetentionDays) * 24 * time.Hour)
		write.CandidateExpiresAt = &expiresAt
	}
	return write, nil
}

func (s *InstructionV2Service) UpdateAdminHash(
	ctx context.Context,
	id int64,
	request UpdateInstructionV2HashRequest,
	actorID int64,
) (InstructionV2Hash, error) {
	if request.Name != nil && len([]rune(strings.TrimSpace(*request.Name))) > 160 {
		return InstructionV2Hash{}, infraerrors.BadRequest("instruction_audit_v2_invalid_hash_name", "可信指令名称过长")
	}
	if request.Note != nil && len([]rune(strings.TrimSpace(*request.Note))) > 1000 {
		return InstructionV2Hash{}, infraerrors.BadRequest("instruction_audit_v2_invalid_hash_note", "可信指令备注过长")
	}
	if request.Status != nil && !validInstructionV2HashStatus(strings.TrimSpace(*request.Status)) {
		return InstructionV2Hash{}, infraerrors.BadRequest("instruction_audit_v2_invalid_hash_status", "可信指令状态无效")
	}
	if request.SetScopes {
		ids, err := normalizeInstructionV2IDs(request.ScopeIDs, 200)
		if err != nil || len(ids) == 0 {
			return InstructionV2Hash{}, infraerrors.BadRequest(
				"instruction_audit_v2_scope_required", "可信指令至少需要一个审核范围",
			)
		}
		request.ScopeIDs = ids
	}
	candidateRetentionDays := InstructionV2DefaultCandidateDays
	if snapshot := s.snapshot.Load(); snapshot != nil && snapshot.Config.CandidateRetentionDays > 0 {
		candidateRetentionDays = snapshot.Config.CandidateRetentionDays
	}
	item, version, err := s.repository.UpdateHash(ctx, id, request, actorID, candidateRetentionDays)
	if err != nil {
		return InstructionV2Hash{}, mapInstructionV2RepositoryError(err, "可信指令")
	}
	s.refreshAfterMutation(ctx, version)
	return item, nil
}

func (s *InstructionV2Service) DeleteAdminHash(ctx context.Context, id, actorID int64) error {
	version, err := s.repository.DeleteHash(ctx, id, actorID)
	if err != nil {
		return mapInstructionV2RepositoryError(err, "可信指令")
	}
	s.refreshAfterMutation(ctx, version)
	return nil
}

func (s *InstructionV2Service) RevealAdminHashRaw(
	ctx context.Context,
	id int64,
	access InstructionV2RawAccess,
) (result InstructionV2EvidenceReview, resultErr error) {
	access.Action = "reveal"
	defer func() {
		_ = s.repository.RecordRawAccess(ctx, "hash", id, access, resultErr == nil, instructionV2RawErrorCode(resultErr))
	}()
	item, ciphertext, err := s.repository.GetHash(ctx, id)
	if err != nil {
		return InstructionV2EvidenceReview{}, mapInstructionV2RepositoryError(err, "可信指令")
	}
	if item.RawStorage == "unavailable" || len(ciphertext) == 0 {
		return InstructionV2EvidenceReview{}, infraerrors.NotFound(
			"instruction_audit_v2_raw_unavailable", "该可信指令没有可查看的原文",
		)
	}
	plaintext, err := s.evidenceCipher.DecryptHashRaw(item.SHA256, ciphertext)
	if err != nil {
		return InstructionV2EvidenceReview{}, infraerrors.ServiceUnavailable(
			"instruction_audit_v2_decrypt_failed", "可信指令原文解密失败",
		)
	}
	consistent := false
	if item.RawStorage == "full" {
		digest := sha256.Sum256([]byte(plaintext))
		consistent = hex.EncodeToString(digest[:]) == item.SHA256
	}
	return InstructionV2EvidenceReview{
		ResourceType: "hash", ResourceID: id,
		Fields: []InstructionV2EvidenceField{{
			FieldName: item.ObservedField, SHA256: item.SHA256, StorageKind: item.RawStorage,
			Plaintext: plaintext, ContentBytes: item.ContentBytes, StoredBytes: item.StoredBytes,
			DigestConsistent: consistent,
		}},
	}, nil
}

func (s *InstructionV2Service) RecordAdminHashRawCopy(ctx context.Context, id int64, access InstructionV2RawAccess) error {
	access.Action = "copy"
	item, ciphertext, err := s.repository.GetHash(ctx, id)
	if err == nil && (item.RawStorage == "unavailable" || len(ciphertext) == 0) {
		err = infraerrors.NotFound("instruction_audit_v2_raw_unavailable", "该可信指令没有可复制的原文")
	}
	_ = s.repository.RecordRawAccess(ctx, "hash", id, access, err == nil, instructionV2RawErrorCode(err))
	return mapInstructionV2RepositoryError(err, "可信指令")
}

func (s *InstructionV2Service) ListAdminEvents(
	ctx context.Context,
	page, pageSize int,
	filter InstructionV2EventFilter,
) (InstructionV2EventPage, error) {
	page, pageSize = normalizeInstructionV2Page(page, pageSize)
	return s.repository.ListEvents(ctx, page, pageSize, filter)
}

func (s *InstructionV2Service) GetAdminEvent(ctx context.Context, id int64) (InstructionV2Event, error) {
	item, err := s.repository.GetEvent(ctx, id)
	if err != nil {
		return InstructionV2Event{}, mapInstructionV2RepositoryError(err, "审核事件")
	}
	return item, nil
}

func (s *InstructionV2Service) AdminStatistics(ctx context.Context, filter InstructionV2EventFilter) (InstructionV2Statistics, error) {
	return s.repository.Statistics(ctx, filter)
}

func (s *InstructionV2Service) DeleteAdminEvents(ctx context.Context, ids []int64) (int64, error) {
	ids, err := normalizeInstructionV2IDs(ids, 1000)
	if err != nil || len(ids) == 0 {
		return 0, infraerrors.BadRequest("instruction_audit_v2_invalid_event_ids", "请选择要删除的审核事件")
	}
	return s.repository.DeleteEvents(ctx, ids)
}

func (s *InstructionV2Service) RevealAdminEventEvidence(
	ctx context.Context,
	id int64,
	access InstructionV2RawAccess,
) (result InstructionV2EvidenceReview, resultErr error) {
	access.Action = "reveal"
	defer func() {
		_ = s.repository.RecordRawAccess(ctx, "event", id, access, resultErr == nil, instructionV2RawErrorCode(resultErr))
	}()
	if _, err := s.repository.GetEvent(ctx, id); err != nil {
		return InstructionV2EvidenceReview{}, mapInstructionV2RepositoryError(err, "审核事件")
	}
	evidence, err := s.repository.GetEventEvidence(ctx, id)
	if err != nil {
		return InstructionV2EvidenceReview{}, err
	}
	if len(evidence) == 0 {
		return InstructionV2EvidenceReview{}, infraerrors.NotFound(
			"instruction_audit_v2_evidence_unavailable", "该审核事件没有可查看的原文证据",
		)
	}
	result = InstructionV2EvidenceReview{ResourceType: "event", ResourceID: id, Fields: make([]InstructionV2EvidenceField, 0, len(evidence))}
	for _, item := range evidence {
		plaintext, decryptErr := s.evidenceCipher.Decrypt(item.FieldName, item.SHA256, item.Ciphertext)
		if decryptErr != nil {
			return InstructionV2EvidenceReview{}, infraerrors.ServiceUnavailable(
				"instruction_audit_v2_decrypt_failed", "审核事件原文解密失败",
			)
		}
		consistent := false
		if item.StorageKind == "full" {
			digest := sha256.Sum256([]byte(plaintext))
			consistent = hex.EncodeToString(digest[:]) == item.SHA256
		}
		result.Fields = append(result.Fields, InstructionV2EvidenceField{
			FieldName: item.FieldName, SHA256: item.SHA256, StorageKind: item.StorageKind,
			Plaintext: plaintext, ContentBytes: item.ContentBytes, StoredBytes: item.StoredBytes,
			DigestConsistent: consistent,
		})
	}
	return result, nil
}

func (s *InstructionV2Service) RecordAdminEventEvidenceCopy(ctx context.Context, id int64, access InstructionV2RawAccess) error {
	access.Action = "copy"
	items, err := s.repository.GetEventEvidence(ctx, id)
	if err == nil && len(items) == 0 {
		err = infraerrors.NotFound("instruction_audit_v2_evidence_unavailable", "该审核事件没有可复制的原文证据")
	}
	_ = s.repository.RecordRawAccess(ctx, "event", id, access, err == nil, instructionV2RawErrorCode(err))
	return mapInstructionV2RepositoryError(err, "审核事件")
}

func (s *InstructionV2Service) TrustAdminEvent(
	ctx context.Context,
	id int64,
	request InstructionV2TrustEventRequest,
	actorID int64,
) (InstructionV2TrustEventResult, error) {
	event, err := s.repository.GetEvent(ctx, id)
	if err != nil {
		return InstructionV2TrustEventResult{}, mapInstructionV2RepositoryError(err, "审核事件")
	}
	if event.ScopeID == nil || *event.ScopeID <= 0 {
		return InstructionV2TrustEventResult{}, infraerrors.Conflict(
			"instruction_audit_v2_event_scope_unavailable", "审核事件的原始作用域已不可用",
		)
	}
	wanted := make(map[string]struct{}, 2)
	for _, field := range request.Fields {
		field = strings.TrimSpace(field)
		if field != "instructions" && field != "input1" {
			return InstructionV2TrustEventResult{}, infraerrors.BadRequest(
				"instruction_audit_v2_invalid_field", "可信字段只能选择 instructions 或 input[1]",
			)
		}
		wanted[field] = struct{}{}
	}
	if len(wanted) == 0 {
		return InstructionV2TrustEventResult{}, infraerrors.BadRequest(
			"instruction_audit_v2_field_required", "请至少选择一个需要信任的字段",
		)
	}
	evidence, err := s.repository.GetEventEvidence(ctx, id)
	if err != nil {
		return InstructionV2TrustEventResult{}, err
	}
	byField := make(map[string]instructionV2EvidenceWrite, len(evidence))
	for _, item := range evidence {
		byField[item.FieldName] = item
	}
	name := strings.TrimSpace(request.Name)
	note := strings.TrimSpace(request.Note)
	if len([]rune(name)) > 160 || len([]rune(note)) > 1000 {
		return InstructionV2TrustEventResult{}, infraerrors.BadRequest(
			"instruction_audit_v2_invalid_hash", "可信指令名称或备注过长",
		)
	}
	writes := make([]instructionV2ManualHashWrite, 0, len(wanted))
	for _, fieldName := range []string{"instructions", "input1"} {
		if _, ok := wanted[fieldName]; !ok {
			continue
		}
		item, ok := byField[fieldName]
		if !ok {
			return InstructionV2TrustEventResult{}, infraerrors.NotFound(
				"instruction_audit_v2_evidence_unavailable", fmt.Sprintf("%s 没有可用于加库的原文证据", fieldName),
			)
		}
		plaintext, decryptErr := s.evidenceCipher.Decrypt(item.FieldName, item.SHA256, item.Ciphertext)
		if decryptErr != nil {
			return InstructionV2TrustEventResult{}, infraerrors.ServiceUnavailable(
				"instruction_audit_v2_decrypt_failed", "审核事件原文解密失败",
			)
		}
		hashCiphertext, encryptErr := s.evidenceCipher.EncryptHashRaw(item.SHA256, plaintext)
		if encryptErr != nil {
			return InstructionV2TrustEventResult{}, infraerrors.ServiceUnavailable(
				"instruction_audit_v2_encrypt_failed", "可信指令原文加密失败",
			)
		}
		itemName := name
		if itemName == "" {
			itemName = fmt.Sprintf("事件 #%d %s", event.ID, fieldName)
		} else if len(wanted) > 1 {
			itemName += " - " + fieldName
		}
		writes = append(writes, instructionV2ManualHashWrite{
			SHA256: item.SHA256, Name: itemName, Note: note, Status: "active", Source: "manual",
			ContentBytes: item.ContentBytes, RawStorage: item.StorageKind, RawCiphertext: hashCiphertext,
			StoredBytes: item.StoredBytes, ScopeIDs: []int64{*event.ScopeID},
		})
	}
	hashes, version, err := s.repository.SaveManualHashes(ctx, writes, actorID)
	if err != nil {
		return InstructionV2TrustEventResult{}, mapInstructionV2RepositoryError(err, "可信指令")
	}
	s.refreshAfterMutation(ctx, version)
	return InstructionV2TrustEventResult{Hashes: hashes}, nil
}

func validInstructionV2HashStatus(status string) bool {
	switch status {
	case "candidate", "active", "disabled", "revoked":
		return true
	default:
		return false
	}
}

func normalizeInstructionV2Page(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func normalizeInstructionV2IDs(values []int64, maximum int) ([]int64, error) {
	if len(values) > maximum {
		return nil, errors.New("too many instruction audit v2 identifiers")
	}
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil, errors.New("invalid instruction audit v2 identifier")
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result, nil
}

func mapInstructionV2RepositoryError(err error, resource string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return instructionV2NotFoundError(resource)
	}
	if errors.Is(err, errInstructionV2RevokedHash) {
		return infraerrors.Conflict("instruction_audit_v2_hash_revoked", "已撤销的可信指令不能重新启用")
	}
	if errors.Is(err, errInstructionV2ImmutableProfile) || errors.Is(err, errInstructionV2BuiltInProfile) {
		return infraerrors.Conflict("instruction_audit_v2_client_protected", "系统内置客户端规则不能执行该操作")
	}
	if errors.Is(err, errInstructionV2ProfileInUse) {
		return infraerrors.Conflict("instruction_audit_v2_client_in_use", "客户端规则仍被审核范围引用，请先删除关联范围")
	}
	return err
}

func instructionV2NotFoundError(resource string) error {
	return infraerrors.NotFound("instruction_audit_v2_not_found", resource+"不存在")
}

func instructionV2UnavailableError() error {
	return infraerrors.ServiceUnavailable("instruction_audit_v2_unavailable", "指令审核服务不可用")
}

func instructionV2RawErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if reason := infraerrors.Reason(err); reason != "" {
		return reason
	}
	return "instruction_audit_v2_raw_access_failed"
}
