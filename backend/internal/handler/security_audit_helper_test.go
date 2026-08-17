package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCachesSecurityAuditCompletionSkipsWebSocketStages(t *testing.T) {
	require.True(t, cachesSecurityAuditCompletion("http"))
	require.True(t, cachesSecurityAuditCompletion(""))
	require.False(t, cachesSecurityAuditCompletion("first_turn"))
	require.False(t, cachesSecurityAuditCompletion("subsequent_turn"))
	require.True(t, isSecurityAuditWebSocketStage("first_turn"))
	require.True(t, isSecurityAuditWebSocketStage("subsequent_turn"))
	require.False(t, isSecurityAuditWebSocketStage("http"))
}

func TestRunSecurityAuditDoesNotSkipSubsequentWebSocketTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeAsync}
	coordinator := newPromptPatchTestCoordinator(engine)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	subject := middleware2.AuthSubject{UserID: 7, Concurrency: 1}
	first := runSecurityAudit(c, nil, coordinator, nil, nil, subject, "openai_chat_websocket", "gpt-test",
		[]byte(`{"type":"response.create","response":{"input":"benign"}}`), "first_turn")
	require.NotNil(t, first)
	require.True(t, first.AllowNextStage)
	require.Equal(t, int64(1), engine.enqueues.Load())
	_, cached := c.Get(securityAuditCompletedContextKey)
	require.False(t, cached, "WebSocket stages must not set the HTTP completion cache")

	// Even if an HTTP path previously cached completion on this Context, WS turns
	// must still audit every response.create payload.
	c.Set(securityAuditCompletedContextKey, true)

	second := runSecurityAudit(c, nil, coordinator, nil, nil, subject, "openai_chat_websocket", "gpt-test",
		[]byte(`{"type":"response.create","response":{"input":"malicious follow-up"}}`), "subsequent_turn")
	require.NotNil(t, second)
	require.Equal(t, int64(2), engine.enqueues.Load(), "subsequent WebSocket turns must be audited again")
}

func TestRunSecurityAuditPreservesDedicatedInstructionPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	instruction := &capturingInstructionEngine{}
	coordinator := securityaudit.NewCoordinatorWithInstruction(nil, nil, instruction)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	subject := middleware2.AuthSubject{UserID: 7, Concurrency: 1}
	forwardBody := []byte(`{"model":"mapped","instructions":"changed"}`)
	originalBody := []byte(`{"model":"original","instructions":"exact"}`)

	decision := runSecurityAuditWithInstructionPayload(c, nil, coordinator, nil, nil, subject,
		"openai_responses", "original", forwardBody, originalBody, true, "http", false)

	require.NotNil(t, decision)
	require.True(t, decision.AllowNextStage)
	require.Equal(t, forwardBody, instruction.request.Body)
	require.Equal(t, originalBody, instruction.request.InstructionBody)
	require.True(t, instruction.request.InstructionAuditExcluded)
}

func TestRunInstructionAuditUsesCompositePublicModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	instruction := &capturingInstructionEngine{}
	coordinator := securityaudit.NewCoordinatorWithInstruction(nil, nil, instruction)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := service.WithCompositeRouteDecision(context.Background(), service.CompositeRouteDecision{
		Matched: true, PublicModel: "public-model", UpstreamModel: "mapped-model", TargetPlatform: service.PlatformOpenAI,
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)

	decision := runInstructionAudit(c, nil, coordinator, nil, middleware2.AuthSubject{UserID: 7},
		"openai_responses", "mapped-model", []byte(`{"model":"mapped-model","instructions":"exact"}`), false, "http")

	require.Nil(t, decision)
	require.Equal(t, "public-model", instruction.request.Model)
	require.True(t, instruction.request.InstructionModelOverride)
}

func TestRunInstructionAuditUsesAuthenticatedDownstreamGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	instruction := &capturingInstructionEngine{}
	coordinator := securityaudit.NewCoordinatorWithInstruction(nil, nil, instruction)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	downstreamGroupID := int64(41)
	apiKey := &service.APIKey{
		ID: 17, GroupID: &downstreamGroupID,
		Group: &service.Group{ID: downstreamGroupID, Name: "Downstream OpenAI", Platform: service.PlatformOpenAI},
	}

	decision := runInstructionAudit(c, nil, coordinator, apiKey, middleware2.AuthSubject{UserID: 7},
		"openai_responses", "mapped-model", []byte(`{"instructions":"exact"}`), false, "http")

	require.Nil(t, decision)
	require.NotNil(t, instruction.request.GroupID)
	require.Equal(t, downstreamGroupID, *instruction.request.GroupID)
	require.Equal(t, "Downstream OpenAI", instruction.request.GroupName)
}

func TestRunInstructionAuditCarriesClientIdentitySignalsAcrossTransports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stage := range []string{"http", "first_turn", "subsequent_turn"} {
		t.Run(stage, func(t *testing.T) {
			instruction := &capturingInstructionEngine{}
			coordinator := securityaudit.NewCoordinatorWithInstruction(nil, nil, instruction)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx := securityaudit.WithTrustedInternalInstructionClient(context.Background())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.145.0")

			decision := runInstructionAudit(c, nil, coordinator, nil, middleware2.AuthSubject{UserID: 7},
				"openai_responses", "gpt-test", []byte(`{"instructions":"exact"}`), false, stage)

			require.Nil(t, decision)
			require.Equal(t, "codex_cli_rs/0.145.0", instruction.request.UserAgent)
			require.True(t, instruction.request.TrustedInternalClient)
			require.Equal(t, stage, instruction.request.Stage)
		})
	}
}

func TestRunInstructionAuditLogsPersistedEventID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	instruction := &capturingInstructionEngine{decision: &securityaudit.InstructionDecision{
		EventID: 17, Applicable: true, Allow: false, Reason: "hash_mismatch",
	}}
	coordinator := securityaudit.NewCoordinatorWithInstruction(nil, nil, instruction)
	core, logs := observer.New(zap.InfoLevel)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	decision := runInstructionAudit(c, zap.New(core), coordinator, nil, middleware2.AuthSubject{UserID: 7},
		"openai_responses", "gpt-test", []byte(`{"instructions":"blocked"}`), false, "http")

	require.NotNil(t, decision)
	require.False(t, decision.AllowNextStage)
	entries := logs.FilterMessage("instruction_audit.gateway_check_done").All()
	require.Len(t, entries, 1)
	require.Equal(t, zap.WarnLevel, entries[0].Level)
	fields := entries[0].ContextMap()
	require.EqualValues(t, 17, fields["event_id"])
	require.Equal(t, "hash_mismatch", fields["reason"])
}

func TestRunInstructionAuditKeepsAllowedCompletionAtInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	instruction := &capturingInstructionEngine{decision: &securityaudit.InstructionDecision{
		Applicable: true, Allow: true, Reason: "hash_match",
	}}
	coordinator := securityaudit.NewCoordinatorWithInstruction(nil, nil, instruction)
	core, logs := observer.New(zap.InfoLevel)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	decision := runInstructionAudit(c, zap.New(core), coordinator, nil, middleware2.AuthSubject{UserID: 7},
		"openai_responses", "gpt-test", []byte(`{"instructions":"allowed"}`), false, "http")

	require.Nil(t, decision)
	entries := logs.FilterMessage("instruction_audit.gateway_check_done").All()
	require.Len(t, entries, 1)
	require.Equal(t, zap.InfoLevel, entries[0].Level)
}

func TestRunSecurityAuditDeduplicatesRepeatedPayloadWithinWebSocketTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeBlocking}
	coordinator := newPromptPatchTestCoordinator(engine)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	payload := []byte(`{"type":"response.create","response":{"input":"same turn"}}`)
	c.Set(securityAuditWSTurnContextKey, 2)
	first := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")
	second := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.True(t, first.AllowNextStage)
	require.True(t, second.AllowNextStage)
	require.Equal(t, int64(1), engine.evaluates.Load())

	// The cache holds only one successful same-turn result.
	entry, exists := c.Get(securityAuditWSDedupeContextKey)
	require.True(t, exists)
	require.IsType(t, securityAuditWSDedupeEntry{}, entry)

	c.Set(securityAuditWSTurnContextKey, 3)
	runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")
	require.Equal(t, int64(2), engine.evaluates.Load())
}

func TestRunSecurityAuditWebSocketDedupePreservesCompletedInstructionMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	instruction := &promptEligibleInstructionEngine{}
	prompt := &turnCountingEngine{mode: securityaudit.ModeBlocking}
	coordinator := securityaudit.NewCoordinatorWithInstruction(nil, prompt, instruction)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(securityAuditWSTurnContextKey, 2)
	payload := []byte(`{"type":"response.create","response":{"instructions":"already audited"}}`)

	first := runSecurityAuditWithInstructionPayload(
		c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7},
		"openai_chat_websocket", "gpt-test", payload, payload, false, "subsequent_turn", true,
	)
	second := runSecurityAuditWithInstructionPayload(
		c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7},
		"openai_chat_websocket", "gpt-test", payload, payload, false, "subsequent_turn", true,
	)

	require.True(t, first.AllowNextStage)
	require.True(t, second.AllowNextStage)
	require.True(t, prompt.request.InstructionAuditCompleted)
	require.Equal(t, payload, prompt.request.InstructionBody)
	require.Zero(t, instruction.calls.Load(), "the post-instruction stage must not rerun synchronous instruction review")
	require.Equal(t, int64(1), prompt.evaluates.Load(), "an allow may be reused only within the same turn")

	c.Set(securityAuditWSTurnContextKey, 3)
	runSecurityAuditWithInstructionPayload(
		c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7},
		"openai_chat_websocket", "gpt-test", payload, payload, false, "subsequent_turn", true,
	)
	require.Equal(t, int64(2), prompt.evaluates.Load(), "the next turn must be audited again")
}

func TestRunSecurityAuditDoesNotCacheFailedWebSocketDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{
		mode: securityaudit.ModeBlocking,
		decisions: []*securityaudit.PromptDecision{
			{Kind: securityaudit.DecisionUnavailable, AllowNextStage: false},
			{Kind: securityaudit.DecisionAllow, AllowNextStage: true},
		},
	}
	coordinator := newPromptPatchTestCoordinator(engine)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(securityAuditWSTurnContextKey, 2)
	payload := []byte(`{"type":"response.create","response":{"input":"retry me"}}`)

	first := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")
	_, cachedAfterFailure := c.Get(securityAuditWSDedupeContextKey)
	second := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")

	require.False(t, first.AllowNextStage)
	require.False(t, cachedAfterFailure)
	require.True(t, second.AllowNextStage)
	require.Equal(t, int64(2), engine.evaluates.Load())
}

func TestRunSecurityAuditDoesNotCacheFlaggedWebSocketDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{
		mode: securityaudit.ModeBlocking,
		decisions: []*securityaudit.PromptDecision{
			{Kind: securityaudit.DecisionFlag, AllowNextStage: true},
			{Kind: securityaudit.DecisionAllow, AllowNextStage: true},
		},
	}
	coordinator := newPromptPatchTestCoordinator(engine)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(securityAuditWSTurnContextKey, 2)
	payload := []byte(`{"type":"response.create","response":{"input":"retry flagged"}}`)

	first := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")
	_, cachedAfterFlag := c.Get(securityAuditWSDedupeContextKey)
	second := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")

	require.Equal(t, securityaudit.DecisionFlag, first.Kind)
	require.True(t, first.AllowNextStage)
	require.False(t, cachedAfterFlag)
	require.Equal(t, securityaudit.DecisionAllow, second.Kind)
	require.Equal(t, int64(2), engine.evaluates.Load())
}

func TestRunSecurityAuditLogsWebSocketChecksAndCacheHits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeBlocking}
	coordinator := newPromptPatchTestCoordinator(engine)
	core, logs := observer.New(zap.InfoLevel)
	reqLog := zap.New(core)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(securityAuditWSTurnContextKey, 2)
	payload := []byte(`{"type":"response.create","response":{"input":"same turn"}}`)

	runSecurityAudit(c, reqLog, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")
	runSecurityAudit(c, reqLog, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_chat_websocket", "gpt-test", payload, "subsequent_turn")

	startLogs := logs.FilterMessage("security_audit.gateway_check_start").All()
	require.Len(t, startLogs, 1)
	require.Equal(t, false, startLogs[0].ContextMap()["cached"])

	doneLogs := logs.FilterMessage("security_audit.gateway_check_done").All()
	require.Len(t, doneLogs, 2)
	require.Equal(t, false, doneLogs[0].ContextMap()["cached"])
	require.Equal(t, true, doneLogs[1].ContextMap()["cached"])
	require.Equal(t, "allow", doneLogs[1].ContextMap()["decision"])
	require.Equal(t, "subsequent_turn", doneLogs[1].ContextMap()["stage"])
	require.Equal(t, int64(1), engine.evaluates.Load())
}

type turnCountingEngine struct {
	mode      securityaudit.Mode
	enqueues  atomic.Int64
	evaluates atomic.Int64
	decisions []*securityaudit.PromptDecision
	request   securityaudit.Request
}

type capturingInstructionEngine struct {
	request  securityaudit.Request
	decision *securityaudit.InstructionDecision
	calls    atomic.Int64
}

type promptEligibleInstructionEngine struct {
	capturingInstructionEngine
}

func (e *capturingInstructionEngine) EvaluateInstruction(_ context.Context, request securityaudit.Request) *securityaudit.InstructionDecision {
	e.calls.Add(1)
	e.request = request
	if e.decision != nil {
		return e.decision
	}
	return &securityaudit.InstructionDecision{Allow: true}
}

func (e *promptEligibleInstructionEngine) ResolvePromptAuditRoute(securityaudit.Request) securityaudit.PromptAuditRoute {
	return securityaudit.PromptAuditRoute{
		Eligible: true, AuditSource: securityaudit.PromptAuditSourceInstructionV2,
		InstructionConfigVersion: 1, ClientProfileKey: securityaudit.InstructionClientOther,
		ClientProfileName: "Other", TriggerReason: securityaudit.PromptAuditTriggerNonResponses,
		ModelContractVersion: securityaudit.PromptAuditModelContractVersion,
	}
}

func newPromptPatchTestCoordinator(prompt securityaudit.PromptEngine) *securityaudit.Coordinator {
	return securityaudit.NewCoordinatorWithInstruction(nil, prompt, &promptEligibleInstructionEngine{})
}

func TestRunInstructionAuditRejectsReservedInternalPurposeAtPublicIngress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, withCoordinator := range []bool{false, true} {
		t.Run(map[bool]string{false: "without coordinator", true: "with coordinator"}[withCoordinator], func(t *testing.T) {
			instruction := &capturingInstructionEngine{}
			var coordinator *securityaudit.Coordinator
			if withCoordinator {
				coordinator = securityaudit.NewCoordinatorWithInstruction(nil, nil, instruction)
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("X-ModelPort-Internal-Purpose", "instruction-audit-review")

			decision := runInstructionAudit(c, nil, coordinator, nil, middleware2.AuthSubject{UserID: 7},
				"openai_responses", "gpt-test", []byte(`{"instructions":"exact"}`), false, "http")

			require.NotNil(t, decision)
			require.False(t, decision.AllowNextStage)
			require.NotNil(t, decision.Instruction)
			require.Equal(t, securityaudit.InstructionErrorCodeRejected, decision.ErrorCode)
			require.Zero(t, instruction.calls.Load(), "a reserved internal marker must not recurse into instruction auditing")
		})
	}
}

func TestRunSecurityAuditRejectsPromptReviewPurposeBeforeChatAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	instruction := &promptEligibleInstructionEngine{}
	prompt := &turnCountingEngine{mode: securityaudit.ModeAsync}
	coordinator := securityaudit.NewCoordinatorWithInstruction(nil, prompt, instruction)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("X-ModelPort-Internal-Purpose", "prompt-audit-review")

	decision := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7},
		"openai_chat", "guard-model", []byte(`{"messages":[{"role":"user","content":"review"}]}`), "http")

	require.NotNil(t, decision)
	require.Equal(t, securityaudit.DecisionBlock, decision.Kind)
	require.False(t, decision.AllowNextStage)
	require.Equal(t, "reserved_internal_marker", decision.Instruction.Reason)
	require.Zero(t, instruction.calls.Load(), "reserved review traffic must not enter instruction auditing")
	require.Zero(t, prompt.enqueues.Load(), "reserved review traffic must not recurse into prompt auditing")
	require.Zero(t, prompt.evaluates.Load(), "reserved review traffic must not recurse into prompt auditing")
}

func (e *turnCountingEngine) EffectiveMode() securityaudit.Mode { return e.mode }
func (e *turnCountingEngine) Enqueue(context.Context, securityaudit.Request) error {
	e.enqueues.Add(1)
	return nil
}
func (e *turnCountingEngine) Evaluate(_ context.Context, request securityaudit.Request) (*securityaudit.PromptDecision, error) {
	e.request = request
	call := e.evaluates.Add(1)
	if int(call) <= len(e.decisions) {
		return e.decisions[call-1], nil
	}
	return &securityaudit.PromptDecision{Kind: securityaudit.DecisionAllow, AllowNextStage: true}, nil
}
