package routes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func requireInstructionSourceOrder(t *testing.T, source, before, after string) {
	t.Helper()
	beforeIndex := strings.Index(source, before)
	afterIndex := strings.Index(source, after)
	require.NotEqualf(t, -1, beforeIndex, "missing source marker %q", before)
	require.NotEqualf(t, -1, afterIndex, "missing source marker %q", after)
	require.Lessf(t, beforeIndex, afterIndex, "%q must appear before %q", before, after)
}

func TestInstructionAuditCoversResponsesHTTPAliasesAndWebSocketBeforeDispatch(t *testing.T) {
	responses := instructionRouteFunctionSource(t, "../../handler/openai_gateway_handler.go", "Responses")
	requireInstructionSourceOrder(t, responses, "checkInstructionAudit(", "gjson.ValidBytes(body)")
	requireInstructionSourceOrder(t, responses, "checkSecurityAuditAfterInstruction(", "SelectAccountWithSchedulerForCapability")
	requireInstructionSourceOrder(t, responses, "checkSecurityAuditAfterInstruction(", "CheckBillingEligibility")
	requireInstructionSourceOrder(t, responses, "checkSecurityAuditAfterInstruction(", "acquireResponsesUserSlot")
	require.Equal(t, 2, strings.Count(responses, "checkInstructionAudit("), "normal and oversized HTTP bodies must reach instruction audit")
	require.Equal(t, 1, strings.Count(responses, "checkSecurityAuditAfterInstruction("))
	require.Contains(t, responses, "readLenientJSONRequestBodyWithAuditSourceBudgetAndLimit")
	require.Contains(t, responses, "service.IsOpenAIResponsesCompactPath(c)")
	require.NotContains(t, responses, "HasCompactionTriggerInInput")

	ws := instructionRouteFunctionSource(t, "../../handler/openai_gateway_handler.go", "ResponsesWebSocket")
	requireInstructionSourceOrder(t, ws, "checkInstructionAudit(", "gjson.ValidBytes(firstMessage)")
	requireInstructionSourceOrder(t, ws, "checkSecurityAuditAfterInstruction(", "TryAcquireUserSlotForAPIKey")
	requireInstructionSourceOrder(t, ws, "checkSecurityAuditAfterInstruction(", "CheckBillingEligibility")
	requireInstructionSourceOrder(t, ws, "checkSecurityAuditAfterInstruction(", "SelectAccountWithSchedulerForCapability")
	require.Equal(t, 3, strings.Count(ws, "checkInstructionAudit("), "oversized, first-turn, and follow-up frames must be wired")
	require.Equal(t, 2, strings.Count(ws, "checkSecurityAuditAfterInstruction("), "first and follow-up turns must run Prompt Audit")
	require.Contains(t, ws, "ReadOpenAIWSClientMessageWithBudget")
	require.Contains(t, ws, "BeforeInstructionRequest:")
	require.Contains(t, ws, `"first_turn"`)
	require.Contains(t, ws, `"subsequent_turn"`)
	require.NotContains(t, ws, "service.HasCompactionTriggerInInput")

	anthropic := instructionRouteFunctionSource(t, "../../handler/gateway_handler_responses.go", "Responses")
	requireInstructionSourceOrder(t, anthropic, "checkInstructionAudit(", "gjson.ValidBytes(body)")
	requireInstructionSourceOrder(t, anthropic, "checkSecurityAuditAfterInstruction(", "SelectAccountWithLoadAwareness(")
	require.Equal(t, 2, strings.Count(anthropic, "checkInstructionAudit("))
	require.Equal(t, 1, strings.Count(anthropic, "checkSecurityAuditAfterInstruction("))

	routesSource, err := os.ReadFile("gateway.go")
	require.NoError(t, err)
	routes := string(routesSource)
	for _, path := range []string{`gateway.POST("/responses"`, `gateway.POST("/responses/*subpath"`, `r.POST("/responses"`, `codexDirect.POST("/responses"`} {
		require.Contains(t, routes, path)
	}
}

func TestInstructionAuditAdminRoutesRejectUnauthenticatedAndNonAdminRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		InstructionAudit: securityaudit.NewInstructionV2AdminHandler(nil),
	}}
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
			return
		}
		servermiddleware.AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Admin access required")
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil, nil)

	for _, test := range []struct {
		name       string
		auth       string
		wantStatus int
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
		{name: "non-admin", auth: "Bearer user-token", wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/instruction-audit/overview", nil)
			if test.auth != "" {
				request.Header.Set("Authorization", test.auth)
			}
			router.ServeHTTP(recorder, request)
			require.Equal(t, test.wantStatus, recorder.Code)
		})
	}
}

func TestInstructionAuditAdminRoutesExposeGroupAndClientScopes(t *testing.T) {
	source, err := os.ReadFile("admin.go")
	require.NoError(t, err)
	routes := string(source)
	for _, route := range []string{
		`instructionAudit.GET("/scopes"`,
		`instructionAudit.POST("/scopes"`,
		`instructionAudit.POST("/scopes/batch"`,
		`instructionAudit.DELETE("/scopes/group/:id"`,
		`instructionAudit.DELETE("/scopes/:id"`,
		`instructionAudit.GET("/client-profiles"`,
		`instructionAudit.GET("/groups"`,
		`instructionAudit.GET("/users"`,
	} {
		require.Contains(t, routes, route)
	}
	require.NotContains(t, routes, `instructionAudit.GET("/rule-sets"`)
	require.NotContains(t, routes, `instructionAudit.GET("/group-bindings"`)
	require.NotContains(t, routes, `/client-profiles/:id/prompt-audit`)
}

func TestInstructionAuditRoutesRemoveModuleSpecificSensitiveAuthorization(t *testing.T) {
	instructionRoutes := instructionRouteFunctionSource(t, "admin.go", "registerInstructionAuditRoutes")
	for _, route := range []string{
		`instructionAudit.POST("/hashes", h.Admin.InstructionAudit.CreateHash)`,
		`instructionAudit.GET("/hashes/:id/raw", h.Admin.InstructionAudit.RevealHashRaw)`,
		`instructionAudit.GET("/risk-hashes", h.Admin.InstructionAudit.ListRiskHashes)`,
		`instructionAudit.GET("/review-jobs", h.Admin.InstructionAudit.ListReviewJobs)`,
	} {
		require.Contains(t, instructionRoutes, route)
	}
	require.NotContains(t, instructionRoutes, "sensitive-access")
	require.NotContains(t, instructionRoutes, "ForceStepUp")
	require.NotContains(t, instructionRoutes, "stepUpAuth")
	handlerSource, err := os.ReadFile("../../securityaudit/instruction_v2_handler.go")
	require.NoError(t, err)
	handlers := string(handlerSource)
	for _, marker := range []string{
		"func (h *InstructionV2AdminHandler) RevealEventEvidence",
		"func (h *InstructionV2AdminHandler) RevealHashRaw",
		"func (h *InstructionV2AdminHandler) RevealRiskHashRaw",
		"func (h *InstructionV2AdminHandler) RevealReviewJobRaw",
	} {
		start := strings.Index(handlers, marker)
		require.NotEqual(t, -1, start)
		window := handlers[start:min(len(handlers), start+1800)]
		require.Contains(t, window, `c.Header("Cache-Control", "no-store")`)
	}
}

func instructionRouteFunctionSource(t *testing.T, filename, functionName string) string {
	t.Helper()
	raw, err := os.ReadFile(filename)
	require.NoError(t, err)
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, filename, raw, 0)
	require.NoError(t, err)
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
		}
		start := files.Position(function.Pos()).Offset
		end := files.Position(function.End()).Offset
		require.Greater(t, end, start)
		return string(raw[start:end])
	}
	t.Fatalf("function %s not found in %s", functionName, filename)
	return ""
}
