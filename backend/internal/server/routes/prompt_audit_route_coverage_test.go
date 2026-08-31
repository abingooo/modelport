package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayRoute struct {
	method string
	path   string
}

type promptAuditRouteClassification struct {
	handlerFiles []string
	reason       string
	transport    string
}

const (
	promptAuditHTTP      = "http"
	promptAuditHTTPSSE   = "http_sse"
	promptAuditWebSocket = "websocket"
)

// TestEveryGatewayRouteIsClassifiedForPromptAuditCoverage is intentionally
// source-based. Gateway routes are registered through several groups and
// aliases, so checking only the literal path (or only POST calls) can silently
// collapse distinct routes and leave a new GET/PATCH/DELETE/WS ingress out of
// the audit review. Keep the receiver prefixes here in sync with gateway.go.
func TestEveryGatewayPOSTRouteIsClassifiedForPromptAuditCoverage(t *testing.T) {
	routeSource, err := os.ReadFile("gateway.go")
	require.NoError(t, err)
	actual := enumerateGatewayRoutes(string(routeSource))
	require.NotEmpty(t, actual)

	classifications := promptAuditRouteManifest()
	actualKeys := make(map[string]struct{}, len(actual))
	methods := make(map[string]struct{})
	for _, route := range actual {
		key := gatewayRouteKey(route)
		actualKeys[key] = struct{}{}
		methods[route.method] = struct{}{}
	}
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		_, present := methods[method]
		require.Truef(t, present, "gateway route inventory must scan %s registrations", method)
	}

	unclassified := make([]string, 0)
	for key := range actualKeys {
		if _, ok := classifications[key]; !ok {
			unclassified = append(unclassified, key)
		}
	}
	sort.Strings(unclassified)
	require.Empty(t, unclassified, "every gateway route must be audited or explicitly classified with a no-prompt reason")

	stale := make([]string, 0)
	for key := range classifications {
		if _, exists := actualKeys[key]; !exists {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	require.Empty(t, stale, "prompt-audit route manifest contains stale entries")

	for key, classification := range classifications {
		if len(classification.handlerFiles) == 0 {
			require.NotEmptyf(t, strings.TrimSpace(classification.reason), "%s has no handler and needs an exclusion reason", key)
			require.NotEmptyf(t, strings.TrimSpace(classification.transport), "%s needs an explicit transport classification", key)
			continue
		}
		require.Emptyf(t, strings.TrimSpace(classification.reason), "%s is both audited and excluded", key)
		for _, filename := range classification.handlerFiles {
			source, readErr := os.ReadFile(filepath.Join("..", "..", "handler", filename))
			require.NoError(t, readErr)
			require.Containsf(t, string(source), "checkSecurityAudit", "%s route handler %s bypasses Coordinator", key, filename)
		}
		require.NotEmptyf(t, strings.TrimSpace(classification.transport), "%s needs an explicit transport classification", key)
	}

	for _, key := range []string{
		"GET /v1/responses", "GET /responses", "GET /backend-api/codex/responses",
		"GET /v1/realtime", "GET /realtime",
	} {
		classification, ok := classifications[key]
		require.Truef(t, ok, "missing WebSocket route classification for %s", key)
		require.Equalf(t, promptAuditWebSocket, classification.transport, "%s must be classified as a WebSocket ingress", key)
	}
}

var gatewayRoutePattern = regexp.MustCompile(`(?m)\b(gateway|gemini|r|codexDirect|antigravityV1|antigravityV1Beta)\.(GET|POST|PATCH|DELETE|PUT|OPTIONS|HEAD|Any|Handle|HandleFunc)\("([^"]+)"`)

func enumerateGatewayRoutes(source string) []gatewayRoute {
	prefixes := map[string]string{
		"gateway":           "/v1",
		"gemini":            "/v1beta",
		"r":                 "",
		"codexDirect":       "/backend-api/codex",
		"antigravityV1":     "/antigravity/v1",
		"antigravityV1Beta": "/antigravity/v1beta",
	}
	seen := make(map[string]struct{})
	routes := make([]gatewayRoute, 0)
	for _, match := range gatewayRoutePattern.FindAllStringSubmatch(source, -1) {
		prefix := prefixes[match[1]]
		path := strings.TrimSpace(match[3])
		if path == "" {
			path = "/"
		}
		path = strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/")
		path = strings.ReplaceAll(path, "//", "/")
		route := gatewayRoute{method: match[2], path: path}
		key := gatewayRouteKey(route)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool { return gatewayRouteKey(routes[i]) < gatewayRouteKey(routes[j]) })
	return routes
}

func gatewayRouteKey(route gatewayRoute) string {
	return route.method + " " + route.path
}

func promptAuditRouteManifest() map[string]promptAuditRouteClassification {
	manifest := make(map[string]promptAuditRouteClassification)
	audited := func(method, transport string, paths []string, files ...string) {
		for _, path := range paths {
			manifest[gatewayRouteKey(gatewayRoute{method: method, path: path})] = promptAuditRouteClassification{
				handlerFiles: append([]string(nil), files...), transport: transport,
			}
		}
	}
	excludedWithTransport := func(method, transport string, paths []string, reason string) {
		for _, path := range paths {
			manifest[gatewayRouteKey(gatewayRoute{method: method, path: path})] = promptAuditRouteClassification{
				reason: reason, transport: transport,
			}
		}
	}
	excluded := func(method string, paths []string, reason string) {
		excludedWithTransport(method, promptAuditHTTP, paths, reason)
	}

	// Text-bearing model-generation routes. Every alias is listed with its
	// fully-qualified path so method/path collisions (for example POST and GET
	// /v1/responses or /v1/images/batches) remain independently reviewable.
	audited("POST", promptAuditHTTP, []string{
		"/v1/messages", "/antigravity/v1/messages",
	}, "gateway_handler.go", "openai_gateway_handler.go")
	audited("POST", promptAuditHTTPSSE, []string{
		"/v1/responses", "/v1/responses/*subpath", "/responses", "/responses/*subpath",
		"/backend-api/codex/responses", "/backend-api/codex/responses/*subpath",
	}, "gateway_handler_responses.go", "openai_gateway_handler.go")
	audited("GET", promptAuditWebSocket, []string{
		"/v1/responses", "/responses", "/backend-api/codex/responses",
	}, "openai_gateway_handler.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/alpha/search", "/alpha/search", "/backend-api/codex/alpha/search",
	}, "openai_alpha_search.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/live", "/backend-api/codex/realtime/calls",
	}, "openai_live.go")
	audited("POST", promptAuditHTTPSSE, []string{
		"/v1/chat/completions", "/chat/completions",
	}, "gateway_handler_chat_completions.go", "openai_chat_completions.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/embeddings", "/embeddings",
	}, "openai_embeddings.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/images/generations", "/v1/images/edits", "/images/generations", "/images/edits",
	}, "openai_images.go", "grok_media.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/images/generations/async", "/v1/images/edits/async",
		"/images/generations/async", "/images/edits/async",
	}, "image_task_handler.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/images/batches",
	}, "batch_image_handler.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/videos", "/v1/videos/generations", "/v1/videos/edits", "/v1/videos/extensions",
		"/videos", "/videos/generations", "/videos/edits", "/videos/extensions",
	}, "grok_media.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/tts", "/tts",
	}, "grok_audio.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1/web_search", "/web_search", "/v1/x_search", "/x_search",
	}, "gateway_web_search.go")
	audited("POST", promptAuditHTTP, []string{
		"/v1beta/models/*modelAction", "/antigravity/v1beta/models/*modelAction",
	}, "gemini_v1beta_handler.go")

	// Explicit no-prompt/control-plane routes. Reasons describe the payload
	// contract, rather than inferring behavior from the URL name alone.
	excluded("GET", []string{"/v1/sub2api/billing", "/v1/models", "/models", "/backend-api/codex/models", "/v1beta/models", "/antigravity/models", "/antigravity/v1/models", "/antigravity/v1beta/models", "/v1beta/models/:model", "/antigravity/v1beta/models/:model"}, "metadata, model-list, or billing lookup; no model prompt is accepted")
	excluded("GET", []string{"/v1/usage", "/antigravity/v1/usage"}, "usage lookup; no model prompt is accepted")
	excluded("POST", []string{"/v1/messages/count_tokens", "/messages/count_tokens", "/antigravity/v1/messages/count_tokens"}, "tokenization-only endpoint; it does not execute a model request")
	excluded("GET", []string{"/v1/live/:call_id", "/backend-api/codex/:call_id"}, "Live/WebRTC sideband transport lookup; session text was audited at call creation")
	excluded("POST", []string{"/v1/images/batches/:id/cancel"}, "batch control-plane cancellation; no new user prompt is accepted")
	excluded("DELETE", []string{"/v1/images/batches/:id", "/v1/images/batches/:id/outputs"}, "batch record/output deletion; no model prompt is accepted")
	excluded("GET", []string{"/v1/images/tasks/:task_id", "/images/tasks/:task_id"}, "asynchronous image result lookup; no new prompt is accepted")
	excluded("GET", []string{
		"/v1/images/batches", "/v1/images/batches/models", "/v1/images/batches/:id", "/v1/images/batches/:id/items",
		"/v1/images/batches/:id/items/:custom_id/content", "/v1/images/batches/:id/download",
	}, "batch status, item, or download lookup; no new model prompt is accepted")
	excluded("GET", []string{
		"/v1/videos/generations/:request_id/content", "/videos/generations/:request_id/content",
		"/v1/videos/edits/:request_id/content", "/videos/edits/:request_id/content",
		"/v1/videos/extensions/:request_id/content", "/videos/extensions/:request_id/content",
		"/v1/videos/:request_id/content", "/videos/:request_id/content",
		"/v1/videos/generations/:request_id", "/videos/generations/:request_id",
		"/v1/videos/edits/:request_id", "/videos/edits/:request_id",
		"/v1/videos/extensions/:request_id", "/videos/extensions/:request_id",
		"/v1/videos/:request_id", "/videos/:request_id",
	}, "video status/content lookup; no new model prompt is accepted")
	excluded("POST", []string{"/v1/stt", "/stt"}, "speech transcription consumes audio; it is not a text-generation prompt")
	excluded("POST", []string{"/v1/custom-voices", "/custom-voices"}, "voice profile creation consumes voice metadata/audio, not a model prompt")
	excluded("GET", []string{
		"/v1/custom-voices", "/custom-voices", "/v1/custom-voices/:voice_id/audio", "/custom-voices/:voice_id/audio",
		"/v1/custom-voices/:voice_id", "/custom-voices/:voice_id",
	}, "voice profile or audio retrieval; no model prompt is accepted")
	excluded("PATCH", []string{"/v1/custom-voices/:voice_id", "/custom-voices/:voice_id"}, "voice profile update; no model prompt is accepted")
	excluded("DELETE", []string{"/v1/custom-voices/:voice_id", "/custom-voices/:voice_id"}, "voice profile deletion; no model prompt is accepted")
	excludedWithTransport("GET", promptAuditWebSocket, []string{"/v1/realtime", "/realtime"}, "native Grok Realtime WebSocket carries audio frames; no text prompt is accepted at this ingress")
	return manifest
}

func TestResponsesWebSocketHasFirstAndSubsequentTurnPromptGates(t *testing.T) {
	routeSource, err := os.ReadFile("gateway.go")
	require.NoError(t, err)
	require.GreaterOrEqual(t, strings.Count(string(routeSource), `.GET("/responses"`), 2)
	handlerSource, err := os.ReadFile(filepath.Join("..", "..", "handler", "openai_gateway_handler.go"))
	require.NoError(t, err)
	require.Contains(t, string(handlerSource), `checkSecurityAuditAfterInstruction`)
	require.Contains(t, string(handlerSource), `"first_turn"`)
	require.Contains(t, string(handlerSource), `"subsequent_turn"`)
	wsStart := strings.Index(string(handlerSource), `func (h *OpenAIGatewayHandler) ResponsesWebSocket`)
	require.NotEqual(t, -1, wsStart)
	wsSource := string(handlerSource)[wsStart:]
	require.Less(t,
		strings.Index(wsSource, `"first_turn"`),
		strings.Index(wsSource, `TryAcquireUserSlotForAPIKey`),
		"the first response.create gate must precede per-request user/account slots",
	)
}

func TestPromptAuditAdminRoutesRejectUnauthenticatedAndNonAdminRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		PromptAudit: securityaudit.NewPromptAdminHandler(nil),
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

	for _, tc := range []struct {
		name       string
		auth       string
		wantStatus int
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
		{name: "non-admin", auth: "Bearer user-token", wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/prompt-audit/config", nil)
			if tc.auth != "" {
				request.Header.Set("Authorization", tc.auth)
			}
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.wantStatus, recorder.Code)
		})
	}
}
