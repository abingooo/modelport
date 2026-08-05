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
)

func TestCachesSecurityAuditCompletionSkipsWebSocketStages(t *testing.T) {
	require.True(t, cachesSecurityAuditCompletion("http"))
	require.True(t, cachesSecurityAuditCompletion(""))
	require.False(t, cachesSecurityAuditCompletion("first_turn"))
	require.False(t, cachesSecurityAuditCompletion("subsequent_turn"))
}

func TestRunSecurityAuditDoesNotSkipSubsequentWebSocketTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeAsync}
	coordinator := securityaudit.NewCoordinator(nil, engine)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	subject := middleware2.AuthSubject{UserID: 7, Concurrency: 1}
	first := runSecurityAudit(c, nil, coordinator, nil, nil, subject, "openai_responses", "gpt-test",
		[]byte(`{"type":"response.create","response":{"input":"benign"}}`), "first_turn")
	require.NotNil(t, first)
	require.True(t, first.AllowNextStage)
	require.Equal(t, int64(1), engine.enqueues.Load())
	_, cached := c.Get(securityAuditCompletedContextKey)
	require.False(t, cached, "WebSocket stages must not set the HTTP completion cache")

	// Even if an HTTP path previously cached completion on this Context, WS turns
	// must still audit every response.create payload.
	c.Set(securityAuditCompletedContextKey, true)

	second := runSecurityAudit(c, nil, coordinator, nil, nil, subject, "openai_responses", "gpt-test",
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

type turnCountingEngine struct {
	mode     securityaudit.Mode
	enqueues atomic.Int64
}

type capturingInstructionEngine struct{ request securityaudit.Request }

func (e *capturingInstructionEngine) EvaluateInstruction(_ context.Context, request securityaudit.Request) *securityaudit.InstructionDecision {
	e.request = request
	return &securityaudit.InstructionDecision{Allow: true}
}

func (e *turnCountingEngine) EffectiveMode() securityaudit.Mode { return e.mode }
func (e *turnCountingEngine) Enqueue(context.Context, securityaudit.Request) error {
	e.enqueues.Add(1)
	return nil
}
func (e *turnCountingEngine) Evaluate(context.Context, securityaudit.Request) (*securityaudit.PromptDecision, error) {
	return &securityaudit.PromptDecision{Kind: securityaudit.DecisionAllow, AllowNextStage: true}, nil
}
