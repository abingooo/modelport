package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var (
	errInstructionV2ConfigConflict      = errors.New("instruction audit v2 configuration conflict")
	errInstructionV2BuiltInProfile      = errors.New("built-in instruction audit v2 client profile cannot be deleted")
	errInstructionV2ProfileInUse        = errors.New("instruction audit v2 client profile is in use")
	errInstructionV2InvalidScopeProfile = errors.New("instruction audit v2 scope references an invalid client profile")
	errInstructionV2AINodeSlotInUse     = errors.New("instruction audit v2 AI node slot is in use")
	errInstructionV2RevokedHash         = errors.New("revoked instruction audit v2 hash cannot be reactivated")
	errInstructionV2ReviewLeaseLost     = errors.New("instruction audit v2 review lease lost")
)

const (
	instructionV2PersistenceTimeout  = 5 * time.Second
	instructionV2NotificationTimeout = 5 * time.Second
	instructionV2SnapshotMaxStale    = 30 * time.Second
)

type InstructionV2Service struct {
	repository        *InstructionV2Repository
	redis             *redis.Client
	evidenceCipher    *InstructionEvidenceCipher
	notifications     *service.SecurityNotificationService
	secretEncryptor   service.SecretEncryptor
	reviewer          instructionV2Reviewer
	httpMaxBody       int64
	wsMaxBody         int64
	requestBodyBudget *pkghttputil.RequestBodyMemoryBudget

	snapshot        atomic.Pointer[instructionV2Snapshot]
	requiredVersion atomic.Int64
	persistFailures atomic.Int64
	passQueue       chan InstructionV2Event

	reloadMu          sync.Mutex
	stateMu           sync.RWMutex
	lastLoadError     string
	lastLoadFailureAt time.Time

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

var _ InstructionRequestBodyProvider = (*InstructionV2Service)(nil)

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
		reviewer:  NewInstructionV2AIReviewer(),
		passQueue: make(chan InstructionV2Event, InstructionV2AsyncQueueCapacity),
	}
	if cfg != nil {
		result.httpMaxBody = cfg.Gateway.MaxBodySize
		result.wsMaxBody = service.ResolveOpenAIWSClientReadLimitBytes(cfg)
	}
	result.requestBodyBudget = pkghttputil.NewRequestBodyMemoryBudget(
		instructionV2RequestBodyBudgetCapacity(result.httpMaxBody, result.wsMaxBody),
	)
	return result
}

func instructionV2RequestBodyBudgetCapacity(httpMaxBody, wsMaxBody int64) int64 {
	maxBody := max(httpMaxBody, wsMaxBody)
	capacity := InstructionDefaultMaxInflightBodyBytes
	workingSet, err := pkghttputil.RequestBodyWorkingSetBytes(maxBody, 2)
	if err == nil && workingSet > capacity {
		capacity = workingSet
	}
	return capacity
}

func (s *InstructionV2Service) RequestBodyMemoryBudget() *pkghttputil.RequestBodyMemoryBudget {
	if s == nil {
		return nil
	}
	return s.requestBodyBudget
}

// V2 uses each gateway's existing HTTP or WebSocket body limit. The shared
// budget above bounds aggregate memory without introducing a second size cap.
func (*InstructionV2Service) RequestBodyReadLimit() int64 {
	return 0
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
	s.wg.Add(3 + InstructionV2ReviewWorkerCount)
	go s.refreshLoop(runCtx)
	go s.passEventWriter(runCtx)
	go s.maintenanceLoop(runCtx)
	for workerID := 0; workerID < InstructionV2ReviewWorkerCount; workerID++ {
		go s.reviewWorker(runCtx, workerID)
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
	if !InstructionV2RouteAllowed(request) {
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
		return s.finishInstructionV2InvalidJSON(ctx, evaluation)
	}
	evaluation.selectedFieldName, evaluation.selectedField = selectInstructionV2Field(fields)
	if evaluation.selectedFieldName == "" {
		if snapshot.Config.AllowEmptyFields || snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
			event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeEmptyPass, "fields_empty")
			s.queuePassEvent(ctx, event)
			return s.allowInstructionV2Decision(evaluation, event, "fields_empty")
		}
		event := s.newInstructionV2Event(evaluation, "block", InstructionV2OutcomeBlocked, "fields_empty")
		eventID, _ := s.persistCriticalEvent(ctx, instructionV2PersistEvent{Event: event})
		event.ID = eventID
		s.notifyInstructionV2Ops(ctx, event)
		return s.blockInstructionV2Decision(evaluation, event, "fields_empty")
	}

	digest := evaluation.selectedField.SHA256
	if risk, matched := snapshot.RiskHashes[digest]; matched {
		decision, outcome := "block", InstructionV2OutcomeRiskBlocked
		if snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
			decision, outcome = "allow", InstructionV2OutcomeObserveAllow
		}
		event := s.newInstructionV2Event(evaluation, decision, outcome, "risk_hash_match")
		event.AIResult = "not_run"
		eventID, _ := s.persistCriticalEvent(ctx, instructionV2PersistEvent{Event: event})
		event.ID = eventID
		_ = risk
		if decision == "allow" {
			return s.allowInstructionV2Decision(evaluation, event, "risk_hash_match")
		}
		s.notifyInstructionV2Ops(ctx, event)
		return s.blockInstructionV2Decision(evaluation, event, "risk_hash_match")
	}
	if hash, matched := matchInstructionV2Field(snapshot, scopes, evaluation.selectedField); matched {
		reason := "scoped_trusted_hash_match"
		if hash.Global {
			reason = "global_trusted_hash_match"
		}
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeHashPass, reason)
		event.MatchedHashID = &hash.ID
		s.queuePassEvent(ctx, event)
		return s.allowInstructionV2Decision(evaluation, event, reason)
	}
	if decision, reused := s.evaluateExistingInstructionV2Review(ctx, evaluation); reused {
		return decision
	}
	return s.evaluateInstructionV2AI(ctx, evaluation)
}

func selectInstructionV2Field(fields instructionV2ParsedFields) (string, InstructionV2Field) {
	if fields.Instructions.State == "valid" && fields.Instructions.SHA256 != "" && fields.Instructions.Plaintext != "" {
		return "instructions", fields.Instructions
	}
	if fields.Input1.State == "valid" && fields.Input1.SHA256 != "" && fields.Input1.Plaintext != "" {
		return "input1", fields.Input1
	}
	return "", InstructionV2Field{State: "empty"}
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
	if hash.Global {
		return hash, true
	}
	for _, scope := range scopes {
		if _, allowed := hash.ScopeIDs[scope.ID]; allowed {
			return hash, true
		}
	}
	return instructionV2HashRuntime{}, false
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
	s.notifyInstructionV2Ops(ctx, event)
	return s.blockInstructionV2Decision(evaluation, event, "config_unavailable")
}

func (s *InstructionV2Service) finishInstructionV2InvalidJSON(ctx context.Context, evaluation instructionV2EvaluationContext) *InstructionDecision {
	if evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
		event := s.newInstructionV2Event(evaluation, "allow", InstructionV2OutcomeObserveAllow, "invalid_json")
		s.persistNormalPass(ctx, event)
		return s.allowInstructionV2Decision(evaluation, event, "invalid_json")
	}
	event := s.newInstructionV2Event(evaluation, "block", InstructionV2OutcomeBlocked, "invalid_json")
	eventID, persistErr := s.persistCriticalEvent(ctx, instructionV2PersistEvent{Event: event})
	if persistErr != nil {
		slog.Error("instruction_audit_v2.invalid_json_persist_failed", "request_id", evaluation.request.RequestID, "error", persistErr)
	}
	event.ID = eventID
	s.notifyInstructionV2Ops(ctx, event)
	return s.blockInstructionV2Decision(evaluation, event, "invalid_json")
}

func (s *InstructionV2Service) evaluateInstructionV2AI(ctx context.Context, evaluation instructionV2EvaluationContext) *InstructionDecision {
	prepared := prepareInstructionV2AISample(evaluation.selectedField, evaluation.snapshot.Config.AIInputMaxChars)
	evaluation.selectedField = prepared
	if evaluation.selectedFieldName == "instructions" {
		evaluation.fields.Instructions = prepared
	} else {
		evaluation.fields.Input1 = prepared
	}
	attempt := s.runInstructionV2Review(ctx, evaluation.snapshot, evaluation.selectedFieldName, prepared, "sync")
	eventDecision := "block"
	eventOutcome := InstructionV2OutcomeAIPending
	reason := "sync_ai_" + attempt.Result
	if evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve || attempt.Result == "pass" {
		eventDecision = "allow"
		if attempt.Result == "pass" {
			eventOutcome = InstructionV2OutcomeAIPass
		} else {
			eventOutcome = InstructionV2OutcomeObserveAllow
		}
	}
	if attempt.Result == "reject" {
		eventOutcome = InstructionV2OutcomeBlocked
		if evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
			eventOutcome = InstructionV2OutcomeObserveAllow
		}
	}
	event := s.newInstructionV2Event(evaluation, eventDecision, eventOutcome, reason)
	event.AIResult = attempt.Result
	event.AIReviewedField = evaluation.selectedFieldName
	event.AISampled = prepared.AISampled
	event.AILatencyMS = attempt.LatencyMS
	write := instructionV2PersistEvent{Event: event, Reviews: []instructionV2AIAttempt{attempt}}

	vault, vaultErr := s.prepareInstructionV2Vault(evaluation.selectedFieldName, prepared)
	if vaultErr != nil {
		attempt.Result = "error"
		attempt.Reason = "Encrypted content storage unavailable"
		attempt.Category = "persistence_error"
		write.Reviews = []instructionV2AIAttempt{attempt}
		write.Event.AIResult = "error"
		write.Event.Decision = "block"
		write.Event.Outcome = InstructionV2OutcomeAIPending
		write.Event.Reason = "persistence_error"
		if evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
			write.Event.Decision = "allow"
			write.Event.Outcome = InstructionV2OutcomeObserveAllow
		}
	} else {
		switch attempt.Result {
		case "reject":
			if evaluation.snapshot.Config.EffectiveMode != InstructionV2ModeObserve {
				write.Risk = &instructionV2RiskWrite{
					Vault: vault, Source: "sync_ai", ObservedField: evaluation.selectedFieldName,
					ReviewerNodeID: attempt.NodeID, ReviewerModel: attempt.ReviewerModel,
					PromptVersion: attempt.PromptVersion, Confidence: attempt.Confidence,
					ReviewReason: attempt.Reason, ReviewCategory: attempt.Category,
				}
			}
		default:
			write.ReviewJob = &instructionV2ReviewJobWrite{
				Vault: vault, SelectedField: evaluation.selectedFieldName,
				PromptVersion:  evaluation.snapshot.PromptVersion,
				ReviewCriteria: evaluation.snapshot.Config.ReviewCriteria,
				ConfigVersion:  evaluation.snapshot.Config.ConfigVersion,
				ObserveOnly:    evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve,
				Sampled:        prepared.AISampled, SampleBytes: len([]byte(prepared.AISample)),
			}
		}
	}

	result, persistErr := s.persistCriticalEventWrite(ctx, write)
	if persistErr != nil {
		slog.Error("instruction_audit_v2.ai_event_persist_failed", "request_id", evaluation.request.RequestID, "error", persistErr)
		failure := s.newInstructionV2Event(evaluation, "block", InstructionV2OutcomeBlocked, "persistence_error")
		failure.AIResult = "error"
		if evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
			return s.allowInstructionV2Decision(evaluation, failure, "persistence_error")
		}
		return s.blockInstructionV2Decision(evaluation, failure, "persistence_error")
	}
	event = write.Event
	event.ID = result.EventID
	event.ReviewJobID = result.JobID
	if write.Risk != nil || (event.Decision == "block" && attempt.Result != "pass") {
		s.notifyInstructionV2Ops(ctx, event)
	}
	if write.Risk != nil {
		s.refreshAfterMutation(ctx, evaluation.snapshot.Config.ConfigVersion)
	}
	if event.Decision == "allow" {
		return s.allowInstructionV2Decision(evaluation, event, event.Reason)
	}
	return s.blockInstructionV2Decision(evaluation, event, event.Reason)
}

func (s *InstructionV2Service) evaluateExistingInstructionV2Review(
	ctx context.Context,
	evaluation instructionV2EvaluationContext,
) (*InstructionDecision, bool) {
	if s.repository == nil {
		return nil, false
	}
	prepared := prepareInstructionV2AISample(
		evaluation.selectedField,
		evaluation.snapshot.Config.AIInputMaxChars,
	)
	write := instructionV2ReviewJobWrite{
		Vault: instructionV2VaultWrite{
			SHA256:       prepared.SHA256,
			ContentBytes: prepared.Bytes,
		},
		SelectedField:   evaluation.selectedFieldName,
		PromptVersion:   evaluation.snapshot.PromptVersion,
		ReviewCriteria:  evaluation.snapshot.Config.ReviewCriteria,
		ConfigVersion:   evaluation.snapshot.Config.ConfigVersion,
		ObserveOnly:     evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve,
		Sampled:         prepared.AISampled,
		SampleBytes:     len([]byte(prepared.AISample)),
		SourceUserEmail: evaluation.request.UserEmail,
	}
	if evaluation.request.UserID > 0 {
		write.SourceUserID = instructionV2Int64Pointer(evaluation.request.UserID)
	}
	reuse, err := s.repository.ResumeOrGetReviewJobBySHA(ctx, write)
	if err != nil {
		slog.Warn(
			"instruction_audit_v2.review_reuse_failed",
			"sha256", prepared.SHA256,
			"request_id", evaluation.request.RequestID,
			"error", err,
		)
		return nil, false
	}
	if reuse == nil {
		return nil, false
	}
	evaluation.selectedField = prepared
	if evaluation.selectedFieldName == "instructions" {
		evaluation.fields.Instructions = prepared
	} else {
		evaluation.fields.Input1 = prepared
	}
	reason := "async_review_pending"
	if reuse.Requeued || reuse.ResetForEnforcement {
		reason = "async_review_requeued"
	}
	decision := "block"
	if evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve ||
		(!reuse.ResetForEnforcement && reuse.SourceDecision == "allow") {
		decision = "allow"
	}
	event := s.newInstructionV2Event(
		evaluation,
		decision,
		InstructionV2OutcomeAIPending,
		reason,
	)
	event.ReviewJobID = instructionV2Int64Pointer(reuse.JobID)
	persistWrite := instructionV2PersistEvent{Event: event}
	if reuse.Requeued || reuse.ResetForEnforcement {
		persistWrite.ReviewReuse = &instructionV2ReviewReuseWrite{Reuse: *reuse, Job: write}
	}
	result, persistErr := s.persistCriticalEventWrite(ctx, persistWrite)
	if persistErr != nil {
		slog.Error(
			"instruction_audit_v2.review_reuse_event_persist_failed",
			"request_id", evaluation.request.RequestID,
			"review_job_id", reuse.JobID,
			"error", persistErr,
		)
		failure := s.newInstructionV2Event(
			evaluation,
			"block",
			InstructionV2OutcomeBlocked,
			"persistence_error",
		)
		if evaluation.snapshot.Config.EffectiveMode == InstructionV2ModeObserve {
			return s.allowInstructionV2Decision(evaluation, failure, "persistence_error"), true
		}
		return s.blockInstructionV2Decision(evaluation, failure, "persistence_error"), true
	}
	event.ID = result.EventID
	if decision == "allow" {
		return s.allowInstructionV2Decision(evaluation, event, reason), true
	}
	return s.blockInstructionV2Decision(evaluation, event, reason), true
}

func (s *InstructionV2Service) runInstructionV2Review(
	ctx context.Context,
	snapshot *instructionV2Snapshot,
	fieldName string,
	field InstructionV2Field,
	slot string,
) instructionV2AIAttempt {
	attemptStartedAt := time.Now()
	attempt := instructionV2AIAttempt{
		FieldName: fieldName, SHA256: field.SHA256, PromptVersion: snapshot.PromptVersion,
		Sampled: field.AISampled, NodeSlot: slot,
	}
	node := snapshot.AINodesBySlot[slot]
	if node == nil {
		attempt.Result, attempt.Reason, attempt.Category = "error", "AI review node is not configured", "technical_error"
		return attempt
	}
	attempt.NodeID = instructionV2Int64Pointer(node.ID)
	attempt.NodeName = node.Name
	attempt.ReviewerModel = node.Model
	wait := time.Duration(snapshot.Config.AIQueueWaitMS) * time.Millisecond
	releaseGlobal, err := acquireInstructionV2Semaphore(ctx, snapshot.GlobalSemaphore, wait)
	if err != nil {
		attempt.Result, attempt.Reason, attempt.Category = "error", "AI review queue unavailable", "technical_error"
		attempt.LatencyMS = int(time.Since(attemptStartedAt).Milliseconds())
		return attempt
	}
	defer releaseGlobal()
	releaseNode, err := acquireInstructionV2Semaphore(ctx, node.semaphore, wait)
	if err != nil {
		attempt.Result, attempt.Reason, attempt.Category = "error", "AI review node queue unavailable", "technical_error"
		attempt.LatencyMS = int(time.Since(attemptStartedAt).Milliseconds())
		return attempt
	}
	defer releaseNode()
	timeoutMS := node.TimeoutMS
	if snapshot.Config.AITotalTimeoutMS > 0 && (timeoutMS <= 0 || snapshot.Config.AITotalTimeoutMS < timeoutMS) {
		timeoutMS = snapshot.Config.AITotalTimeoutMS
	}
	nodeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	result, reviewErr := s.reviewer.Review(
		nodeCtx, node, snapshot.Config.ReviewCriteria, snapshot.PromptVersion,
		fieldName, field.AISample, field.AISampled,
	)
	cancel()
	attempt.LatencyMS = int(time.Since(attemptStartedAt).Milliseconds())
	if reviewErr != nil {
		switch {
		case errors.Is(reviewErr, context.DeadlineExceeded), errors.Is(nodeCtx.Err(), context.DeadlineExceeded):
			attempt.Result, attempt.Reason, attempt.Category = "timeout", "AI review timed out", "technical_error"
		case errors.Is(reviewErr, errInstructionV2AIInvalid):
			attempt.Result, attempt.Reason, attempt.Category = "invalid", "AI review returned an invalid result", "technical_error"
		default:
			attempt.Result, attempt.Reason, attempt.Category = "error", "AI review node unavailable", "technical_error"
		}
		return attempt
	}
	attempt.Result, attempt.Confidence = result.Result, result.Confidence
	attempt.Reason, attempt.Category = result.Reason, result.Category
	applyInstructionV2ConfidenceThreshold(&attempt, snapshot.Config.ConfidenceThreshold)
	return attempt
}

func applyInstructionV2ConfidenceThreshold(attempt *instructionV2AIAttempt, threshold float64) {
	if attempt == nil || attempt.Result == "uncertain" || threshold <= 0 || attempt.Confidence >= threshold {
		return
	}
	attempt.Result = "uncertain"
	attempt.Reason = "AI review confidence is below the configured threshold"
	attempt.Category = "low_confidence"
}

func (s *InstructionV2Service) prepareInstructionV2Vault(fieldName string, field InstructionV2Field) (instructionV2VaultWrite, error) {
	if field.State != "valid" || field.SHA256 == "" || field.Plaintext == "" {
		return instructionV2VaultWrite{}, errors.New("instruction audit selected field is unavailable")
	}
	if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
		return instructionV2VaultWrite{}, errInstructionEvidenceEncryptionUnavailable
	}
	ciphertext, err := s.evidenceCipher.EncryptHashRaw(field.SHA256, field.Plaintext)
	if err != nil {
		return instructionV2VaultWrite{}, err
	}
	return instructionV2VaultWrite{
		SHA256: field.SHA256, ObservedField: fieldName, RawCiphertext: ciphertext,
		ContentBytes: field.Bytes, StoredBytes: len([]byte(field.Plaintext)),
	}, nil
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
		SelectedField: evaluation.selectedFieldName, SelectedSHA256: evaluation.selectedField.SHA256,
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
		Input1: evaluationLegacyInput1(evaluation), ConfigVersion: evaluation.snapshot.Config.ConfigVersion,
		BodyBytes: evaluation.bodyBytes, Latency: time.Since(evaluation.startedAt),
	}
}

func (s *InstructionV2Service) blockInstructionV2Decision(evaluation instructionV2EvaluationContext, event InstructionV2Event, reason string) *InstructionDecision {
	return &InstructionDecision{
		EventID: event.ID, HTTPStatus: http.StatusForbidden, ErrorCode: InstructionErrorCodeRejected,
		ClientMessage: InstructionClientMessage, Applicable: true, Allow: false, Reason: reason,
		InitialReason: reason, FinalReason: reason, FinalOutcome: event.Outcome,
		PolicyAction: "block", Instructions: instructionV2LegacyField(evaluation.fields.Instructions),
		Input1: evaluationLegacyInput1(evaluation), ConfigVersion: evaluation.snapshot.Config.ConfigVersion,
		BodyBytes: evaluation.bodyBytes, Latency: time.Since(evaluation.startedAt),
		AILatency: time.Duration(event.AILatencyMS) * time.Millisecond,
	}
}

func evaluationLegacyInput1(evaluation instructionV2EvaluationContext) InstructionFieldResult {
	return instructionV2LegacyField(evaluation.fields.Input1)
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

func (s *InstructionV2Service) persistCriticalEvent(ctx context.Context, write instructionV2PersistEvent) (int64, error) {
	result, err := s.persistCriticalEventWrite(ctx, write)
	return result.EventID, err
}

func (s *InstructionV2Service) persistCriticalEventWrite(ctx context.Context, write instructionV2PersistEvent) (instructionV2PersistResult, error) {
	if s == nil || s.repository == nil {
		return instructionV2PersistResult{}, errors.New("instruction audit v2 repository unavailable")
	}
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
	if s == nil || s.repository == nil {
		return
	}
	event.Instructions.Plaintext, event.Instructions.AISample = "", ""
	event.Input1.Plaintext, event.Input1.AISample = "", ""
	select {
	case s.passQueue <- event:
	default:
		s.persistNormalPass(ctx, event)
	}
}

func (s *InstructionV2Service) persistNormalPass(ctx context.Context, event InstructionV2Event) {
	if s == nil || s.repository == nil {
		return
	}
	if _, err := s.persistCriticalEvent(ctx, instructionV2PersistEvent{Event: event}); err != nil {
		slog.Error("instruction_audit_v2.pass_persist_failed", "request_id", event.RequestID, "error", err)
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

func (s *InstructionV2Service) reviewWorker(ctx context.Context, workerID int) {
	defer s.wg.Done()
	hostname, _ := os.Hostname()
	owner := fmt.Sprintf("%s:%d:%d", hostname, os.Getpid(), workerID)
	ticker := time.NewTicker(InstructionV2ReviewPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				job, err := s.repository.ClaimReviewJob(ctx, owner, InstructionV2ReviewLeaseDuration)
				if err != nil {
					slog.Warn("instruction_audit_v2.review_claim_failed", "error", err)
					break
				}
				if job == nil {
					break
				}
				s.processInstructionV2ReviewJob(ctx, job)
			}
		}
	}
}

func (s *InstructionV2Service) processInstructionV2ReviewJob(ctx context.Context, job *instructionV2ClaimedReviewJob) {
	if job == nil {
		return
	}
	plaintext, err := s.evidenceCipher.DecryptHashRaw(job.SHA256, job.Ciphertext)
	if err != nil {
		s.scheduleInstructionV2ReviewRetry(ctx, job, "content decrypt failed")
		return
	}
	field := newInstructionV2TextField(plaintext, false)
	if field.SHA256 != job.SHA256 || field.Bytes != job.ContentBytes {
		s.scheduleInstructionV2ReviewRetry(ctx, job, "content integrity check failed")
		return
	}
	snapshot := s.snapshot.Load()
	if snapshot == nil {
		s.scheduleInstructionV2ReviewRetry(ctx, job, "runtime configuration unavailable")
		return
	}
	field = prepareInstructionV2AISample(field, snapshot.Config.AIInputMaxChars)
	jobSnapshot := *snapshot
	jobSnapshot.Config = snapshot.Config
	jobSnapshot.Config.ReviewCriteria = job.ReviewCriteria
	jobSnapshot.PromptVersion = job.PromptVersion
	existing, err := s.repository.ListReviewAttempts(ctx, job.ID)
	if err != nil {
		s.scheduleInstructionV2ReviewRetry(ctx, job, "attempt history unavailable")
		return
	}
	resolved := make(map[string]bool, 3)
	for _, attempt := range existing {
		if attempt.Result == "pass" || attempt.Result == "reject" {
			resolved[attempt.NodeSlot] = true
		}
	}
	type result struct {
		attempt instructionV2AIAttempt
	}
	results := make(chan result, 3)
	pending := 0
	for _, slot := range []string{"async_1", "async_2", "async_3"} {
		if resolved[slot] {
			continue
		}
		pending++
		go func(nodeSlot string) {
			results <- result{attempt: s.runInstructionV2Review(ctx, &jobSnapshot, job.SelectedField, field, nodeSlot)}
		}(slot)
	}
	finalized := false
	finalResult := ""
	lastError := "no valid async majority"
	attempts := make([]InstructionV2ReviewAttempt, 0, pending)
	for index := 0; index < pending; index++ {
		result := <-results
		attempt := InstructionV2ReviewAttempt{
			NodeID: result.attempt.NodeID, NodeSlot: result.attempt.NodeSlot,
			NodeName: result.attempt.NodeName, ReviewerModel: result.attempt.ReviewerModel,
			Result: result.attempt.Result, Confidence: result.attempt.Confidence,
			Reason: result.attempt.Reason, Category: result.attempt.Category,
			PromptVersion: job.PromptVersion, Sampled: result.attempt.Sampled,
			LatencyMS: result.attempt.LatencyMS,
		}
		attempts = append(attempts, attempt)
		if attempt.Result != "pass" && attempt.Result != "reject" {
			lastError = attempt.Result + ": " + attempt.Reason
		}
	}
	if len(attempts) > 0 {
		resolvedResult, didFinalize, recordErr := s.repository.RecordReviewAttempts(
			ctx, job.ID, job.LeaseOwner, attempts,
		)
		if recordErr != nil {
			if errors.Is(recordErr, errInstructionV2ReviewLeaseLost) {
				return
			}
			lastError = "review attempt persistence failed"
			slog.Warn("instruction_audit_v2.review_attempts_persist_failed", "job_id", job.ID, "error", recordErr)
		} else if didFinalize {
			finalized, finalResult = true, resolvedResult
		}
	}
	if finalized {
		s.refreshAfterMutation(ctx, snapshot.Config.ConfigVersion)
		if finalResult == "reject" && !job.ObserveOnly {
			s.notifyInstructionV2ReviewJob(ctx, job, "async_ai_rejected")
		}
		return
	}
	s.scheduleInstructionV2ReviewRetry(ctx, job, lastError)
}

func (s *InstructionV2Service) scheduleInstructionV2ReviewRetry(ctx context.Context, job *instructionV2ClaimedReviewJob, message string) {
	snapshot := s.snapshot.Load()
	schedule := []int{30, 120, 600, 3600, 21600}
	if snapshot != nil && len(snapshot.Config.AsyncRetrySchedule) > 0 {
		schedule = snapshot.Config.AsyncRetrySchedule
	}
	exhausted, err := s.repository.ScheduleReviewRetry(ctx, job.ID, job.LeaseOwner, schedule, message)
	if err != nil {
		if errors.Is(err, errInstructionV2ReviewLeaseLost) {
			return
		}
		slog.Warn("instruction_audit_v2.review_retry_schedule_failed", "job_id", job.ID, "error", err)
		return
	}
	if exhausted && !job.ObserveOnly {
		s.notifyInstructionV2ReviewJob(ctx, job, "async_retry_exhausted")
	}
}

func (s *InstructionV2Service) notifyInstructionV2ReviewJob(ctx context.Context, job *instructionV2ClaimedReviewJob, reason string) {
	if job == nil || job.SourceEventID == nil {
		return
	}
	event, err := s.repository.GetEvent(ctx, *job.SourceEventID)
	if err != nil {
		return
	}
	event.Reason = reason
	s.notifyInstructionV2Ops(ctx, event)
}

func (s *InstructionV2Service) notifyInstructionV2Ops(ctx context.Context, event InstructionV2Event) {
	if s.notifications == nil || event.ID <= 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instructionV2NotificationTimeout)
	defer cancel()
	digest := event.SelectedSHA256
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
		"policy_action": event.Decision, "config_version": strconv.FormatInt(event.ConfigVersion, 10),
		"instructions_present": strconv.FormatBool(event.Instructions.State == "valid"),
		"instructions_result":  event.Instructions.State, "instructions_sha256": event.Instructions.SHA256,
		"input1_present": strconv.FormatBool(event.Input1.State == "valid"),
		"input1_result":  event.Input1.State, "input1_sha256": event.Input1.SHA256,
	}
	if err := s.notifications.Enqueue(notifyCtx, service.SecurityNotificationEnqueueInput{
		SourceType: service.SecurityNotificationSourceInstructionAuditV2, SourceID: event.ID,
		DedupeScope:  fmt.Sprintf("%d:%s:%s", pointerInt64Value(event.UserID), digest, event.Reason),
		DedupeWindow: 10 * time.Minute, SkipUser: true,
		OpsTemplate: service.NotificationEmailEventInstructionAuditOpsNotice,
		Variables:   variables,
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
