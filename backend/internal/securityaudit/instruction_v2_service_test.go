package securityaudit

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type countingInstructionV2Reviewer struct {
	calls int
}

func TestInstructionV2ClientProfileUpdateAllowsEveryBuiltInRuleToBeDisabled(t *testing.T) {
	for _, key := range []string{InstructionClientOther, InstructionClientUnknown} {
		t.Run(key, func(t *testing.T) {
			request, err := normalizeInstructionV2ClientProfileUpdate(
				InstructionV2ClientProfile{ProfileKey: key, Name: key, BuiltIn: true, Enabled: true},
				SaveInstructionV2ClientProfileRequest{ProfileKey: "ignored", Name: key, Enabled: false},
			)
			require.NoError(t, err)
			require.Equal(t, key, request.ProfileKey)
			require.False(t, request.Enabled)
		})
	}

	existing := InstructionV2ClientProfile{
		ProfileKey: InstructionClientModelPortInternal, Name: "ModelPort Internal",
		Description: "trusted identity", Priority: 0, Enabled: true,
		BuiltIn: true, ImmutableInternal: true,
	}
	request, err := normalizeInstructionV2ClientProfileUpdate(existing, SaveInstructionV2ClientProfileRequest{
		ProfileKey: "tampered", Name: "tampered", Description: "tampered", Priority: 999,
		Enabled: false, Matchers: []InstructionV2ClientMatcher{{Type: "prefix", Value: "tampered/"}},
	})
	require.NoError(t, err)
	require.Equal(t, existing.ProfileKey, request.ProfileKey)
	require.Equal(t, existing.Name, request.Name)
	require.Equal(t, existing.Description, request.Description)
	require.Equal(t, existing.Priority, request.Priority)
	require.Empty(t, request.Matchers)
	require.False(t, request.Enabled)

	_, _, err = normalizeInstructionV2ClientProfiles([]InstructionV2ClientProfile{
		{ID: 1, ProfileKey: InstructionClientModelPortInternal, Name: "ModelPort Internal", Enabled: false, BuiltIn: true, ImmutableInternal: true},
		{ID: 2, ProfileKey: InstructionClientOther, Name: "Other", Enabled: false, BuiltIn: true},
		{ID: 3, ProfileKey: InstructionClientUnknown, Name: "Unknown", Enabled: false, BuiltIn: true},
	})
	require.NoError(t, err)
}

func (r *countingInstructionV2Reviewer) Review(
	context.Context,
	*instructionV2AINodeRuntime,
	string,
	string,
	string,
	string,
	bool,
) (instructionV2AIResult, error) {
	r.calls++
	return instructionV2AIResult{
		Result: "pass", Confidence: 0.99, Reason: "test", Category: "test",
	}, nil
}

func TestInstructionV2ServiceUsesAuthenticatedGroupAndClientScope(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	trustedDigest := instructionV2TestDigest("trusted-template")
	snapshot.Hashes[trustedDigest] = instructionV2HashRuntime{
		ID: 501, SHA256: trustedDigest, ScopeIDs: map[int64]struct{}{101: {}, 102: {}},
	}
	service.snapshot.Store(snapshot)

	tests := []struct {
		name       string
		groupID    int64
		userAgent  string
		model      string
		applicable bool
		outcome    string
	}{
		{name: "all clients scope", groupID: 7, userAgent: "curl/8.0", model: "gpt-a", applicable: true, outcome: InstructionV2OutcomeHashPass},
		{name: "client-specific scope", groupID: 8, userAgent: "codex_cli_rs/0.145.0", model: "unrelated-model", applicable: true, outcome: InstructionV2OutcomeHashPass},
		{name: "different client", groupID: 8, userAgent: "opencode/1.0", model: "gpt-a", applicable: false},
		{name: "different group", groupID: 99, userAgent: "codex_cli_rs/0.145.0", model: "gpt-a", applicable: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := service.EvaluateInstruction(context.Background(), Request{
				Protocol:        instructionAuditProtocol,
				GroupID:         instructionV2TestInt64Pointer(test.groupID),
				UserAgent:       test.userAgent,
				Model:           test.model,
				InstructionBody: []byte(`{"instructions":"trusted-template"}`),
			})
			require.True(t, decision.Allow)
			require.Equal(t, test.applicable, decision.Applicable)
			if test.applicable {
				require.Equal(t, test.outcome, decision.FinalOutcome)
				require.Equal(t, "scoped_trusted_hash_match", decision.Reason)
			}
		})
	}
}

func TestInstructionV2ServiceChecksHashBeforeAI(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeObserve)
	digest := instructionV2TestDigest("trusted input1")
	snapshot.Hashes[digest] = instructionV2HashRuntime{
		ID: 502, SHA256: digest, ScopeIDs: map[int64]struct{}{101: {}},
	}
	service.snapshot.Store(snapshot)

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol:        instructionAuditProtocol,
		GroupID:         instructionV2TestInt64Pointer(7),
		UserAgent:       "curl/8.0",
		InstructionBody: []byte(`{"input":[{}, {"content":[{"type":"input_text","text":"trusted input1"}]}]}`),
	})

	require.True(t, decision.Allow)
	require.True(t, decision.Applicable)
	require.Equal(t, InstructionV2OutcomeHashPass, decision.FinalOutcome)
	require.Equal(t, "scoped_trusted_hash_match", decision.Reason)
}

func TestInstructionV2ServiceAllowsConfiguredExceptionsBeforeAI(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	snapshot.AllowedUsers[77] = struct{}{}
	service.snapshot.Store(snapshot)

	allowlisted := service.EvaluateInstruction(context.Background(), Request{
		Protocol:        instructionAuditProtocol,
		UserID:          77,
		GroupID:         instructionV2TestInt64Pointer(7),
		InstructionBody: []byte(`{`),
	})
	require.True(t, allowlisted.Allow)
	require.Equal(t, InstructionV2OutcomeAllowlistPass, allowlisted.FinalOutcome)
	require.Equal(t, "user_allowlist", allowlisted.Reason)

	empty := service.EvaluateInstruction(context.Background(), Request{
		Protocol:        instructionAuditProtocol,
		GroupID:         instructionV2TestInt64Pointer(7),
		InstructionBody: []byte(`{"input":[]}`),
	})
	require.True(t, empty.Allow)
	require.Equal(t, InstructionV2OutcomeEmptyPass, empty.FinalOutcome)
	require.Equal(t, "fields_empty", empty.Reason)
}

func TestInstructionV2ServiceSelectsExactlyOneField(t *testing.T) {
	instructions := newInstructionV2TextField("primary instructions", false)
	input1 := newInstructionV2TextField("fallback input", false)

	name, field := selectInstructionV2Field(instructionV2ParsedFields{Instructions: instructions, Input1: input1})
	require.Equal(t, "instructions", name)
	require.Equal(t, instructions.SHA256, field.SHA256)

	name, field = selectInstructionV2Field(instructionV2ParsedFields{
		Instructions: InstructionV2Field{State: "invalid"},
		Input1:       input1,
	})
	require.Equal(t, "input1", name)
	require.Equal(t, input1.SHA256, field.SHA256)
}

func TestInstructionV2ServiceRiskHashTakesPrecedence(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	digest := instructionV2TestDigest("same content")
	snapshot.Hashes[digest] = instructionV2HashRuntime{ID: 10, SHA256: digest, Global: true}
	snapshot.RiskHashes[digest] = instructionV2RiskRuntime{ID: 20, SHA256: digest}
	service.snapshot.Store(snapshot)

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, GroupID: instructionV2TestInt64Pointer(7),
		InstructionBody: []byte(`{"instructions":"same content"}`),
	})

	require.False(t, decision.Allow)
	require.Equal(t, InstructionV2OutcomeRiskBlocked, decision.FinalOutcome)
	require.Equal(t, "risk_hash_match", decision.Reason)
	require.Equal(t, InstructionClientMessage, decision.ClientMessage)
}

func TestInstructionV2ServiceReusesReviewJobBeforeSyncAI(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		jobStatus      string
		sourceDecision string
		wantAllow      bool
		wantReason     string
		wantRequeue    bool
	}{
		{
			name: "pending task reuses blocked sync result", mode: InstructionV2ModeEnforce,
			jobStatus:      "pending",
			sourceDecision: "block", wantAllow: false, wantReason: "async_review_pending",
		},
		{
			name: "failed task requeues and reuses allowed sync result", mode: InstructionV2ModeEnforce,
			jobStatus:      "failed",
			sourceDecision: "allow", wantAllow: true, wantReason: "async_review_requeued",
			wantRequeue: true,
		},
		{
			name: "missing source decision fails closed", mode: InstructionV2ModeEnforce,
			jobStatus: "retry", sourceDecision: "", wantAllow: false,
			wantReason: "async_review_pending",
		},
		{
			name: "observe mode overrides blocked source decision", mode: InstructionV2ModeObserve,
			jobStatus: "processing", sourceDecision: "block", wantAllow: true,
			wantReason: "async_review_pending",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			service, snapshot := newInstructionV2TestService(t, test.mode)
			service.repository = NewInstructionV2Repository(db)
			reviewer := &countingInstructionV2Reviewer{}
			service.reviewer = reviewer
			snapshot.AINodesBySlot["sync"] = &instructionV2AINodeRuntime{
				InstructionV2AINode: InstructionV2AINode{
					ID: 1, Name: "sync", TimeoutMS: 1000, MaxConcurrency: 1, Slot: "sync",
				},
				APIKey: "test-key", semaphore: make(chan struct{}, 1),
			}
			service.snapshot.Store(snapshot)
			digest := instructionV2TestDigest("reuse review field")

			mock.ExpectBegin()
			mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtext\(\$1\)\)`).
				WithArgs(digest).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(`(?s)SELECT job.id, job.status, job.observe_only.*FOR UPDATE OF job`).
				WithArgs(digest).
				WillReturnRows(sqlmock.NewRows([]string{"id", "status", "observe_only", "decision"}).
					AddRow(int64(71), test.jobStatus, false, test.sourceDecision))
			if test.wantRequeue {
				mock.ExpectExec(`(?s)UPDATE instruction_audit_v2_review_jobs.*SET status = 'retry'.*WHERE id = \$1`).
					WithArgs(int64(71)).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
			mock.ExpectBegin()
			mock.ExpectQuery(`(?s)INSERT INTO instruction_audit_v2_events.*RETURNING id`).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
			mock.ExpectCommit()
			mock.ExpectClose()

			decision := service.EvaluateInstruction(context.Background(), Request{
				Protocol: instructionAuditProtocol, RequestID: "reuse-review-request",
				GroupID:         instructionV2TestInt64Pointer(7),
				InstructionBody: []byte(`{"instructions":"reuse review field"}`),
			})

			require.Equal(t, test.wantAllow, decision.Allow)
			require.Equal(t, test.wantReason, decision.Reason)
			require.Equal(t, InstructionV2OutcomeAIPending, decision.FinalOutcome)
			require.Equal(t, int64(91), decision.EventID)
			require.Zero(t, reviewer.calls)
			require.NoError(t, db.Close())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestInstructionV2ServiceRequiresConfiguredReviewSlot(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	field := prepareInstructionV2AISample(newInstructionV2TextField("review me", false), 64000)

	attempt := service.runInstructionV2Review(context.Background(), snapshot, "instructions", field, "sync")

	require.Equal(t, "error", attempt.Result)
	require.Equal(t, "technical_error", attempt.Category)
}

func TestInstructionV2ServiceRejectsAIWhenGlobalQueueIsFull(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	snapshot.Config.AIQueueWaitMS = 0
	snapshot.GlobalSemaphore = make(chan struct{}, 1)
	snapshot.GlobalSemaphore <- struct{}{}
	node := newInstructionV2UnavailableNode(1, "sync")
	snapshot.AINodesBySlot["sync"] = node
	field := prepareInstructionV2AISample(newInstructionV2TextField("review me", false), 64000)

	attempt := service.runInstructionV2Review(context.Background(), snapshot, "instructions", field, "sync")

	require.Equal(t, "error", attempt.Result)
	require.Contains(t, attempt.Reason, "queue")
}

func TestInstructionV2ConfidenceThresholdConvertsLowConfidenceDecision(t *testing.T) {
	attempt := instructionV2AIAttempt{Result: "pass", Confidence: 0.79, Reason: "looks safe", Category: "benign"}
	applyInstructionV2ConfidenceThreshold(&attempt, 0.8)
	require.Equal(t, "uncertain", attempt.Result)
	require.Equal(t, "low_confidence", attempt.Category)

	highConfidence := instructionV2AIAttempt{Result: "reject", Confidence: 0.95}
	applyInstructionV2ConfidenceThreshold(&highConfidence, 0.8)
	require.Equal(t, "reject", highConfidence.Result)
}

func TestReuseInstructionV2SemaphoresPreservesInFlightLimits(t *testing.T) {
	previousGlobal := make(chan struct{}, 64)
	previousGlobal <- struct{}{}
	previousNode := make(chan struct{}, 16)
	previousNode <- struct{}{}
	previous := &instructionV2Snapshot{
		GlobalSemaphore: previousGlobal,
		AINodes: []*instructionV2AINodeRuntime{{
			InstructionV2AINode: InstructionV2AINode{ID: 1},
			semaphore:           previousNode,
		}},
	}
	next := &instructionV2Snapshot{
		GlobalSemaphore: make(chan struct{}, 64),
		AINodes: []*instructionV2AINodeRuntime{
			{InstructionV2AINode: InstructionV2AINode{ID: 1}, semaphore: make(chan struct{}, 16)},
			{InstructionV2AINode: InstructionV2AINode{ID: 2}, semaphore: make(chan struct{}, 8)},
		},
	}

	reuseInstructionV2Semaphores(previous, next)

	require.Equal(t, previousGlobal, next.GlobalSemaphore)
	require.Len(t, next.GlobalSemaphore, 1)
	require.Equal(t, previousNode, next.AINodes[0].semaphore)
	require.Len(t, next.AINodes[0].semaphore, 1)
	require.NotEqual(t, previousNode, next.AINodes[1].semaphore)
}

func TestReuseInstructionV2SemaphoresReplacesChangedLimits(t *testing.T) {
	previous := &instructionV2Snapshot{
		GlobalSemaphore: make(chan struct{}, 64),
		AINodes: []*instructionV2AINodeRuntime{{
			InstructionV2AINode: InstructionV2AINode{ID: 1},
			semaphore:           make(chan struct{}, 16),
		}},
	}
	nextGlobal := make(chan struct{}, 32)
	nextNode := make(chan struct{}, 8)
	next := &instructionV2Snapshot{
		GlobalSemaphore: nextGlobal,
		AINodes: []*instructionV2AINodeRuntime{{
			InstructionV2AINode: InstructionV2AINode{ID: 1},
			semaphore:           nextNode,
		}},
	}

	reuseInstructionV2Semaphores(previous, next)

	require.Equal(t, nextGlobal, next.GlobalSemaphore)
	require.Equal(t, nextNode, next.AINodes[0].semaphore)
}

func newInstructionV2TestService(t *testing.T, mode string) (*InstructionV2Service, *instructionV2Snapshot) {
	t.Helper()
	profiles, byKey, err := normalizeInstructionV2ClientProfiles([]InstructionV2ClientProfile{
		{ID: 1, ProfileKey: InstructionClientCodexCLI, Name: "Codex CLI", Enabled: true, Priority: 10, Matchers: []InstructionV2ClientMatcher{{Type: "prefix", Value: "codex_cli_rs/"}, {Type: "prefix", Value: "codex-tui/"}}},
		{ID: 2, ProfileKey: InstructionClientOpenCode, Name: "OpenCode", Enabled: true, Priority: 20, Matchers: []InstructionV2ClientMatcher{{Type: "prefix", Value: "opencode/"}}},
		{ID: 3, ProfileKey: InstructionClientModelPortInternal, Name: "ModelPort Internal", Enabled: true, BuiltIn: true, ImmutableInternal: true},
		{ID: 4, ProfileKey: InstructionClientOther, Name: "Other", Enabled: true, BuiltIn: true, Priority: 100000},
		{ID: 5, ProfileKey: InstructionClientUnknown, Name: "Unknown", Enabled: true, BuiltIn: true, Priority: 100000},
	})
	require.NoError(t, err)
	codexID := int64(1)
	snapshot := &instructionV2Snapshot{
		Config: InstructionV2Config{
			Mode: mode, EffectiveMode: mode, ConfigVersion: 1,
			AIInputMaxChars: 64000, AIQueueWaitMS: 10, AITotalTimeoutMS: 1000,
			RawFullMaxBytes: 4 << 20, AllowEmptyFields: true,
		},
		PromptVersion: "test-v1",
		Profiles:      profiles,
		ProfilesByKey: byKey,
		ScopesByGroup: map[int64][]instructionV2ScopeRuntime{
			7: {{ID: 101, GroupID: 7}},
			8: {{ID: 102, GroupID: 8, ClientProfileID: &codexID, ClientProfileKey: InstructionClientCodexCLI}},
		},
		Hashes:          map[string]instructionV2HashRuntime{},
		RiskHashes:      map[string]instructionV2RiskRuntime{},
		AllowedUsers:    map[int64]struct{}{},
		AINodesBySlot:   map[string]*instructionV2AINodeRuntime{},
		GlobalSemaphore: make(chan struct{}, InstructionV2DefaultGlobalConcurrency),
		LoadedAt:        time.Now().UTC(),
	}
	return &InstructionV2Service{
		reviewer:  NewInstructionV2AIReviewer(),
		passQueue: make(chan InstructionV2Event, 8),
	}, snapshot
}

func newInstructionV2UnavailableNode(id int64, name string) *instructionV2AINodeRuntime {
	return &instructionV2AINodeRuntime{
		InstructionV2AINode: InstructionV2AINode{
			ID: id, Name: name, BaseURL: "://invalid", Model: "reviewer",
			TimeoutMS: 50, MaxConcurrency: 1, Enabled: true, Slot: name,
		},
		APIKey:    "test-key",
		semaphore: make(chan struct{}, 1),
	}
}

func instructionV2TestInt64Pointer(value int64) *int64 {
	return &value
}
