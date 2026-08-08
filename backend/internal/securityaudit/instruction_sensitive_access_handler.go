package securityaudit

import (
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type instructionSensitiveGrantRequest struct {
	Reason string `json:"reason"`
}

func (h *InstructionAdminHandler) GetInstructionSensitiveAccessMe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	item, err := h.service.GetInstructionSensitiveCapability(
		c.Request.Context(), adminID(c), c.GetString("auth_method"),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *InstructionAdminHandler) ListInstructionSensitiveAccessGrants(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	items, err := h.service.ListInstructionSensitiveGrants(c.Request.Context(), adminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *InstructionAdminHandler) GrantInstructionSensitiveAccess(c *gin.Context) {
	userID, ok := instructionSensitiveUserIDParam(c)
	if !ok {
		return
	}
	var request instructionSensitiveGrantRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			response.ErrorFrom(c, infraerrors.BadRequest(
				"INSTRUCTION_SENSITIVE_INVALID_GRANT_REQUEST", "Sensitive access grant request is invalid",
			))
			return
		}
	}
	item, err := h.service.GrantInstructionSensitiveAccess(
		c.Request.Context(), adminID(c), userID, request.Reason,
	)
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"user_id": userID})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"user_id": userID, "grant_id": item.ID})
	response.Success(c, item)
}

func (h *InstructionAdminHandler) RevokeInstructionSensitiveAccess(c *gin.Context) {
	userID, ok := instructionSensitiveUserIDParam(c)
	if !ok {
		return
	}
	var request instructionSensitiveGrantRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			response.ErrorFrom(c, infraerrors.BadRequest(
				"INSTRUCTION_SENSITIVE_INVALID_REVOKE_REQUEST", "Sensitive access revoke request is invalid",
			))
			return
		}
	}
	item, err := h.service.RevokeInstructionSensitiveAccess(
		c.Request.Context(), adminID(c), userID, request.Reason,
	)
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"user_id": userID})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"user_id": userID, "grant_id": item.ID})
	response.Success(c, item)
}

func instructionSensitiveUserIDParam(c *gin.Context) (int64, bool) {
	value := strings.TrimSpace(c.Param("user_id"))
	userID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || userID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest(
			"INSTRUCTION_SENSITIVE_INVALID_USER", "Target administrator is invalid",
		))
		return 0, false
	}
	return userID, true
}
