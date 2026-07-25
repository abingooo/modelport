package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func lotteryRouteHandlers() *handler.Handlers {
	return &handler.Handlers{
		Lottery: handler.NewLotteryHandler(nil),
		Admin: &handler.AdminHandlers{
			Lottery: adminhandler.NewLotteryHandler(nil),
		},
	}
}

func TestLotteryRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	allow := func(context *gin.Context) { context.Next() }
	RegisterUserRoutes(router.Group("/api/v1"), lotteryRouteHandlers(),
		servermiddleware.JWTAuthMiddleware(allow), servermiddleware.AuditLogMiddleware(allow), nil)
	RegisterAdminRoutes(router.Group("/api/v1"), lotteryRouteHandlers(),
		servermiddleware.AdminAuthMiddleware(allow), servermiddleware.AuditLogMiddleware(allow),
		servermiddleware.StepUpAuthMiddleware(allow), nil)

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/v1/lottery", "GET /api/v1/lottery/history", "GET /api/v1/lottery/:id",
		"POST /api/v1/lottery/:id/participate", "GET /api/v1/admin/lottery",
		"POST /api/v1/admin/lottery", "GET /api/v1/admin/lottery/:id",
		"PUT /api/v1/admin/lottery/:id", "PUT /api/v1/admin/lottery/:id/status",
		"DELETE /api/v1/admin/lottery/:id", "GET /api/v1/admin/lottery/:id/entries",
		"POST /api/v1/admin/lottery/:id/draw",
	} {
		require.True(t, registered[route], route)
	}
}

func TestLotteryRoutesEnforceAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	reject := func(context *gin.Context) {
		servermiddleware.AbortWithError(context, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
	}
	allow := func(context *gin.Context) { context.Next() }
	RegisterUserRoutes(router.Group("/api/v1"), lotteryRouteHandlers(),
		servermiddleware.JWTAuthMiddleware(reject), servermiddleware.AuditLogMiddleware(allow), nil)
	RegisterAdminRoutes(router.Group("/api/v1"), lotteryRouteHandlers(),
		servermiddleware.AdminAuthMiddleware(reject), servermiddleware.AuditLogMiddleware(allow),
		servermiddleware.StepUpAuthMiddleware(allow), nil)

	for _, path := range []string{"/api/v1/lottery", "/api/v1/admin/lottery"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusUnauthorized, recorder.Code, path)
	}
}

func TestLotteryWritesUseAuditMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	auditCalls := 0
	allow := func(context *gin.Context) { context.Next() }
	audit := func(context *gin.Context) {
		auditCalls++
		context.Next()
	}
	RegisterAdminRoutes(router.Group("/api/v1"), lotteryRouteHandlers(),
		servermiddleware.AdminAuthMiddleware(allow), servermiddleware.AuditLogMiddleware(audit),
		servermiddleware.StepUpAuthMiddleware(allow), nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/lottery", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Equal(t, 1, auditCalls)
}
