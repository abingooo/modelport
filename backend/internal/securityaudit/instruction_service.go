package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
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

type InstructionService struct {
	repository *InstructionRepository
	redis      *redis.Client
	email      *service.EmailService

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
		email:      emailService,
	}
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
	if err := s.repository.ReclaimStaleOutbox(runCtx); err != nil {
		slog.Warn("instruction_audit.outbox_reclaim_failed", "error", err)
	}
	s.wg.Add(2)
	go s.refreshLoop(runCtx)
	go s.outboxLoop(runCtx)
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
	snapshot := s.snapshot.Load()
	if snapshot == nil {
		return s.blockInstructionConfigUnavailable(ctx, request, snapshot, startedAt)
	}
	if !snapshot.Enabled {
		return &InstructionDecision{Allow: true, ConfigVersion: snapshot.ConfigVersion}
	}
	if _, audited := snapshot.AuditedUsers[request.UserID]; !audited {
		return &InstructionDecision{Allow: true, ConfigVersion: snapshot.ConfigVersion}
	}
	preAuditModel := normalizeInstructionAuditModel(request.Model)

	body := request.InstructionBody
	if len(body) == 0 {
		body = request.Body
	}
	root, err := decodeStrictJSONObject(body)
	if err != nil {
		model, policy, applicable := "", instructionPolicy{}, false
		candidates := []string{preAuditModel}
		if !errors.Is(err, errInstructionAuditBodyTooLarge) {
			candidates = append(candidates, lenientLastInstructionModel(body))
		}
		for _, candidate := range candidates {
			candidate = normalizeInstructionAuditModel(candidate)
			if candidate == "" || candidate == model {
				continue
			}
			if candidatePolicy, ok := instructionPolicyFor(snapshot, request.UserID, candidate); ok {
				model = candidate
				policy = candidatePolicy
				applicable = true
				break
			}
		}
		if !applicable {
			return &InstructionDecision{Allow: true, ConfigVersion: snapshot.ConfigVersion}
		}
		request.Model = model
		if s.instructionSnapshotStale(snapshot, startedAt) {
			return s.blockInstructionConfigUnavailable(ctx, request, snapshot, startedAt)
		}
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

	model := preAuditModel
	if !request.InstructionModelOverride || model == "" {
		if strictModel, ok := strictInstructionModel(root); ok {
			model = strictModel
		}
	}
	policy, applicable := instructionPolicyFor(snapshot, request.UserID, model)
	if !applicable {
		return &InstructionDecision{Allow: true, ConfigVersion: snapshot.ConfigVersion}
	}
	request.Model = model
	if s.instructionSnapshotStale(snapshot, startedAt) {
		return s.blockInstructionConfigUnavailable(ctx, request, snapshot, startedAt)
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

func instructionPolicyFor(snapshot *instructionSnapshot, userID int64, model string) (instructionPolicy, bool) {
	if snapshot == nil {
		return instructionPolicy{}, false
	}
	models := snapshot.Policies[userID]
	if models == nil {
		return instructionPolicy{}, false
	}
	policy, ok := models[normalizeInstructionAuditModel(model)]
	return policy, ok
}

func normalizeInstructionAuditModel(model string) string {
	return strings.TrimSpace(model)
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
	if ctx == nil {
		ctx = context.Background()
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instructionBlockedEventPersistenceTimeout)
	defer cancel()
	if err := s.repository.RecordBlocked(recordCtx, event.Request, &event.Decision); err != nil {
		s.failedBlockedEventPersists.Add(1)
		slog.Error("instruction_audit.record_blocked_failed",
			"request_id", event.Request.RequestID,
			"user_id", event.Request.UserID,
			"api_key_id", event.Request.APIKeyID,
			"model", event.Request.Model,
			"reason", event.Decision.Reason,
			"error", err,
		)
	}
}

func (s *InstructionService) Overview(ctx context.Context) (*InstructionOverview, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("instruction audit service unavailable")
	}
	hashes, activeHashes, ruleSets, bindings, pendingEmails, err := s.repository.OverviewCounts(ctx)
	if err != nil {
		return nil, err
	}
	overview := &InstructionOverview{
		HashCount:           hashes,
		ActiveHashCount:     activeHashes,
		RuleSetCount:        ruleSets,
		ActiveBindingCount:  bindings,
		PendingEmailCount:   pendingEmails,
		QueuedEventCount:    0,
		DroppedEventCount:   0,
		PersistFailureCount: int64(s.failedBlockedEventPersists.Load()),
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
	update, err := s.repository.SetEnabled(ctx, request.Enabled, request.ConfirmNoRules)
	if errors.Is(err, ErrInstructionAuditConfirmationRequired) {
		return nil, update.Before, infraerrors.Conflict("instruction_audit_confirmation_required", "启用前没有有效规则，需要确认风险")
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

func (s *InstructionService) ListBindings(ctx context.Context) ([]InstructionBinding, error) {
	return s.repository.ListBindings(ctx)
}

func (s *InstructionService) SaveBinding(ctx context.Context, request CreateInstructionBindingRequest, actorID int64) (*InstructionBinding, error) {
	request.Model = normalizeInstructionAuditModel(request.Model)
	if request.UserID <= 0 || request.RuleSetID <= 0 || request.Model == "" || len(request.Model) > 255 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_binding", "用户、模型和规则集必须有效")
	}
	item, err := s.repository.SaveBinding(ctx, request, actorID)
	if err != nil {
		return nil, err
	}
	s.refreshAfterMutation(ctx, 0)
	return item, nil
}

func (s *InstructionService) DeleteBinding(ctx context.Context, id int64) error {
	if id <= 0 {
		return infraerrors.BadRequest("instruction_audit_invalid_binding_id", "绑定 ID 无效")
	}
	if err := s.repository.DeleteBinding(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return infraerrors.NotFound("instruction_audit_binding_not_found", "绑定不存在")
	} else if err != nil {
		return err
	}
	s.refreshAfterMutation(ctx, 0)
	return nil
}

func (s *InstructionService) SearchUsers(ctx context.Context, query string) ([]InstructionUserOption, error) {
	return s.repository.SearchUsers(ctx, query, 30)
}

func (s *InstructionService) ListEvents(ctx context.Context, page, pageSize int, userID int64, model string) (*InstructionEventPage, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_page_size", "每页最多 100 条")
	}
	return s.repository.ListEvents(ctx, page, pageSize, userID, model)
}

func (s *InstructionService) GetEvent(ctx context.Context, id int64) (*InstructionEvent, error) {
	item, err := s.repository.GetEvent(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("instruction_audit_event_not_found", "审核记录不存在")
	}
	return item, err
}

func (s *InstructionService) CreateCandidateFromEvent(ctx context.Context, eventID int64, request CreateInstructionCandidateRequest, actorID int64) (*InstructionHashEntry, error) {
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

func (s *InstructionService) outboxLoop(ctx context.Context) {
	defer s.wg.Done()
	poll := time.NewTicker(time.Second)
	reclaim := time.NewTicker(time.Minute)
	defer poll.Stop()
	defer reclaim.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reclaim.C:
			if err := s.repository.ReclaimStaleOutbox(ctx); err != nil {
				slog.Warn("instruction_audit.outbox_reclaim_failed", "error", err)
			}
		case <-poll.C:
			s.processOutbox(ctx)
		}
	}
}

func (s *InstructionService) processOutbox(ctx context.Context) {
	for processed := 0; processed < 20; processed++ {
		item, err := s.repository.ClaimOutbox(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			slog.Warn("instruction_audit.outbox_claim_failed", "error", err)
			return
		}
		sendErr := s.sendAdminNotification(ctx, item)
		if sendErr == nil {
			if err := s.repository.MarkOutboxSent(ctx, item.ID); err != nil {
				slog.Warn("instruction_audit.outbox_mark_sent_failed", "outbox_id", item.ID, "error", err)
			}
			continue
		}
		delay := instructionRetryDelay(item.Attempts)
		if err := s.repository.MarkOutboxFailed(ctx, *item, sendErr, delay); err != nil {
			slog.Warn("instruction_audit.outbox_mark_failed_failed", "outbox_id", item.ID, "error", err)
		}
	}
}

func (s *InstructionService) sendAdminNotification(ctx context.Context, outbox *instructionOutboxItem) error {
	if s.email == nil {
		return errors.New("email service unavailable")
	}
	if outbox == nil {
		return errors.New("instruction audit outbox item required")
	}
	event, err := s.repository.GetEvent(ctx, outbox.EventID)
	if err != nil {
		return err
	}
	recipients, err := s.repository.ListAdminRecipients(ctx)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return errors.New("no active administrator email recipient")
	}
	subject := fmt.Sprintf("[ModelPort] 安全策略拦截通知 #%d", event.ID)
	body := buildInstructionAuditEmail(event)
	var sendErrors []error
	for _, recipient := range recipients {
		if instructionRecipientAlreadySent(outbox.SentRecipientIDs, recipient.ID) {
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		sendErr := s.email.SendEmail(sendCtx, recipient.Email, subject, body)
		cancel()
		if sendErr != nil {
			sendErrors = append(sendErrors, fmt.Errorf("recipient %d: %w", recipient.ID, sendErr))
			continue
		}
		if err := s.repository.MarkOutboxRecipientSent(ctx, outbox.ID, recipient.ID); err != nil {
			return fmt.Errorf("record recipient %d delivery: %w", recipient.ID, err)
		}
		outbox.SentRecipientIDs = append(outbox.SentRecipientIDs, recipient.ID)
	}
	return errors.Join(sendErrors...)
}

func instructionRecipientAlreadySent(sent []int64, recipientID int64) bool {
	for _, id := range sent {
		if id == recipientID {
			return true
		}
	}
	return false
}

func buildInstructionAuditEmail(event *InstructionEvent) string {
	if event == nil {
		return "<p>ModelPort blocked a request under the configured security policy.</p>"
	}
	escape := html.EscapeString
	userID := "-"
	if event.UserID != nil {
		userID = strconv.FormatInt(*event.UserID, 10)
	}
	apiKeyID := "-"
	if event.APIKeyID != nil {
		apiKeyID = strconv.FormatInt(*event.APIKeyID, 10)
	}
	return fmt.Sprintf(`<h2>ModelPort 安全策略拦截通知</h2>
<table cellpadding="6" cellspacing="0" border="1" style="border-collapse:collapse">
<tr><td>请求 ID</td><td>%s</td></tr>
<tr><td>时间</td><td>%s</td></tr>
<tr><td>用户</td><td>%s / %s</td></tr>
<tr><td>API Key ID</td><td>%s</td></tr>
<tr><td>模型</td><td>%s</td></tr>
<tr><td>字段一</td><td>present=%t, sha256=%s, result=%s</td></tr>
<tr><td>字段二</td><td>present=%t, sha256=%s, result=%s</td></tr>
<tr><td>拒绝原因</td><td>%s</td></tr>
<tr><td>配置版本</td><td>%d</td></tr>
</table>`,
		escape(event.RequestID), escape(event.CreatedAt.UTC().Format(time.RFC3339)), escape(userID), escape(event.UserEmailSnapshot),
		escape(apiKeyID), escape(event.Model), event.Instructions.Present, escape(event.Instructions.SHA256), escape(event.Instructions.Result),
		event.Input1.Present, escape(event.Input1.SHA256), escape(event.Input1.Result), escape(event.Reason), event.ConfigVersion)
}

func instructionRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second * time.Duration(1<<min(attempt-1, 8))
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

func (s *InstructionService) setLoadError(message string) {
	s.stateMu.Lock()
	s.lastLoadError = message
	s.stateMu.Unlock()
}
