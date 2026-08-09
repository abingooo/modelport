package middleware

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type InstructionSensitiveAccessAuthorizer interface {
	AuthorizeInstructionSensitiveAccess(ctx context.Context, userID int64, authMethod string) (int64, error)
}

type InstructionSensitiveAccessMiddleware gin.HandlerFunc

type InstructionSensitiveAuthorization struct {
	GrantID             int64
	UserID              int64
	AuthMethod          string
	AuthorizationResult string
}

type instructionSensitiveAuthorizationContextKey struct{}

func WithInstructionSensitiveAuthorization(
	ctx context.Context,
	authorization InstructionSensitiveAuthorization,
) context.Context {
	return context.WithValue(ctx, instructionSensitiveAuthorizationContextKey{}, authorization)
}

func InstructionSensitiveAuthorizationFromContext(ctx context.Context) (InstructionSensitiveAuthorization, bool) {
	if ctx == nil {
		return InstructionSensitiveAuthorization{}, false
	}
	authorization, ok := ctx.Value(instructionSensitiveAuthorizationContextKey{}).(InstructionSensitiveAuthorization)
	return authorization, ok
}

func NewInstructionSensitiveAccessMiddleware(
	authorizer InstructionSensitiveAccessAuthorizer,
) InstructionSensitiveAccessMiddleware {
	return InstructionSensitiveAccessMiddleware(func(c *gin.Context) {
		authMethod := c.GetString("auth_method")
		if authMethod == service.AuditAuthMethodAdminAPIKey {
			SetAuditExtra(c, map[string]any{
				"auth_method": authMethod, "authorization_result": "admin_api_key_denied",
			})
			AbortWithError(c, 403, "STEP_UP_ADMIN_API_KEY_FORBIDDEN",
				"Admin API key cannot access this endpoint; a two-factor verified admin session is required")
			return
		}
		if authMethod != service.AuditAuthMethodJWT {
			SetAuditExtra(c, map[string]any{
				"auth_method": authMethod, "authorization_result": "human_session_required",
			})
			response.ErrorFrom(c, infraerrors.Forbidden(
				"INSTRUCTION_SENSITIVE_HUMAN_SESSION_REQUIRED",
				"A signed-in administrator session is required for sensitive instruction content",
			))
			return
		}

		subject, ok := GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
			return
		}
		if authorizer == nil {
			SetAuditExtra(c, map[string]any{"authorization_result": "unavailable"})
			response.ErrorFrom(c, infraerrors.ServiceUnavailable(
				"INSTRUCTION_SENSITIVE_ACCESS_UNAVAILABLE", "Sensitive content authorization is unavailable",
			))
			return
		}

		grantID, err := authorizer.AuthorizeInstructionSensitiveAccess(
			c.Request.Context(), subject.UserID, authMethod,
		)
		if err != nil {
			SetAuditExtra(c, map[string]any{
				"auth_method": authMethod, "authorization_result": "denied",
			})
			response.ErrorFrom(c, err)
			return
		}

		authorization := InstructionSensitiveAuthorization{
			GrantID: grantID, UserID: subject.UserID, AuthMethod: authMethod,
			AuthorizationResult: "granted",
		}
		c.Request = c.Request.WithContext(WithInstructionSensitiveAuthorization(
			c.Request.Context(), authorization,
		))
		SetAuditExtra(c, map[string]any{
			"grant_id": grantID, "auth_method": authMethod, "authorization_result": "granted",
		})
		c.Next()
	})
}
