package routes

import (
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
	openAIHandler, err := os.ReadFile("../../handler/openai_gateway_handler.go")
	require.NoError(t, err)
	openAISource := string(openAIHandler)
	responsesStart := strings.Index(openAISource, "func (h *OpenAIGatewayHandler) Responses(c *gin.Context)")
	require.NotEqual(t, -1, responsesStart)
	responsesEnd := strings.Index(openAISource[responsesStart:], "func isOpenAIRemoteCompactPath")
	require.NotEqual(t, -1, responsesEnd)
	responses := openAISource[responsesStart : responsesStart+responsesEnd]
	requireInstructionSourceOrder(t, responses, "checkInstructionAudit", "gjson.ValidBytes")
	requireInstructionSourceOrder(t, responses, "checkInstructionAudit", "SelectAccount")
	requireInstructionSourceOrder(t, responses, "checkInstructionAudit", "CheckBillingEligibility")
	require.Contains(t, responses, "checkSecurityAuditAfterInstruction")
	require.Contains(t, responses, "isOpenAIInstructionAuditExcluded(c, false)")
	require.NotContains(t, responses, "HasCompactionTriggerInInput")

	wsStart := strings.Index(openAISource, "func (h *OpenAIGatewayHandler) ResponsesWebSocket")
	require.NotEqual(t, -1, wsStart)
	ws := openAISource[wsStart:]
	requireInstructionSourceOrder(t, ws, "checkInstructionAudit", "gjson.ValidBytes(firstMessage)")
	require.Contains(t, ws, `"first_turn"`)
	require.Contains(t, ws, `"subsequent_turn"`)
	require.Contains(t, ws, "checkSecurityAuditAfterInstruction")
	require.Contains(t, ws, "isOpenAIInstructionAuditExcluded(c, true)")
	require.NotContains(t, ws, "service.HasCompactionTriggerInInput")

	exclusionStart := strings.Index(openAISource, "func isOpenAIInstructionAuditExcluded")
	require.NotEqual(t, -1, exclusionStart)
	exclusionEnd := strings.Index(openAISource[exclusionStart:], "func isBareOpenAIResponsesPath")
	require.NotEqual(t, -1, exclusionEnd)
	exclusion := openAISource[exclusionStart : exclusionStart+exclusionEnd]
	require.Contains(t, exclusion, "!websocket && isOpenAIRemoteCompactPath(c)")
	require.NotContains(t, exclusion, "HasCompactionTriggerInInput")

	anthropicHandler, err := os.ReadFile("../../handler/gateway_handler_responses.go")
	require.NoError(t, err)
	anthropic := string(anthropicHandler)
	requireInstructionSourceOrder(t, anthropic, "checkInstructionAudit", "gjson.ValidBytes")
	requireInstructionSourceOrder(t, anthropic, "checkInstructionAudit", "SelectAccount")

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
		InstructionAudit: securityaudit.NewInstructionAdminHandler(nil),
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
