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

func TestModelCatalogRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{
		ModelCatalog: handler.NewModelCatalogHandler(nil),
		Admin: &handler.AdminHandlers{
			ModelCatalog: adminhandler.NewModelCatalogHandler(nil),
		},
	}
	allow := func(context *gin.Context) { context.Next() }
	RegisterUserRoutes(
		router.Group("/api/v1"), handlers,
		servermiddleware.JWTAuthMiddleware(allow),
		servermiddleware.AuditLogMiddleware(allow), nil,
	)
	RegisterAdminRoutes(
		router.Group("/api/v1"), handlers,
		servermiddleware.AdminAuthMiddleware(allow),
		servermiddleware.AuditLogMiddleware(allow),
		servermiddleware.StepUpAuthMiddleware(allow), nil,
	)

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	require.True(t, registered["GET /api/v1/model-catalog"])
	require.True(t, registered["GET /api/v1/admin/model-catalog"])
	require.True(t, registered["PUT /api/v1/admin/model-catalog"])
	require.True(t, registered["DELETE /api/v1/admin/model-catalog/:id"])
}

func TestModelCatalogRoutesEnforceAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{
		ModelCatalog: handler.NewModelCatalogHandler(nil),
		Admin: &handler.AdminHandlers{
			ModelCatalog: adminhandler.NewModelCatalogHandler(nil),
		},
	}
	reject := func(context *gin.Context) {
		servermiddleware.AbortWithError(context, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
	}
	allow := func(context *gin.Context) { context.Next() }
	RegisterUserRoutes(
		router.Group("/api/v1"), handlers,
		servermiddleware.JWTAuthMiddleware(reject),
		servermiddleware.AuditLogMiddleware(allow), nil,
	)
	RegisterAdminRoutes(
		router.Group("/api/v1"), handlers,
		servermiddleware.AdminAuthMiddleware(reject),
		servermiddleware.AuditLogMiddleware(allow),
		servermiddleware.StepUpAuthMiddleware(allow), nil,
	)

	for _, path := range []string{"/api/v1/model-catalog", "/api/v1/admin/model-catalog"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusUnauthorized, recorder.Code, path)
	}
}

func TestModelCatalogAdminWriteUsesAuditMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		ModelCatalog: adminhandler.NewModelCatalogHandler(nil),
	}}
	auditCalls := 0
	allow := func(context *gin.Context) { context.Next() }
	audit := func(context *gin.Context) {
		auditCalls++
		context.Next()
	}
	RegisterAdminRoutes(
		router.Group("/api/v1"), handlers,
		servermiddleware.AdminAuthMiddleware(allow),
		servermiddleware.AuditLogMiddleware(audit),
		servermiddleware.StepUpAuthMiddleware(allow), nil,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/model-catalog", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, 1, auditCalls)
}
