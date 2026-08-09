package securityaudit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

var (
	errInstructionV2ConfigConflict   = errors.New("instruction audit v2 configuration conflict")
	errInstructionV2ImmutableProfile = errors.New("immutable instruction audit v2 client profile")
	errInstructionV2BuiltInProfile   = errors.New("built-in instruction audit v2 client profile cannot be deleted")
	errInstructionV2ProfileInUse     = errors.New("instruction audit v2 client profile is in use")
	errInstructionV2RevokedHash      = errors.New("revoked instruction audit v2 hash cannot be reactivated")
)

const (
	instructionV2PersistenceTimeout  = 5 * time.Second
	instructionV2NotificationTimeout = 5 * time.Second
	instructionV2SnapshotMaxStale    = 30 * time.Second
	instructionV2AsyncMemoryMaximum  = int64(512 << 20)
)

type InstructionV2Service struct {
	repository      *InstructionV2Repository
	redis           *redis.Client
	evidenceCipher  *InstructionEvidenceCipher
	notifications   *service.SecurityNotificationService
	secretEncryptor service.SecretEncryptor
	reviewer        *InstructionV2AIReviewer
	httpMaxBody     int64
	wsMaxBody       int64

	snapshot        atomic.Pointer[instructionV2Snapshot]
	requiredVersion atomic.Int64
	asyncBytes      atomic.Int64
	persistFailures atomic.Int64
	reviewFlight    singleflight.Group

	asyncQueue chan instructionV2AsyncJob
	passQueue  chan InstructionV2Event

	reloadMu          sync.Mutex
	stateMu           sync.RWMutex
	lastLoadError     string
	lastLoadFailureAt time.Time

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func ProvideInstructionV2Service(
	repository *InstructionV2Repository,
	redisClient *redis.Client,
	evidenceCipher *InstructionEvidenceCipher,
	notifications *service.SecurityNotificationService,
	secretEncryptor service.SecretEncryptor,
	cfg *config.Config,
) *InstructionV2Service {
	result := &InstructionV2Service{
		repository: repository, redis: redisClient, evidenceCipher: evidenceCipher,
		notifications: notifications, secretEncryptor: secretEncryptor,
		reviewer:   NewInstructionV2AIReviewer(),
		asyncQueue: make(chan instructionV2AsyncJob, InstructionV2AsyncQueueCapacity),
		passQueue:  make(chan InstructionV2Event, InstructionV2AsyncQueueCapacity),
	}
	if cfg != nil {
		result.httpMaxBody = cfg.Gateway.MaxBodySize
		result.wsMaxBody = service.ResolveOpenAIWSClientReadLimitBytes(cfg)
	}
	return result
}

func (s *InstructionV2Service) Start(ctx context.Context) error {
	if s == nil || s.repository == nil {
		return errors.New("instruction audit v2 service unavailable")
	}
	s.lifecycleMu.Lock()
	if s.cancel != nil {
		s.lifecycleMu.Unlock()
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.lifecycleMu.Unlock()

	loadErr := s.Reload(runCtx)
	s.wg.Add(3 + InstructionV2AsyncWorkerMaximum)
	go s.refreshLoop(runCtx)
	go s.passEventWriter(runCtx)
	go s.maintenanceLoop(runCtx)
	for workerID := 0; workerID < InstructionV2AsyncWorkerMaximum; workerID++ {
		go s.observeWorker(runCtx)
	}
	if s.redis != nil {
		s.wg.Add(1)
		go s.subscribeLoop(runCtx)
	}
	return loadErr
}

func (s *InstructionV2Service) Shutdown(ctx context.Context) error {
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

func (s *InstructionV2Service) Reload(ctx context.Context) error {
	if s == nil || s.repository == nil {
		return errors.New("instruction audit v2 repository unavailable")
	}
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	snapshot, err := s.repository.LoadSnapshot(ctx, s.secretEncryptor)
	if err != nil {
		s.setLoadError(err)
		return err
	}
	if required := s.requiredVersion.Load(); snapshot.Config.ConfigVersion < required {
		err = fmt.Errorf("instruction audit v2 snapshot version %d is below required version %d", snapshot.Config.ConfigVersion, required)
		s.setLoadError(err)
		return err
	}
	reuseInstructionV2Semaphores(s.snapshot.Load(), snapshot)
	s.snapshot.Store(snapshot)
	s.stateMu.Lock()
	s.lastLoadError = ""
	s.lastLoadFailureAt = time.Time{}
	s.stateMu.Unlock()
	return nil
}

func reuseInstructionV2Semaphores(previous, next *instructionV2Snapshot) {
	if previous == nil || next == nil {
		return
	}
	if previous.GlobalSemaphore != nil && cap(previous.GlobalSemaphore) == cap(next.GlobalSemaphore) {
		next.GlobalSemaphore = previous.GlobalSemaphore
	}
	previousNodes := make(map[int64]*instructionV2AINodeRuntime, len(previous.AINodes))
	for _, node := range previous.AINodes {
		if node != nil {
			previousNodes[node.ID] = node
		}
	}
	for _, node := range next.AINodes {
		old := previousNodes[node.ID]
		if old != nil && old.semaphore != nil && cap(old.semaphore) == cap(node.semaphore) {
			node.semaphore = old.semaphore
		}
	}
}

func (s *InstructionV2Service) setLoadError(err error) {
	s.stateMu.Lock()
	if err != nil {
		s.lastLoadError = err.Error()
		if s.lastLoadFailureAt.IsZero() {
			s.lastLoadFailureAt = time.Now().UTC()
		}
	}
	s.stateMu.Unlock()
}

func (s *InstructionV2Service) ConfigVersion() int64 {
	if s == nil {
		return 0
	}
	if snapshot := s.snapshot.Load(); snapshot != nil {
		return snapshot.Config.ConfigVersion
	}
	return 0
}

func (s *InstructionV2Service) EvaluateInstruction(ctx context.Context, request Request) *InstructionDecision {
	if request.Protocol != instructionAuditProtocol || request.InstructionAuditExcluded {
		return &InstructionDecision{Allow: true}
	}
	snapshot := s.snapshot.Load()
	if snapshot == nil || snapshot.Config.EffectiveMode == InstructionV2ModeOff {
		return &InstructionDecision{Allow: true}
	}
	startedAt := time.Now()
	profile := classifyInstructionV2Client(snapshot, request.UserAgent, request.TrustedInternalClient)
	request.InstructionClientType = profile.profile.ProfileKey
	request.UserAgent = instructionUserAgentSnapshot(request.UserAgent)
	scopes := applicableInstructionV2Scopes(snapshot, instructionGroupID(request.GroupID), profile.profile.ProfileKey)
	if len(scopes) == 0 {
		return &InstructionDecision{Allow: true, ConfigVersion: snapshot.Config.ConfigVersion}
	}
	evaluation := instructionV2EvaluationContext{
		request: request, snapshot: snapshot, profile: profile, scopes: scopes,
		scope: scopes[0], bodyBytes: int64(len(instructionRequestBody(request))), startedAt: startedAt,
	}
	if s.snapshotStale(snapshot, startedAt) {
		return s.finishInstructionV2Unavailable(ctx, evaluation)
	}
	if _, allowed := snapshot.AllowedUsers[request.UserID]; allowed && request.UserID > 0 {
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeAllowlistPass, "user_allowlist")
		s.queuePassEvent(ctx, event)
		return s.allowInstructionV2Decision(evaluation, event, "user_allowlist")
	}

	fields, parseErr := parseInstructionV2Fields(ctx, instructionRequestBody(request))
	evaluation.fields = fields
	if parseErr != nil {
		return s.finishInstructionV2InvalidJSON(ctx, evaluation, parseErr)
	}
	if instructionV2FieldsEmpty(fields) {
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeEmptyPass, "fields_empty")
		s.queuePassEvent(ctx, event)
		return s.allowInstructionV2Decision(evaluation, event, "fields_empty")
	}
	if hash, matched := matchInstructionV2Field(snapshot, scopes, fields.Instructions); matched {
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeHashPass, "instructions_hash_match")
		event.MatchedHashID = &hash.ID
		s.queuePassEvent(ctx, event)
		return s.allowInstructionV2Decision(evaluation, event, "instructions_hash_match")
	}
	if hash, matched := matchInstructionV2Field(snapshot, scopes, fields.Input1); matched {
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeHashPass, "input1_hash_match")
		event.MatchedHashID = &hash.ID
		s.queuePassEvent(ctx, event)
		return s.allowInstructionV2Decision(evaluation, event, "input1_hash_match")
	}

	evaluation.fields.Instructions = prepareInstructionV2AISample(fields.Instructions, snapshot.Config.AIInputMaxChars)
	evaluation.fields.Input1 = prepareInstructionV2AISample(fields.Input1, snapshot.Config.AIInputMaxChars)
	if snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
		if s.enqueueObserveJob(evaluation) {
			return s.allowInstructionV2Decision(evaluation, InstructionV2Event{}, "observe_ai_queued")
		}
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeObserveAllow, "ai_queue_full")
		event.AIResult = "queue_full"
		s.persistNormalPass(ctx, event)
		return s.allowInstructionV2Decision(evaluation, event, "ai_queue_full")
	}
	return s.evaluateInstructionV2Enforced(ctx, evaluation)
}

func applicableInstructionV2Scopes(snapshot *instructionV2Snapshot, groupID int64, clientKey string) []instructionV2ScopeRuntime {
	if snapshot == nil || groupID <= 0 {
		return nil
	}
	available := snapshot.ScopesByGroup[groupID]
	result := make([]instructionV2ScopeRuntime, 0, len(available))
	for _, scope := range available {
		if scope.ClientProfileID == nil || scope.ClientProfileKey == clientKey {
			result = append(result, scope)
		}
	}
	return result
}

func matchInstructionV2Field(snapshot *instructionV2Snapshot, scopes []instructionV2ScopeRuntime, field InstructionV2Field) (instructionV2HashRuntime, bool) {
	if snapshot == nil || field.State != "valid" || field.SHA256 == "" {
		return instructionV2HashRuntime{}, false
	}
	hash, ok := snapshot.Hashes[field.SHA256]
	if !ok {
		return instructionV2HashRuntime{}, false
	}
	for _, scope := range scopes {
		if _, allowed := hash.ScopeIDs[scope.ID]; allowed {
			return hash, true
		}
	}
	return instructionV2HashRuntime{}, false
}

func instructionV2FieldsEmpty(fields instructionV2ParsedFields) bool {
	isEmpty := func(field InstructionV2Field) bool { return field.State == "missing" || field.State == "empty" }
	return isEmpty(fields.Instructions) && isEmpty(fields.Input1)
}

func (s *InstructionV2Service) snapshotStale(snapshot *instructionV2Snapshot, now time.Time) bool {
	if snapshot == nil {
		return true
	}
	s.stateMu.RLock()
	failureAt := s.lastLoadFailureAt
	s.stateMu.RUnlock()
	return !failureAt.IsZero() && now.Sub(failureAt) > instructionV2SnapshotMaxStale
}

func (s *InstructionV2Service) finishInstructionV2Unavailable(ctx context.Context, evaluation instructionV2EvaluationContext) *InstructionDecision {
	if evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeObserveAllow, "config_unavailable")
		s.persistNormalPass(ctx, event)
		return s.allowInstructionV2Decision(evaluation, event, "config_unavailable")
	}
	event := s.newInstructionV2Event(evaluation, "block", InstructionV2OutcomeBlocked, "config_unavailable")
	eventID, _ := s.persistCriticalEvent(ctx, instructionV2PersistEvent{Event: event})
	event.ID = eventID
	s.notifyInstructionV2Block(ctx, event)
	return s.blockInstructionV2Decision(evaluation, event, http.StatusForbidden, "config_unavailable")
}

func (s *InstructionV2Service) finishInstructionV2InvalidJSON(ctx context.Context, evaluation instructionV2EvaluationContext, parseErr error) *InstructionDecision {
	mode := evaluation.snapshot.Config.EffectiveMode
	decision := "block"
	outcome := InstructionV2OutcomeBlocked
	if mode == InstructionV2ModeObserve {
		decision = "allow"
		outcome = InstructionV2OutcomeObserveAllow
	}
	event := s.newInstructionV2Event(evaluation, decision, outcome, "invalid_json")
	eventID, persistErr := s.persistCriticalEvent(ctx, instructionV2PersistEvent{Event: event})
	if persistErr != nil {
		slog.Error("instruction_audit_v2.invalid_json_persist_failed", "request_id", evaluation.request.RequestID, "error", persistErr)
	}
	event.ID = eventID
	if mode == InstructionV2ModeObserve {
		return s.allowInstructionV2Decision(evaluation, event, "invalid_json")
	}
	s.notifyInstructionV2Block(ctx, event)
	decisionResult := s.blockInstructionV2Decision(evaluation, event, http.StatusBadRequest, "invalid_json")
	decisionResult.ErrorCode = "invalid_request_error"
	decisionResult.ClientMessage = "Invalid JSON request body."
	_ = parseErr
	return decisionResult
}

func (s *InstructionV2Service) evaluateInstructionV2Enforced(ctx context.Context, evaluation instructionV2EvaluationContext) *InstructionDecision {
	timeout := time.Duration(evaluation.snapshot.Config.AITotalTimeoutMS) * time.Millisecond
	reviewCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	outcome := s.reviewInstructionV2Fields(reviewCtx, evaluation)
	if outcome.Result == "pass" {
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeAIPass, "ai_pass")
		applyInstructionV2AIOutcome(&event, outcome)
		write, err := s.prepareInstructionV2AIWrite(evaluation, event, outcome)
		if err == nil {
			result, persistErr := s.persistCriticalEventWrite(ctx, write)
			if persistErr == nil {
				event.ID = result.EventID
				return s.allowInstructionV2Decision(evaluation, event, "ai_pass")
			}
			err = persistErr
		}
		slog.Error("instruction_audit_v2.ai_allow_transaction_failed", "request_id", evaluation.request.RequestID, "error", err)
		failure := s.newInstructionV2Event(evaluation, "block", InstructionV2OutcomeBlocked, "persistence_error")
		failure.AIResult = "error"
		failure.AILatencyMS = int(outcome.Latency.Milliseconds())
		failureID, _ := s.persistCriticalEvent(ctx, instructionV2PersistEvent{Event: failure, Reviews: outcome.Attempts})
		failure.ID = failureID
		s.notifyInstructionV2Block(ctx, failure)
		return s.blockInstructionV2Decision(evaluation, failure, http.StatusForbidden, "persistence_error")
	}

	reason := "ai_error"
	switch outcome.Result {
	case "reject":
		reason = "ai_rejected"
	case "uncertain":
		reason = "ai_uncertain"
	case "queue_full":
		reason = "ai_queue_full"
	}
	event := s.newInstructionV2Event(evaluation, "block", InstructionV2OutcomeBlocked, reason)
	applyInstructionV2AIOutcome(&event, outcome)
	write := instructionV2PersistEvent{Event: event, Reviews: outcome.Attempts}
	write.Evidence, write.Event.EvidenceStatus = s.prepareInstructionV2Evidence(evaluation, true)
	result, persistErr := s.persistCriticalEventWrite(ctx, write)
	if persistErr != nil {
		slog.Error("instruction_audit_v2.block_persist_failed", "request_id", evaluation.request.RequestID, "reason", reason, "error", persistErr)
	} else {
		event.ID = result.EventID
	}
	s.notifyInstructionV2Block(ctx, event)
	return s.blockInstructionV2Decision(evaluation, event, http.StatusForbidden, reason)
}

func applyInstructionV2AIOutcome(event *InstructionV2Event, outcome instructionV2AIOutcome) {
	if event == nil {
		return
	}
	event.AIResult = outcome.Result
	if event.AIResult == "" {
		event.AIResult = "error"
	}
	event.AIReviewedField = outcome.ReviewedField
	event.AISampled = outcome.ApprovedField.AISampled
	event.AILatencyMS = int(outcome.Latency.Milliseconds())
}

func (s *InstructionV2Service) reviewInstructionV2Fields(ctx context.Context, evaluation instructionV2EvaluationContext) instructionV2AIOutcome {
	key := instructionV2ReviewFlightKey(evaluation)
	resultChannel := s.reviewFlight.DoChan(key, func() (any, error) {
		timeout := time.Duration(evaluation.snapshot.Config.AITotalTimeoutMS) * time.Millisecond
		sharedCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return s.reviewInstructionV2FieldsShared(sharedCtx, evaluation), nil
	})
	select {
	case result := <-resultChannel:
		if result.Err != nil {
			return instructionV2AIOutcome{Result: "error"}
		}
		outcome, ok := result.Val.(instructionV2AIOutcome)
		if !ok {
			return instructionV2AIOutcome{Result: "error"}
		}
		return outcome
	case <-ctx.Done():
		return instructionV2AIOutcome{Result: "error"}
	}
}

func (s *InstructionV2Service) reviewInstructionV2FieldsShared(ctx context.Context, evaluation instructionV2EvaluationContext) instructionV2AIOutcome {
	startedAt := time.Now()
	wait := time.Duration(evaluation.snapshot.Config.AIQueueWaitMS) * time.Millisecond
	release, err := acquireInstructionV2Semaphore(ctx, evaluation.snapshot.GlobalSemaphore, wait)
	if err != nil {
		result := "error"
		if errors.Is(err, errInstructionV2AIQueueFull) {
			result = "queue_full"
		}
		return instructionV2AIOutcome{Result: result, Latency: time.Since(startedAt)}
	}
	defer release()

	fields := []struct {
		name  string
		field InstructionV2Field
	}{
		{name: "instructions", field: evaluation.fields.Instructions},
		{name: "input1", field: evaluation.fields.Input1},
	}
	allAttempts := make([]instructionV2AIAttempt, 0)
	hadUncertain := false
	hadReject := false
	hadError := false
	for _, item := range fields {
		if item.field.State != "valid" || item.field.SHA256 == "" || item.field.AISample == "" {
			continue
		}
		fieldOutcome := s.reviewInstructionV2Field(ctx, evaluation, item.name, item.field)
		allAttempts = append(allAttempts, fieldOutcome.Attempts...)
		switch fieldOutcome.Result {
		case "pass":
			fieldOutcome.Attempts = allAttempts
			fieldOutcome.Latency = time.Since(startedAt)
			return fieldOutcome
		case "reject":
			hadReject = true
		case "uncertain":
			hadUncertain = true
		default:
			hadError = true
		}
	}
	result := "reject"
	if hadError {
		result = "error"
	} else if hadUncertain {
		result = "uncertain"
	} else if !hadReject {
		result = "error"
	}
	return instructionV2AIOutcome{Result: result, Attempts: allAttempts, Latency: time.Since(startedAt)}
}

func (s *InstructionV2Service) reviewInstructionV2Field(ctx context.Context, evaluation instructionV2EvaluationContext, fieldName string, field InstructionV2Field) instructionV2AIOutcome {
	if cached, ok := s.loadInstructionV2AICache(ctx, evaluation, fieldName, field); ok {
		return instructionV2AIOutcome{
			Result: cached.Result, ReviewedField: fieldName, ApprovedField: field,
			Attempts: []instructionV2AIAttempt{cached},
		}
	}
	attempts := make([]instructionV2AIAttempt, 0, len(evaluation.snapshot.AINodes))
	hadUncertain := false
	for _, node := range evaluation.snapshot.AINodes {
		wait := time.Duration(evaluation.snapshot.Config.AIQueueWaitMS) * time.Millisecond
		release, err := acquireInstructionV2Semaphore(ctx, node.semaphore, wait)
		if err != nil {
			attempts = append(attempts, instructionV2AIAttempt{
				NodeID: instructionV2Int64Pointer(node.ID), NodeName: node.Name, ReviewerModel: node.Model,
				FieldName: fieldName, SHA256: field.SHA256, Result: "error", Reason: "node queue unavailable",
				Category: "technical_error", PromptVersion: evaluation.snapshot.PromptVersion, Sampled: field.AISampled,
			})
			continue
		}
		attemptStartedAt := time.Now()
		nodeTimeout := time.Duration(node.TimeoutMS) * time.Millisecond
		nodeCtx, cancel := context.WithTimeout(ctx, nodeTimeout)
		result, reviewErr := s.reviewer.Review(
			nodeCtx, node, evaluation.snapshot.Config.ReviewCriteria, evaluation.snapshot.PromptVersion,
			fieldName, field.AISample, field.AISampled,
		)
		cancel()
		release()
		attempt := instructionV2AIAttempt{
			NodeID: instructionV2Int64Pointer(node.ID), NodeName: node.Name, ReviewerModel: node.Model,
			FieldName: fieldName, SHA256: field.SHA256, PromptVersion: evaluation.snapshot.PromptVersion,
			Sampled: field.AISampled, LatencyMS: int(time.Since(attemptStartedAt).Milliseconds()),
		}
		if reviewErr != nil {
			attempt.Result, attempt.Reason, attempt.Category = "error", "AI review node unavailable", "technical_error"
			attempts = append(attempts, attempt)
			continue
		}
		attempt.Result, attempt.Confidence, attempt.Reason, attempt.Category = result.Result, result.Confidence, result.Reason, result.Category
		if attempt.Result == "pass" && attempt.Confidence < evaluation.snapshot.Config.ConfidenceThreshold {
			attempt.Result = "uncertain"
			attempt.Reason = "Confidence below configured threshold: " + attempt.Reason
		}
		attempts = append(attempts, attempt)
		switch attempt.Result {
		case "pass", "reject":
			s.storeInstructionV2AICache(ctx, evaluation, fieldName, field, attempt)
			return instructionV2AIOutcome{
				Result: attempt.Result, ReviewedField: fieldName, ApprovedField: field, Attempts: attempts,
			}
		case "uncertain":
			hadUncertain = true
		}
	}
	result := "error"
	if hadUncertain {
		result = "uncertain"
	}
	return instructionV2AIOutcome{Result: result, ReviewedField: fieldName, ApprovedField: field, Attempts: attempts}
}

func instructionV2ReviewFlightKey(evaluation instructionV2EvaluationContext) string {
	material := strings.Join([]string{
		evaluation.fields.Instructions.SHA256, evaluation.fields.Input1.SHA256,
		strconv.FormatInt(evaluation.scope.ID, 10), evaluation.profile.profile.ProfileKey,
		evaluation.snapshot.PromptVersion, strconv.FormatInt(evaluation.snapshot.Config.ConfigVersion, 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return hex.EncodeToString(digest[:])
}

func (s *InstructionV2Service) instructionV2AICacheKey(evaluation instructionV2EvaluationContext, fieldName string, field InstructionV2Field) string {
	material := strings.Join([]string{
		field.SHA256, fieldName, strconv.FormatInt(evaluation.scope.ID, 10),
		evaluation.profile.profile.ProfileKey, evaluation.snapshot.PromptVersion,
		strconv.FormatInt(evaluation.snapshot.Config.ConfigVersion, 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return "modelport:instruction-audit-v2:ai-cache:" + hex.EncodeToString(digest[:])
}

func (s *InstructionV2Service) loadInstructionV2AICache(ctx context.Context, evaluation instructionV2EvaluationContext, fieldName string, field InstructionV2Field) (instructionV2AIAttempt, bool) {
	if s.redis == nil || evaluation.snapshot.Config.AICacheTTLSeconds <= 0 {
		return instructionV2AIAttempt{}, false
	}
	raw, err := s.redis.Get(ctx, s.instructionV2AICacheKey(evaluation, fieldName, field)).Bytes()
	if err != nil {
		return instructionV2AIAttempt{}, false
	}
	var attempt instructionV2AIAttempt
	if json.Unmarshal(raw, &attempt) != nil || (attempt.Result != "pass" && attempt.Result != "reject") || attempt.SHA256 != field.SHA256 {
		return instructionV2AIAttempt{}, false
	}
	attempt.Cached = true
	attempt.LatencyMS = 0
	return attempt, true
}

func (s *InstructionV2Service) storeInstructionV2AICache(ctx context.Context, evaluation instructionV2EvaluationContext, fieldName string, field InstructionV2Field, attempt instructionV2AIAttempt) {
	if s.redis == nil || evaluation.snapshot.Config.AICacheTTLSeconds <= 0 || (attempt.Result != "pass" && attempt.Result != "reject") {
		return
	}
	raw, err := json.Marshal(attempt)
	if err != nil {
		return
	}
	_ = s.redis.Set(ctx, s.instructionV2AICacheKey(evaluation, fieldName, field), raw,
		time.Duration(evaluation.snapshot.Config.AICacheTTLSeconds)*time.Second).Err()
}

func (s *InstructionV2Service) newInstructionV2Event(evaluation instructionV2EvaluationContext, decision, outcome, reason string) InstructionV2Event {
	request := evaluation.request
	event := InstructionV2Event{
		RequestID: request.RequestID, UserEmail: request.UserEmail, APIKeyName: request.APIKeyName,
		GroupName: request.GroupName, ClientKey: evaluation.profile.profile.ProfileKey,
		ClientName: evaluation.profile.profile.Name, ClientUserAgent: request.UserAgent,
		Model: request.Model, Endpoint: request.Endpoint, Stage: request.Stage,
		Mode: evaluation.snapshot.Config.EffectiveMode, Decision: decision, Outcome: outcome, Reason: reason,
		Instructions: evaluation.fields.Instructions, Input1: evaluation.fields.Input1,
		AIResult: "not_run", BodyBytes: evaluation.bodyBytes,
		ConfigVersion: evaluation.snapshot.Config.ConfigVersion, EvidenceStatus: "not_stored",
		AuditLatencyMS: int(time.Since(evaluation.startedAt).Milliseconds()),
	}
	if request.UserID > 0 {
		event.UserID = instructionV2Int64Pointer(request.UserID)
	}
	if request.APIKeyID > 0 {
		event.APIKeyID = instructionV2Int64Pointer(request.APIKeyID)
	}
	if request.GroupID != nil {
		event.GroupID = instructionV2Int64Pointer(*request.GroupID)
	}
	if evaluation.scope.ID > 0 {
		event.ScopeID = instructionV2Int64Pointer(evaluation.scope.ID)
	}
	if evaluation.profile.profile.ID > 0 {
		event.ClientProfileID = instructionV2Int64Pointer(evaluation.profile.profile.ID)
	}
	return event
}

func (s *InstructionV2Service) allowInstructionV2Decision(evaluation instructionV2EvaluationContext, event InstructionV2Event, reason string) *InstructionDecision {
	return &InstructionDecision{
		EventID: event.ID, Applicable: true, Allow: true, Reason: reason,
		InitialReason: reason, FinalReason: reason, FinalOutcome: event.Outcome,
		PolicyAction: "allow", Instructions: instructionV2LegacyField(evaluation.fields.Instructions),
		Input1:        instructionV2LegacyField(evaluation.fields.Input1),
		ConfigVersion: evaluation.snapshot.Config.ConfigVersion,
		BodyBytes:     evaluation.bodyBytes,
		Latency:       time.Since(evaluation.startedAt),
	}
}

func (s *InstructionV2Service) blockInstructionV2Decision(evaluation instructionV2EvaluationContext, event InstructionV2Event, status int, reason string) *InstructionDecision {
	return &InstructionDecision{
		EventID: event.ID, HTTPStatus: status, ErrorCode: InstructionErrorCodeRejected,
		ClientMessage: InstructionClientMessage, Applicable: true, Allow: false, Reason: reason,
		InitialReason: reason, FinalReason: reason, FinalOutcome: InstructionV2OutcomeBlocked,
		PolicyAction: "block", Instructions: instructionV2LegacyField(evaluation.fields.Instructions),
		Input1:        instructionV2LegacyField(evaluation.fields.Input1),
		ConfigVersion: evaluation.snapshot.Config.ConfigVersion,
		BodyBytes:     evaluation.bodyBytes,
		Latency:       time.Since(evaluation.startedAt), AILatency: time.Duration(event.AILatencyMS) * time.Millisecond,
	}
}

func instructionV2LegacyField(field InstructionV2Field) InstructionFieldResult {
	result := field.State
	if result == "valid" {
		result = "mismatch"
	}
	return InstructionFieldResult{
		Present: field.State != "missing" && field.State != "not_checked",
		SHA256:  field.SHA256, Result: result,
	}
}

func (s *InstructionV2Service) prepareInstructionV2AIWrite(evaluation instructionV2EvaluationContext, event InstructionV2Event, outcome instructionV2AIOutcome) (instructionV2PersistEvent, error) {
	evidence, evidenceStatus := s.prepareInstructionV2Evidence(evaluation, true)
	event.EvidenceStatus = evidenceStatus
	candidate, err := s.prepareInstructionV2Candidate(evaluation, outcome)
	if err != nil {
		return instructionV2PersistEvent{}, err
	}
	return instructionV2PersistEvent{Event: event, Evidence: evidence, Reviews: outcome.Attempts, Candidate: candidate}, nil
}

func (s *InstructionV2Service) prepareInstructionV2Candidate(evaluation instructionV2EvaluationContext, outcome instructionV2AIOutcome) (*instructionV2CandidateWrite, error) {
	field := outcome.ApprovedField
	if field.State != "valid" || field.SHA256 == "" || evaluation.scope.ID <= 0 {
		return nil, errors.New("instruction audit v2 AI candidate is incomplete")
	}
	if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
		return nil, errInstructionEvidenceEncryptionUnavailable
	}
	plaintext := field.Plaintext
	storage := "full"
	if field.Bytes > int64(evaluation.snapshot.Config.RawFullMaxBytes) {
		plaintext = field.AISample
		storage = "sample"
	}
	if plaintext == "" {
		return nil, errors.New("instruction audit v2 AI candidate raw content unavailable")
	}
	ciphertext, err := s.evidenceCipher.EncryptHashRaw(field.SHA256, plaintext)
	if err != nil {
		return nil, err
	}
	var approved instructionV2AIAttempt
	for index := len(outcome.Attempts) - 1; index >= 0; index-- {
		if outcome.Attempts[index].Result == "pass" {
			approved = outcome.Attempts[index]
			break
		}
	}
	return &instructionV2CandidateWrite{
		SHA256: field.SHA256, Name: "AI 候选 " + field.SHA256[:12],
		Note:          "由指令审核 AI 二审生成，需管理员确认后才会成为可信指令。",
		ObservedField: outcome.ReviewedField, ContentBytes: field.Bytes,
		RawStorage: storage, RawCiphertext: ciphertext, StoredBytes: len([]byte(plaintext)),
		AISampled: field.AISampled, ScopeID: evaluation.scope.ID,
		ReviewerNodeID: approved.NodeID, ReviewerModel: approved.ReviewerModel,
		PromptVersion: approved.PromptVersion, Confidence: approved.Confidence,
		ReviewReason: approved.Reason, ReviewCategory: approved.Category,
		CandidateExpiresAt: time.Now().UTC().Add(time.Duration(evaluation.snapshot.Config.CandidateRetentionDays) * 24 * time.Hour),
	}, nil
}

func (s *InstructionV2Service) prepareInstructionV2Evidence(evaluation instructionV2EvaluationContext, needed bool) ([]instructionV2EvidenceWrite, string) {
	if !needed {
		return nil, "not_stored"
	}
	if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
		return nil, "encryption_unavailable"
	}
	fields := []struct {
		name  string
		field InstructionV2Field
	}{
		{name: "instructions", field: evaluation.fields.Instructions},
		{name: "input1", field: evaluation.fields.Input1},
	}
	writes := make([]instructionV2EvidenceWrite, 0, 2)
	partial := false
	for _, item := range fields {
		if item.field.State != "valid" || item.field.SHA256 == "" {
			continue
		}
		plaintext := item.field.Plaintext
		storage := "full"
		if item.field.Bytes > int64(evaluation.snapshot.Config.RawFullMaxBytes) {
			prepared := prepareInstructionV2AISample(item.field, evaluation.snapshot.Config.AIInputMaxChars)
			plaintext = prepared.AISample
			storage = "sample"
			partial = true
		}
		if plaintext == "" {
			continue
		}
		ciphertext, err := s.evidenceCipher.Encrypt(item.name, item.field.SHA256, plaintext)
		if err != nil {
			return nil, "encryption_unavailable"
		}
		writes = append(writes, instructionV2EvidenceWrite{
			FieldName: item.name, SHA256: item.field.SHA256, StorageKind: storage,
			Ciphertext: ciphertext, ContentBytes: item.field.Bytes, StoredBytes: len([]byte(plaintext)),
			ExpiresAt: time.Now().UTC().Add(time.Duration(evaluation.snapshot.Config.EvidenceRetentionDays) * 24 * time.Hour),
		})
	}
	if len(writes) == 0 {
		return nil, "not_stored"
	}
	if partial {
		return writes, "partial"
	}
	return writes, "stored"
}

func (s *InstructionV2Service) persistCriticalEvent(ctx context.Context, write instructionV2PersistEvent) (int64, error) {
	result, err := s.persistCriticalEventWrite(ctx, write)
	return result.EventID, err
}

func (s *InstructionV2Service) persistCriticalEventWrite(ctx context.Context, write instructionV2PersistEvent) (instructionV2PersistResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instructionV2PersistenceTimeout)
	defer cancel()
	result, err := s.repository.PersistInstructionV2Event(persistCtx, write)
	if err != nil {
		s.persistFailures.Add(1)
	}
	return result, err
}

func (s *InstructionV2Service) queuePassEvent(ctx context.Context, event InstructionV2Event) {
	event.Instructions.Plaintext, event.Instructions.AISample = "", ""
	event.Input1.Plaintext, event.Input1.AISample = "", ""
	select {
	case s.passQueue <- event:
	default:
		s.persistNormalPass(ctx, event)
	}
}

func (s *InstructionV2Service) persistNormalPass(ctx context.Context, event InstructionV2Event) {
	if _, err := s.persistCriticalEvent(ctx, instructionV2PersistEvent{Event: event}); err != nil {
		slog.Error("instruction_audit_v2.pass_persist_failed", "request_id", event.RequestID, "error", err)
	}
}

func (s *InstructionV2Service) enqueueObserveJob(evaluation instructionV2EvaluationContext) bool {
	request := evaluation.request
	request.Body, request.InstructionBody = nil, nil
	job := instructionV2AsyncJob{
		request: request.Clone(), snapshot: evaluation.snapshot, profile: evaluation.profile,
		scopes: append([]instructionV2ScopeRuntime(nil), evaluation.scopes...),
		fields: evaluation.fields, bodyBytes: evaluation.bodyBytes, startedAt: evaluation.startedAt,
	}
	job.fields.Instructions = compactInstructionV2AsyncField(job.fields.Instructions, evaluation.snapshot.Config.RawFullMaxBytes)
	job.fields.Input1 = compactInstructionV2AsyncField(job.fields.Input1, evaluation.snapshot.Config.RawFullMaxBytes)
	job.weight = int64(len(job.fields.Instructions.Plaintext) + len(job.fields.Instructions.AISample) + len(job.fields.Input1.Plaintext) + len(job.fields.Input1.AISample))
	if !s.reserveInstructionV2AsyncBytes(job.weight) {
		return false
	}
	select {
	case s.asyncQueue <- job:
		return true
	default:
		s.asyncBytes.Add(-job.weight)
		return false
	}
}

func compactInstructionV2AsyncField(field InstructionV2Field, rawMaximum int) InstructionV2Field {
	if field.Bytes > int64(rawMaximum) {
		field.Plaintext = ""
	}
	return field
}

func (s *InstructionV2Service) reserveInstructionV2AsyncBytes(weight int64) bool {
	if weight < 0 || weight > instructionV2AsyncMemoryMaximum {
		return false
	}
	for {
		current := s.asyncBytes.Load()
		if current > instructionV2AsyncMemoryMaximum-weight {
			return false
		}
		if s.asyncBytes.CompareAndSwap(current, current+weight) {
			return true
		}
	}
}

func (s *InstructionV2Service) observeWorker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.asyncQueue:
			s.processObserveJob(ctx, job)
			s.asyncBytes.Add(-job.weight)
		}
	}
}

func (s *InstructionV2Service) processObserveJob(ctx context.Context, job instructionV2AsyncJob) {
	evaluation := instructionV2EvaluationContext{
		request: job.request, snapshot: job.snapshot, profile: job.profile,
		scopes: job.scopes, scope: job.scopes[0], fields: job.fields,
		bodyBytes: job.bodyBytes, startedAt: job.startedAt,
	}
	timeout := time.Duration(job.snapshot.Config.AITotalTimeoutMS) * time.Millisecond
	reviewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	outcome := s.reviewInstructionV2Fields(reviewCtx, evaluation)
	cancel()
	event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeObserveAllow, "ai_"+outcome.Result)
	applyInstructionV2AIOutcome(&event, outcome)
	if outcome.Result == "pass" {
		event.Outcome, event.Reason = InstructionV2OutcomeAIPass, "ai_pass"
		write, err := s.prepareInstructionV2AIWrite(evaluation, event, outcome)
		if err == nil {
			if _, err = s.persistCriticalEventWrite(ctx, write); err == nil {
				return
			}
		}
		event.Outcome, event.Reason, event.AIResult = InstructionV2OutcomeObserveAllow, "ai_error", "error"
	}
	write := instructionV2PersistEvent{Event: event, Reviews: outcome.Attempts}
	write.Evidence, write.Event.EvidenceStatus = s.prepareInstructionV2Evidence(evaluation, true)
	if _, err := s.persistCriticalEventWrite(ctx, write); err != nil {
		slog.Error("instruction_audit_v2.observe_event_persist_failed", "request_id", job.request.RequestID, "error", err)
	}
}

func (s *InstructionV2Service) passEventWriter(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(InstructionV2EventBatchFlushInterval)
	defer ticker.Stop()
	batch := make([]InstructionV2Event, 0, InstructionV2EventBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		persistCtx, cancel := context.WithTimeout(context.Background(), instructionV2PersistenceTimeout)
		err := s.repository.PersistInstructionV2Events(persistCtx, batch)
		cancel()
		if err != nil {
			s.persistFailures.Add(int64(len(batch)))
			slog.Error("instruction_audit_v2.pass_batch_persist_failed", "count", len(batch), "error", err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case event := <-s.passQueue:
					batch = append(batch, event)
					if len(batch) >= InstructionV2EventBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case event := <-s.passQueue:
			batch = append(batch, event)
			if len(batch) >= InstructionV2EventBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (s *InstructionV2Service) notifyInstructionV2Block(ctx context.Context, event InstructionV2Event) {
	if s.notifications == nil || event.ID <= 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instructionV2NotificationTimeout)
	defer cancel()
	digest := event.Instructions.SHA256
	if event.AIReviewedField == "input1" || digest == "" {
		digest = event.Input1.SHA256
	}
	if digest == "" {
		digest = event.Reason
	}
	variables := map[string]string{
		"event_id": strconv.FormatInt(event.ID, 10), "request_id": event.RequestID,
		"triggered_at": time.Now().UTC().Format(time.RFC3339), "user_id": strconv.FormatInt(pointerInt64Value(event.UserID), 10),
		"user_email": event.UserEmail, "api_key_id": strconv.FormatInt(pointerInt64Value(event.APIKeyID), 10),
		"group_id": strconv.FormatInt(pointerInt64Value(event.GroupID), 10), "group_name": event.GroupName,
		"client_type": event.ClientKey, "model": event.Model, "admin_qq": "2145236436",
		"initial_reason": event.Reason, "final_reason": event.Reason, "final_outcome": event.Outcome,
		"policy_action": "block", "config_version": strconv.FormatInt(event.ConfigVersion, 10),
		"instructions_present": strconv.FormatBool(event.Instructions.State != "missing"),
		"instructions_result":  event.Instructions.State, "instructions_sha256": event.Instructions.SHA256,
		"input1_present": strconv.FormatBool(event.Input1.State != "missing"),
		"input1_result":  event.Input1.State, "input1_sha256": event.Input1.SHA256,
	}
	if err := s.notifications.Enqueue(notifyCtx, service.SecurityNotificationEnqueueInput{
		SourceType: service.SecurityNotificationSourceInstructionAuditV2, SourceID: event.ID,
		UserID: pointerInt64Value(event.UserID), UserEmail: event.UserEmail,
		DedupeScope:  fmt.Sprintf("%d:%s", pointerInt64Value(event.UserID), digest),
		DedupeWindow: 10 * time.Minute,
		UserTemplate: service.NotificationEmailEventInstructionAuditUserNotice,
		OpsTemplate:  service.NotificationEmailEventInstructionAuditOpsNotice,
		Variables:    variables,
	}); err != nil {
		slog.Warn("instruction_audit_v2.notification_enqueue_failed", "event_id", event.ID, "error", err)
	}
}

func (s *InstructionV2Service) refreshLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(InstructionV2ConfigurationRefreshPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reload(ctx); err != nil {
				slog.Warn("instruction_audit_v2.config_reload_failed", "error", err)
			}
		}
	}
}

func (s *InstructionV2Service) subscribeLoop(ctx context.Context) {
	defer s.wg.Done()
	pubsub := s.redis.Subscribe(ctx, InstructionV2ConfigInvalidationChannel)
	defer func() { _ = pubsub.Close() }()
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-channel:
			if !ok {
				return
			}
			if err := s.Reload(ctx); err != nil {
				slog.Warn("instruction_audit_v2.invalidation_reload_failed", "error", err)
			}
		}
	}
}

func (s *InstructionV2Service) maintenanceLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if snapshot := s.snapshot.Load(); snapshot != nil {
				maintenanceCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				err := s.repository.Cleanup(maintenanceCtx, snapshot.Config)
				cancel()
				if err != nil {
					slog.Warn("instruction_audit_v2.maintenance_failed", "error", err)
				}
			}
		}
	}
}

func (s *InstructionV2Service) refreshAfterMutation(ctx context.Context, version int64) {
	if version > 0 {
		s.requiredVersion.Store(version)
	}
	if err := s.Reload(ctx); err != nil {
		slog.Warn("instruction_audit_v2.post_mutation_reload_failed", "version", version, "error", err)
	}
	if s.redis != nil {
		_ = s.redis.Publish(context.WithoutCancel(ctx), InstructionV2ConfigInvalidationChannel, strconv.FormatInt(version, 10)).Err()
	}
}

func instructionV2Int64Pointer(value int64) *int64 {
	copyValue := value
	return &copyValue
}

func (s *InstructionV2Service) TestAINode(ctx context.Context, id int64) (instructionV2AIResult, time.Duration, error) {
	snapshot := s.snapshot.Load()
	if snapshot == nil {
		return instructionV2AIResult{}, 0, errors.New("instruction audit v2 configuration unavailable")
	}
	for _, node := range snapshot.AINodes {
		if node.ID != id {
			continue
		}
		startedAt := time.Now()
		testCtx, cancel := context.WithTimeout(ctx, time.Duration(node.TimeoutMS)*time.Millisecond)
		defer cancel()
		result, err := s.reviewer.Review(testCtx, node, snapshot.Config.ReviewCriteria, snapshot.PromptVersion,
			"instructions", "You are a coding assistant. Follow the user's request safely.", false)
		return result, time.Since(startedAt), err
	}
	return instructionV2AIResult{}, 0, sql.ErrNoRows
}
