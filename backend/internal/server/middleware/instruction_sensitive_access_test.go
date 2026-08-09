package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type instructionSensitiveAuthorizerFunc func(context.Context, int64, string) (int64, error)

func (f instructionSensitiveAuthorizerFunc) AuthorizeInstructionSensitiveAccess(
	ctx context.Context,
	userID int64,
	authMethod string,
) (int64, error) {
	return f(ctx, userID, authMethod)
}

func instructionSensitiveTestRouter(
	authMethod string,
	authorizer InstructionSensitiveAccessAuthorizer,
	handler gin.HandlerFunc,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 42})
		c.Set(string(ContextKeyUserRole), service.RoleAdmin)
		c.Set("auth_method", authMethod)
		c.Next()
	})
	router.GET("/sensitive", gin.HandlerFunc(NewInstructionSensitiveAccessMiddleware(authorizer)), handler)
	return router
}

func TestInstructionSensitiveAccessMiddlewareStoresExactGrant(t *testing.T) {
	authorizer := instructionSensitiveAuthorizerFunc(func(_ context.Context, userID int64, authMethod string) (int64, error) {
		require.EqualValues(t, 42, userID)
		require.Equal(t, service.AuditAuthMethodJWT, authMethod)
		return 91, nil
	})
	router := instructionSensitiveTestRouter(service.AuditAuthMethodJWT, authorizer, func(c *gin.Context) {
		authorization, ok := InstructionSensitiveAuthorizationFromContext(c.Request.Context())
		require.True(t, ok)
		require.EqualValues(t, 91, authorization.GrantID)
		require.EqualValues(t, 42, authorization.UserID)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sensitive", nil))
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestInstructionSensitiveAccessMiddlewareRejectsAdminAPIKey(t *testing.T) {
	called := false
	authorizer := instructionSensitiveAuthorizerFunc(func(context.Context, int64, string) (int64, error) {
		called = true
		return 0, nil
	})
	router := instructionSensitiveTestRouter(service.AuditAuthMethodAdminAPIKey, authorizer, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sensitive", nil))
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.False(t, called)
	require.Contains(t, recorder.Body.String(), "STEP_UP_ADMIN_API_KEY_FORBIDDEN")
}

func TestInstructionSensitiveAccessMiddlewareFailsClosedWithoutGrant(t *testing.T) {
	authorizer := instructionSensitiveAuthorizerFunc(func(context.Context, int64, string) (int64, error) {
		return 0, infraerrors.Forbidden(
			"INSTRUCTION_SENSITIVE_ACCESS_REQUIRED", "Sensitive instruction content access is required",
		)
	})
	router := instructionSensitiveTestRouter(service.AuditAuthMethodJWT, authorizer, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sensitive", nil))
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "INSTRUCTION_SENSITIVE_ACCESS_REQUIRED")
}
