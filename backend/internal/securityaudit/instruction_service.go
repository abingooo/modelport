package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	instructionAuditProtocol                  = "openai_responses"
	instructionBlockedEventPersistenceTimeout = 3 * time.Second
	instructionSnapshotMaxStaleness           = 30 * time.Second
)

var instructionDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var validInstructionEventReasons = map[string]struct{}{
	"hash_mismatch": {}, "fields_missing": {}, "field_invalid": {}, "invalid_json": {},
	"request_too_large": {}, "structure_too_complex": {}, "parse_timeout": {}, "config_unavailable": {},
}

var validInstructionFieldResults = map[string]struct{}{
	"missing": {}, "invalid": {}, "mismatch": {}, "match": {}, "not_checked": {},
}

var validSecurityNotificationStatuses = map[string]struct{}{
	"pending": {}, "processing": {}, "retry": {}, "sent": {}, "failed": {},
	"suppressed": {}, "no_recipient": {},
}

type InstructionService struct {
	repository     *InstructionRepository
	redis          *redis.Client
	evidenceCipher *InstructionEvidenceCipher
	notifications  *service.SecurityNotificationService

	snapshot                   atomic.Pointer[instructionSnapshot]
	requiredConfigVersion      atomic.Int64
	failedBlockedEventPersists atomic.Uint64
	reloadMu                   sync.Mutex

	stateMu       sync.RWMutex
	lastLoadError string

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type instructionBlockedEvent struct {
	Request  Request
	Decision InstructionDecision
}

func NewInstructionService(repository *InstructionRepository, redisClient *redis.Client, emailService *service.EmailService) *InstructionService {
	return &InstructionService{
		repository: repository,
		redis:      redisClient,
	}
}

func ProvideInstructionService(
	repository *InstructionRepository,
	redisClient *redis.Client,
	emailService *service.EmailService,
	evidenceCipher *InstructionEvidenceCipher,
	notifications *service.SecurityNotificationService,
) *InstructionService {
	result := NewInstructionService(repository, redisClient, emailService)
	result.evidenceCipher = evidenceCipher
	result.notifications = notifications
	return result
}

func (s *InstructionService) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("instruction audit service unavailable")
	}
	if s.repository == nil {
		s.setLoadError("configuration repository unavailable")
		return errors.New("instruction audit repository unavailable")
	}
	s.lifecycleMu.Lock()
	if s.cancel != nil {
		s.lifecycleMu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.lifecycleMu.Unlock()

	loadErr := s.Reload(runCtx)
	s.wg.Add(2)
	go s.refreshLoop(runCtx)
	go s.evidenceCleanupLoop(runCtx)
	if s.redis != nil {
		s.wg.Add(1)
		go s.subscribeLoop(runCtx)
	}
	return loadErr
}

func (s *InstructionService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *InstructionService) Reload(ctx context.Context) error {
	if s == nil || s.repository == nil {
		s.setLoadError("configuration repository unavailable")
		return errors.New("instruction audit repository unavailable")
	}
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	snapshot, err := s.repository.LoadSnapshot(ctx)
	if err != nil {
		s.setLoadError("configuration refresh failed")
		return err
	}
	if required := s.requiredConfigVersion.Load(); snapshot.ConfigVersion < required {
		s.setLoadError("required configuration version is not available")
		return fmt.Errorf("instruction audit snapshot version %d is below required version %d", snapshot.ConfigVersion, required)
	}
	if err := s.storeSnapshot(snapshot); err != nil {
		s.setLoadError("stale configuration snapshot rejected")
		return err
	}
	s.setLoadError("")
	return nil
}

func (s *InstructionService) storeSnapshot(snapshot *instructionSnapshot) error {
	if snapshot == nil {
		return errors.New("instruction audit snapshot is nil")
	}
	if current := s.snapshot.Load(); current != nil && snapshot.ConfigVersion < current.ConfigVersion {
		return fmt.Errorf("instruction audit snapshot version regressed from %d to %d", current.ConfigVersion, snapshot.ConfigVersion)
	}
	s.snapshot.Store(snapshot)
	return nil
}

func (s *InstructionService) EvaluateInstruction(ctx context.Context, request Request) *InstructionDecision {
	if request.Protocol != instructionAuditProtocol || request.InstructionAuditExcluded {
		return &InstructionDecision{Allow: true}
	}
	startedAt := time.Now()
	request.InstructionClientType = ClassifyInstructionClient(request.UserAgent, request.TrustedInternalClient)
	request.UserAgent = instructionUserAgentSnapshot(request.UserAgent)
	snapshot := s.snapshot.Load()
	if snapshot == nil {
		return s.blockInstructionConfigUnavailable(ctx, request, snapshot, startedAt)
	}
	if !snapshot.Enabled {
		return &InstructionDecision{Allow: true, ConfigVersion: snapshot.ConfigVersion}
	}
	policy, applicable := instructionPolicyFor(snapshot, instructionGroupID(request.GroupID), request.InstructionClientType)
	if !applicable {
		return &InstructionDecision{Allow: true, ConfigVersion: snapshot.ConfigVersion}
	}
	if s.instructionSnapshotStale(snapshot, startedAt) {
		return s.blockInstructionConfigUnavailable(ctx, request, snapshot, startedAt)
	}

	body := request.InstructionBody
	if len(body) == 0 {
		body = request.Body
	}
	root, err := decodeStrictJSONObject(body)
	if err != nil {
		decision := &InstructionDecision{
			Applicable:    true,
			Allow:         false,
			Reason:        instructionAuditParseReason(err),
			Instructions:  InstructionFieldResult{Result: "invalid"},
			Input1:        InstructionFieldResult{Result: "not_checked"},
			RuleSetIDs:    append([]int64(nil), policy.RuleSetIDs...),
			ConfigVersion: snapshot.ConfigVersion,
		}
		decision.Latency = time.Since(startedAt)
		s.recordBlocked(ctx, request, decision)
		return decision
	}

	if !request.InstructionModelOverride || strings.TrimSpace(request.Model) == "" {
		if strictModel, ok := strictInstructionModel(root); ok {
			request.Model = strictModel
		}
	}
	inspection := inspectInstructionRoot(root, policy.Hashes, time.Now().UTC())
	decision := &InstructionDecision{
		Applicable:    true,
		Allow:         inspection.Allow,
		Reason:        inspection.Reason,
		Instructions:  inspection.Instructions,
		Input1:        inspection.Input1,
		RuleSetIDs:    append([]int64(nil), policy.RuleSetIDs...),
		ConfigVersion: snapshot.ConfigVersion,
	}
	decision.Latency = time.Since(startedAt)
	if !decision.Allow {
		s.recordBlocked(ctx, request, decision)
	}
	return decision
}

func (s *InstructionService) instructionSnapshotStale(snapshot *instructionSnapshot, now time.Time) bool {
	if snapshot == nil {
		return true
	}
	s.stateMu.RLock()
	hasLoadError := s.lastLoadError != ""
	s.stateMu.RUnlock()
	if !hasLoadError {
		return false
	}
	return snapshot.LoadedAt.IsZero() || now.Sub(snapshot.LoadedAt) > instructionSnapshotMaxStaleness
}

func (s *InstructionService) blockInstructionConfigUnavailable(ctx context.Context, request Request, snapshot *instructionSnapshot, startedAt time.Time) *InstructionDecision {
	configVersion := int64(1)
	if snapshot != nil && snapshot.ConfigVersion > 0 {
		configVersion = snapshot.ConfigVersion
	}
	decision := &InstructionDecision{
		Applicable:    true,
		Allow:         false,
		Unavailable:   true,
		Reason:        "config_unavailable",
		Instructions:  InstructionFieldResult{Result: "not_checked"},
		Input1:        InstructionFieldResult{Result: "not_checked"},
		ConfigVersion: configVersion,
		Latency:       time.Since(startedAt),
	}
	s.recordBlocked(ctx, request, decision)
	return decision
}

func (s *InstructionService) requireConfigVersion(version int64) {
	if version < 1 {
		return
	}
	for {
		current := s.requiredConfigVersion.Load()
		if version <= current || s.requiredConfigVersion.CompareAndSwap(current, version) {
			return
		}
	}
}

func instructionPolicyFor(snapshot *instructionSnapshot, groupID int64, clientType string) (instructionPolicy, bool) {
	if snapshot == nil {
		return instructionPolicy{}, false
	}
	_, wildcardAudited := snapshot.AuditedGroups[groupID]
	scope := instructionPolicyScope{GroupID: groupID, ClientType: clientType}
	_, clientAudited := snapshot.AuditedClientScopes[scope]
	if !wildcardAudited && !clientAudited {
		return instructionPolicy{}, false
	}
	accumulator := newInstructionPolicyAccumulator()
	if wildcardAudited {
		mergeInstructionPolicy(accumulator, snapshot.Policies[groupID])
	}
	if clientAudited {
		mergeInstructionPolicy(accumulator, snapshot.ClientPolicies[scope])
	}
	return buildInstructionPolicy(accumulator), true
}

func mergeInstructionPolicy(accumulator *instructionPolicyAccumulator, policy instructionPolicy) {
	if accumulator == nil {
		return
	}
	for _, ruleSetID := range policy.RuleSetIDs {
		accumulator.ruleSets[ruleSetID] = struct{}{}
	}
	for _, hash := range policy.Hashes {
		accumulator.hashes[hash.Digest] = hash
	}
}

func (s *InstructionService) recordBlocked(ctx context.Context, request Request, decision *InstructionDecision) {
	if s == nil || s.repository == nil || decision == nil {
		return
	}
	eventRequest := request
	if request.GroupID != nil {
		groupID := *request.GroupID
		eventRequest.GroupID = &groupID
	}
	event := instructionBlockedEvent{Request: eventRequest, Decision: *decision}
	event.Request.Body = nil
	event.Request.InstructionBody = nil
	event.Decision.RuleSetIDs = append([]int64(nil), decision.RuleSetIDs...)
	evidenceStatus, evidenceExpiresAt, evidence := s.prepareEvidence(ctx, decision)
	event.Decision.Instructions.Plaintext = ""
	event.Decision.Input1.Plaintext = ""
	if ctx == nil {
		ctx = context.Background()
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instructionBlockedEventPersistenceTimeout)
	defer cancel()
	eventID, err := s.repository.RecordBlocked(
		recordCtx, event.Request, &event.Decision, evidenceStatus, evidenceExpiresAt, evidence,
	)
	if err != nil {
		s.failedBlockedEventPersists.Add(1)
		slog.Error("instruction_audit.record_blocked_failed",
			"request_id", event.Request.RequestID,
			"user_id", event.Request.UserID,
			"api_key_id", event.Request.APIKeyID,
			"group_id", instructionGroupID(event.Request.GroupID),
			"model", event.Request.Model,
			"reason", event.Decision.Reason,
			"error", err,
		)
		return
	}
	if s.notifications != nil {
		groupID := instructionGroupID(event.Request.GroupID)
		variables := map[string]string{
			"request_id":   event.Request.RequestID,
			"triggered_at": time.Now().UTC().Format(time.RFC3339),
			"user_id":      strconv.FormatInt(event.Request.UserID, 10),
			"user_email":   event.Request.UserEmail,
			"api_key_id":   strconv.FormatInt(event.Request.APIKeyID, 10),
			"group_id":     strconv.FormatInt(groupID, 10),
			"group_name":   event.Request.GroupName,
			"client_type":  event.Request.InstructionClientType,
			"model":        event.Request.Model,
			"admin_qq":     "2145236436",
		}
		err = s.notifications.Enqueue(recordCtx, service.SecurityNotificationEnqueueInput{
			SourceType: service.SecurityNotificationSourceInstructionAudit,
			SourceID:   eventID, UserID: event.Request.UserID, UserEmail: event.Request.UserEmail,
			DedupeScope:  fmt.Sprintf("%d:%d:instruction-audit", event.Request.UserID, groupID),
			UserTemplate: service.NotificationEmailEventInstructionAuditUserNotice,
			OpsTemplate:  service.NotificationEmailEventInstructionAuditOpsNotice,
			Variables:    variables,
		})
		if err != nil {
			slog.Warn("instruction_audit.notification_enqueue_failed", "event_id", eventID, "error", err)
		}
	}
}

func (s *InstructionService) prepareEvidence(ctx context.Context, decision *InstructionDecision) (string, *time.Time, []InstructionEvidence) {
	if decision == nil {
		return "not_available", nil, nil
	}
	fields := []struct {
		source string
		field  InstructionFieldResult
	}{
		{source: "instructions", field: decision.Instructions},
		{source: "input1", field: decision.Input1},
	}
	hasPlaintext := false
	for _, item := range fields {
		if item.field.Plaintext != "" && item.field.SHA256 != "" {
			hasPlaintext = true
			break
		}
	}
	if !hasPlaintext {
		return "not_available", nil, nil
	}
	if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
		return "encryption_unavailable", nil, nil
	}
	retentionDays, err := s.repository.GetEvidenceRetentionDays(ctx)
	if err != nil || retentionDays < 1 || retentionDays > 3650 {
		retentionDays = 30
		slog.Warn("instruction_audit.evidence_retention_fallback", "error", err)
	}
	expiresAt := time.Now().UTC().Add(time.Duration(retentionDays) * 24 * time.Hour)
	evidence := make([]InstructionEvidence, 0, 2)
	for _, item := range fields {
		if item.field.Plaintext == "" || item.field.SHA256 == "" {
			continue
		}
		ciphertext, err := s.evidenceCipher.Encrypt(item.source, item.field.SHA256, item.field.Plaintext)
		if err != nil {
			slog.Error("instruction_audit.evidence_encrypt_failed", "source", item.source, "error", err)
			return "encryption_unavailable", nil, nil
		}
		evidence = append(evidence, InstructionEvidence{
			Source: item.source, Digest: item.field.SHA256, Ciphertext: ciphertext,
			KeyVersion: instructionEvidenceKeyVersion, PlaintextBytes: len([]byte(item.field.Plaintext)),
			ExpiresAt: expiresAt,
		})
	}
	return "stored", &expiresAt, evidence
}

func (s *InstructionService) Overview(ctx context.Context) (*InstructionOverview, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("instruction audit service unavailable")
	}
	hashes, activeHashes, ruleSets, auditedGroups, effectiveGroups, pendingEmails, err := s.repository.OverviewCounts(ctx)
	if err != nil {
		return nil, err
	}
	overview := &InstructionOverview{
		HashCount:                   hashes,
		ActiveHashCount:             activeHashes,
		RuleSetCount:                ruleSets,
		AuditedGroupCount:           auditedGroups,
		EffectiveGroupCount:         effectiveGroups,
		PendingEmailCount:           pendingEmails,
		QueuedEventCount:            0,
		DroppedEventCount:           0,
		PersistFailureCount:         int64(s.failedBlockedEventPersists.Load()),
		EvidenceEncryptionAvailable: s.evidenceCipher != nil && s.evidenceCipher.Available(),
	}
	if days, retentionErr := s.repository.GetEvidenceRetentionDays(ctx); retentionErr == nil {
		overview.EvidenceRetentionDays = days
	}
	if snapshot := s.snapshot.Load(); snapshot != nil {
		overview.Enabled = snapshot.Enabled
		overview.ConfigVersion = snapshot.ConfigVersion
		loadedAt := snapshot.LoadedAt
		overview.LoadedAt = &loadedAt
	}
	s.stateMu.RLock()
	overview.LoadError = s.lastLoadError
	s.stateMu.RUnlock()
	return overview, nil
}

func (s *InstructionService) ConfigVersion() int64 {
	if s == nil {
		return 0
	}
	if snapshot := s.snapshot.Load(); snapshot != nil {
		return snapshot.ConfigVersion
	}
	return 0
}

func (s *InstructionService) UpdateEnabled(ctx context.Context, request UpdateInstructionEnabledRequest) (*InstructionOverview, bool, error) {
	update, err := s.repository.SetEnabled(ctx, request.Enabled)
	if errors.Is(err, ErrInstructionAuditNoEffectiveGroupRules) {
		return nil, update.Before, infraerrors.Conflict("instruction_audit_no_effective_group_rules", "启用前必须至少配置一个包含有效哈希的分组规则")
	}
	if err != nil {
		return nil, update.Before, err
	}
	s.refreshAfterMutation(ctx, update.Version)
	overview, err := s.Overview(ctx)
	return overview, update.Before, err
}

func (s *InstructionService) ListHashes(ctx context.Context, status string) ([]InstructionHashEntry, error) {
	status = strings.TrimSpace(status)
	if status != "" && !validInstructionHashStatus(status) {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_hash_status", "哈希状态无效")
	}
	return s.repository.ListHashes(ctx, status)
}

func (s *InstructionService) CreateHash(ctx context.Context, request CreateInstructionHashRequest, actorID int64) (*InstructionHashEntry, error) {
	normalized, err := normalizeInstructionHashRequest(request)
	if err != nil {
		return nil, err
	}
	if existing, findErr := s.repository.FindHashByDigest(ctx, normalized.Digest); findErr == nil {
		return nil, infraerrors.Conflict("instruction_audit_hash_exists", fmt.Sprintf("哈希已存在：%s", existing.Name))
	} else if !errors.Is(findErr, sql.ErrNoRows) {
		return nil, findErr
	}
	item, err := s.repository.CreateHash(ctx, normalized, actorID)
	if err != nil {
		return nil, err
	}
	s.refreshAfterMutation(ctx, 0)
	return item, nil
}

func (s *InstructionService) UpdateHash(ctx context.Context, id int64, request UpdateInstructionHashRequest) (*InstructionHashEntry, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_hash_id", "哈希 ID 无效")
	}
	item, err := s.repository.GetHash(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("instruction_audit_hash_not_found", "哈希不存在")
	}
	if err != nil {
		return nil, err
	}
	if request.Name != nil {
		item.Name = strings.TrimSpace(*request.Name)
	}
	if request.Note != nil {
		item.Note = strings.TrimSpace(*request.Note)
	}
	if request.ObservedSource != nil {
		item.ObservedSource = strings.TrimSpace(*request.ObservedSource)
	}
	if request.ClientName != nil {
		item.ClientName = strings.TrimSpace(*request.ClientName)
	}
	if request.ClientVersion != nil {
		item.ClientVersion = strings.TrimSpace(*request.ClientVersion)
	}
	if request.Status != nil {
		item.Status = strings.TrimSpace(*request.Status)
	}
	if request.ClearValidFrom {
		item.ValidFrom = nil
	} else if request.ValidFrom != nil {
		item.ValidFrom = request.ValidFrom
	}
	if request.ClearValidUntil {
		item.ValidUntil = nil
	} else if request.ValidUntil != nil {
		item.ValidUntil = request.ValidUntil
	}
	if err := validateInstructionHash(*item); err != nil {
		return nil, err
	}
	item, err = s.repository.UpdateHash(ctx, *item)
	if err != nil {
		return nil, err
	}
	s.refreshAfterMutation(ctx, 0)
	return item, nil
}

func (s *InstructionService) ListRuleSets(ctx context.Context) ([]InstructionRuleSet, error) {
	return s.repository.ListRuleSets(ctx)
}

func (s *InstructionService) SaveRuleSet(ctx context.Context, id int64, request SaveInstructionRuleSetRequest, actorID int64) (*InstructionRuleSet, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	if request.Name == "" || len(request.Name) > 160 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_rule_set", "规则集名称不能为空且不能超过 160 个字符")
	}
	item, err := s.repository.SaveRuleSet(ctx, id, request, actorID)
	if err != nil {
		return nil, err
	}
	s.refreshAfterMutation(ctx, 0)
	return item, nil
}

func (s *InstructionService) ListGroupBindings(ctx context.Context) ([]InstructionGroupBinding, error) {
	return s.repository.ListGroupBindings(ctx)
}

func (s *InstructionService) SaveGroupBindings(ctx context.Context, request SaveInstructionGroupBindingsRequest, actorID int64) ([]InstructionGroupBinding, error) {
	request.GroupIDs = uniquePositiveInt64s(request.GroupIDs)
	if len(request.GroupIDs) == 0 || len(request.GroupIDs) > 500 || request.RuleSetID <= 0 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_group_binding", "分组和规则集必须有效，单次最多选择 500 个分组")
	}
	if request.ClientTypes != nil {
		clientTypes, err := normalizeInstructionClientTypes(request.ClientTypes)
		if err != nil {
			return nil, infraerrors.BadRequest("instruction_audit_invalid_client_scope", "客户端范围无效")
		}
		request.ClientTypes = clientTypes
	}
	items, err := s.repository.SaveGroupBindings(ctx, request, actorID)
	if err != nil {
		return nil, err
	}
	s.refreshAfterMutation(ctx, 0)
	return items, nil
}

func (s *InstructionService) DeleteGroupBinding(ctx context.Context, id int64) error {
	if id <= 0 {
		return infraerrors.BadRequest("instruction_audit_invalid_group_binding_id", "分组绑定 ID 无效")
	}
	if err := s.repository.DeleteGroupBinding(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return infraerrors.NotFound("instruction_audit_group_binding_not_found", "分组绑定不存在")
	} else if err != nil {
		return err
	}
	s.refreshAfterMutation(ctx, 0)
	return nil
}

func (s *InstructionService) ListGroupOptions(ctx context.Context) ([]InstructionGroupOption, error) {
	return s.repository.ListGroupOptions(ctx)
}

func (s *InstructionService) ListEvents(ctx context.Context, page, pageSize int, filter InstructionEventFilter) (*InstructionEventPage, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_page_size", "每页最多 100 条")
	}
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Model = strings.TrimSpace(filter.Model)
	if filter.UserID < 0 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_user_id", "用户 ID 无效")
	}
	filter.GroupIDs = uniquePositiveInt64s(filter.GroupIDs)
	filter.Reasons = normalizeInstructionFilterValues(filter.Reasons, validInstructionEventReasons)
	filter.InstructionResults = normalizeInstructionFilterValues(filter.InstructionResults, validInstructionFieldResults)
	filter.Input1Results = normalizeInstructionFilterValues(filter.Input1Results, validInstructionFieldResults)
	filter.UserNotifications = normalizeInstructionFilterValues(filter.UserNotifications, validSecurityNotificationStatuses)
	filter.OpsNotifications = normalizeInstructionFilterValues(filter.OpsNotifications, validSecurityNotificationStatuses)
	filter.ClientTypes = normalizeInstructionFilterValues(filter.ClientTypes, validInstructionDetectedClientTypeSet)
	if filter.GroupIDs == nil {
		filter.GroupIDs = []int64{}
	}
	if filter.Reasons == nil {
		filter.Reasons = []string{}
	}
	if filter.InstructionResults == nil {
		filter.InstructionResults = []string{}
	}
	if filter.Input1Results == nil {
		filter.Input1Results = []string{}
	}
	if filter.UserNotifications == nil {
		filter.UserNotifications = []string{}
	}
	if filter.OpsNotifications == nil {
		filter.OpsNotifications = []string{}
	}
	if filter.ClientTypes == nil {
		filter.ClientTypes = []string{}
	}
	return s.repository.ListEvents(ctx, page, pageSize, filter)
}

func (s *InstructionService) UpdateEvidenceRetention(ctx context.Context, days int) (*InstructionOverview, error) {
	if days < 1 || days > 3650 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_evidence_retention", "明文保留天数必须在 1 到 3650 之间")
	}
	if err := s.repository.SetEvidenceRetentionDays(ctx, days); err != nil {
		return nil, err
	}
	return s.Overview(ctx)
}

func (s *InstructionService) RevealEvidence(ctx context.Context, eventID int64, access InstructionEvidenceAccess) (*InstructionEvidenceReview, error) {
	event, err := s.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	review := &InstructionEvidenceReview{
		EventID: event.ID, RequestID: event.RequestID, Status: event.EvidenceStatus,
		ExpiresAt: event.EvidenceExpiresAt, Fields: []InstructionEvidenceField{},
	}
	if event.EvidenceStatus != "stored" {
		access.Action, access.Source, access.Succeeded, access.ErrorCode = "reveal", "all", false, event.EvidenceStatus
		_ = s.repository.RecordEvidenceAccess(ctx, eventID, access)
		review.AccessCount, _ = s.repository.CountEvidenceAccesses(ctx, eventID)
		return review, nil
	}
	if event.EvidenceExpiresAt != nil && !time.Now().UTC().Before(*event.EvidenceExpiresAt) {
		_, _ = s.repository.ExpireEvidence(ctx)
		review.Status = "expired"
		access.Action, access.Source, access.Succeeded, access.ErrorCode = "reveal", "all", false, "expired"
		_ = s.repository.RecordEvidenceAccess(ctx, eventID, access)
		review.AccessCount, _ = s.repository.CountEvidenceAccesses(ctx, eventID)
		return review, nil
	}
	items, err := s.repository.ListEvidence(ctx, eventID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		plaintext, decryptErr := s.evidenceCipher.Decrypt(item.Source, item.Digest, item.Ciphertext)
		fieldAccess := access
		fieldAccess.Action, fieldAccess.Source = "reveal", item.Source
		if decryptErr != nil {
			fieldAccess.Succeeded, fieldAccess.ErrorCode = false, "decrypt_failed"
			_ = s.repository.RecordEvidenceAccess(ctx, eventID, fieldAccess)
			return nil, errors.New("instruction evidence decryption failed")
		}
		recomputed := sha256Hex(plaintext)
		fieldAccess.Succeeded = true
		_ = s.repository.RecordEvidenceAccess(ctx, eventID, fieldAccess)
		review.Fields = append(review.Fields, InstructionEvidenceField{
			Source: item.Source, Available: true, Plaintext: plaintext, SHA256: item.Digest,
			PlaintextBytes: item.PlaintextBytes, RecomputedSHA256: recomputed,
			DigestConsistent: recomputed == item.Digest && len([]byte(plaintext)) == item.PlaintextBytes,
		})
	}
	review.AccessCount, _ = s.repository.CountEvidenceAccesses(ctx, eventID)
	return review, nil
}

func (s *InstructionService) RecordEvidenceCopy(ctx context.Context, eventID int64, access InstructionEvidenceAccess) error {
	if _, err := s.GetEvent(ctx, eventID); err != nil {
		return err
	}
	if !validInstructionEvidenceCopySource(access.Source) {
		return infraerrors.BadRequest("instruction_audit_invalid_copy_source", "复制内容类型无效")
	}
	access.Action, access.Succeeded = "copy", true
	return s.repository.RecordEvidenceAccess(ctx, eventID, access)
}

func (s *InstructionService) GetEvent(ctx context.Context, id int64) (*InstructionEvent, error) {
	item, err := s.repository.GetEvent(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("instruction_audit_event_not_found", "审核记录不存在")
	}
	return item, err
}

func (s *InstructionService) CreateCandidateFromEvent(ctx context.Context, eventID int64, request CreateInstructionCandidateRequest, actorID int64) (*InstructionHashEntry, error) {
	if !request.ReviewConfirmed {
		return nil, infraerrors.BadRequest("instruction_audit_review_confirmation_required", "请先审查并确认证据")
	}
	reviewed, err := s.repository.HasSuccessfulEvidenceReveal(ctx, eventID, actorID)
	if err != nil {
		return nil, err
	}
	if !reviewed {
		return nil, infraerrors.BadRequest("instruction_audit_evidence_review_required", "请先打开证据详情完成审查")
	}
	event, err := s.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	source := strings.TrimSpace(request.Source)
	digest := ""
	switch source {
	case "instructions":
		digest = event.Instructions.SHA256
	case "input1":
		digest = event.Input1.SHA256
	default:
		return nil, infraerrors.BadRequest("instruction_audit_invalid_candidate_source", "候选哈希来源无效")
	}
	if digest == "" {
		return nil, infraerrors.BadRequest("instruction_audit_candidate_digest_missing", "该字段没有可用摘要")
	}
	if existing, findErr := s.repository.FindHashByDigest(ctx, digest); findErr == nil {
		return existing, nil
	} else if !errors.Is(findErr, sql.ErrNoRows) {
		return nil, findErr
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = fmt.Sprintf("事件 #%d 候选", eventID)
	}
	return s.CreateHash(ctx, CreateInstructionHashRequest{
		Digest:         digest,
		Name:           name,
		Note:           strings.TrimSpace(request.Note),
		ObservedSource: source,
		ClientName:     strings.TrimSpace(request.ClientName),
		ClientVersion:  strings.TrimSpace(request.ClientVersion),
		Status:         "candidate",
	}, actorID)
}

func normalizeInstructionHashRequest(request CreateInstructionHashRequest) (CreateInstructionHashRequest, error) {
	request.Digest = strings.ToLower(strings.TrimSpace(request.Digest))
	request.Name = strings.TrimSpace(request.Name)
	request.Note = strings.TrimSpace(request.Note)
	request.ObservedSource = strings.TrimSpace(request.ObservedSource)
	request.ClientName = strings.TrimSpace(request.ClientName)
	request.ClientVersion = strings.TrimSpace(request.ClientVersion)
	request.Status = strings.TrimSpace(request.Status)
	if request.Status == "" {
		request.Status = "candidate"
	}
	if err := validateInstructionHash(InstructionHashEntry{
		Digest:         request.Digest,
		Name:           request.Name,
		Note:           request.Note,
		ObservedSource: request.ObservedSource,
		ClientName:     request.ClientName,
		ClientVersion:  request.ClientVersion,
		Status:         request.Status,
		ValidFrom:      request.ValidFrom,
		ValidUntil:     request.ValidUntil,
	}); err != nil {
		return CreateInstructionHashRequest{}, err
	}
	return request, nil
}

func validateInstructionHash(item InstructionHashEntry) error {
	if !instructionDigestPattern.MatchString(item.Digest) {
		return infraerrors.BadRequest("instruction_audit_invalid_digest", "SHA-256 必须是 64 位小写十六进制摘要")
	}
	if item.Name == "" || len(item.Name) > 160 {
		return infraerrors.BadRequest("instruction_audit_invalid_hash_name", "哈希名称不能为空且不能超过 160 个字符")
	}
	if item.ObservedSource != "" && item.ObservedSource != "instructions" && item.ObservedSource != "input1" {
		return infraerrors.BadRequest("instruction_audit_invalid_hash_source", "首次发现来源无效")
	}
	if len(item.ClientName) > 120 || len(item.ClientVersion) > 120 {
		return infraerrors.BadRequest("instruction_audit_invalid_client_metadata", "客户端名称和版本不能超过 120 个字符")
	}
	if !validInstructionHashStatus(item.Status) {
		return infraerrors.BadRequest("instruction_audit_invalid_hash_status", "哈希状态无效")
	}
	if item.ValidFrom != nil && item.ValidUntil != nil && !item.ValidUntil.After(*item.ValidFrom) {
		return infraerrors.BadRequest("instruction_audit_invalid_hash_validity", "失效时间必须晚于生效时间")
	}
	return nil
}

func validInstructionHashStatus(status string) bool {
	switch status {
	case "candidate", "active", "disabled", "expired":
		return true
	default:
		return false
	}
}

func (s *InstructionService) refreshAfterMutation(ctx context.Context, version int64) {
	minimumVersion := version
	if minimumVersion < 1 {
		if current := s.snapshot.Load(); current != nil {
			minimumVersion = current.ConfigVersion + 1
		}
	}
	if version < 1 && s.repository != nil {
		loadedVersion, err := s.repository.GetConfigVersion(ctx)
		if err == nil {
			version = loadedVersion
		}
	}
	if version < minimumVersion {
		version = minimumVersion
	}
	s.requireConfigVersion(version)
	if err := s.Reload(ctx); err != nil {
		slog.Warn("instruction_audit.local_reload_failed", "config_version", version, "error", err)
	}
	if s.redis != nil && version > 0 {
		if err := s.redis.Publish(ctx, InstructionConfigInvalidationChannel, strconv.FormatInt(version, 10)).Err(); err != nil {
			slog.Warn("instruction_audit.invalidation_publish_failed", "config_version", version, "error", err)
		}
	}
}

func (s *InstructionService) refreshLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reload(ctx); err != nil {
				slog.Warn("instruction_audit.periodic_reload_failed", "error", err)
			}
		}
	}
}

func (s *InstructionService) evidenceCleanupLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.repository.ExpireEvidence(ctx); err != nil {
				slog.Warn("instruction_audit.evidence_cleanup_failed", "error", err)
			}
		}
	}
}

func (s *InstructionService) subscribeLoop(ctx context.Context) {
	defer s.wg.Done()
	pubsub := s.redis.Subscribe(ctx, InstructionConfigInvalidationChannel)
	defer func() { _ = pubsub.Close() }()
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-channel:
			if !ok {
				return
			}
			version, err := strconv.ParseInt(strings.TrimSpace(message.Payload), 10, 64)
			if err != nil || version < 1 {
				slog.Warn("instruction_audit.invalidation_version_invalid", "payload_bytes", len(message.Payload))
			} else {
				s.requireConfigVersion(version)
			}
			if err := s.Reload(ctx); err != nil {
				slog.Warn("instruction_audit.invalidation_reload_failed", "error", err)
			}
		}
	}
}

func normalizeInstructionFilterValues(values []string, allowed map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if _, ok := allowed[value]; !ok {
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

func validInstructionEvidenceCopySource(source string) bool {
	switch source {
	case "instructions_plaintext", "instructions_hash", "input1_plaintext", "input1_hash", "request_id", "review_bundle":
		return true
	default:
		return false
	}
}

func (s *InstructionService) setLoadError(message string) {
	s.stateMu.Lock()
	s.lastLoadError = message
	s.stateMu.Unlock()
}
