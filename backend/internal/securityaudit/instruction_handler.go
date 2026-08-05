package securityaudit

import (
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type InstructionAdminHandler struct{ service *InstructionService }

func NewInstructionAdminHandler(service *InstructionService) *InstructionAdminHandler {
	return &InstructionAdminHandler{service: service}
}

func (h *InstructionAdminHandler) GetOverview(c *gin.Context) {
	overview, err := h.service.Overview(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func (h *InstructionAdminHandler) UpdateEnabled(c *gin.Context) {
	var request UpdateInstructionEnabledRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_enabled_request", nil)
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_enabled_request", "开关请求无效"))
		return
	}
	overview, before, err := h.service.UpdateEnabled(c.Request.Context(), request)
	fields := map[string]any{"before": before, "after": request.Enabled}
	if overview != nil {
		fields["config_version"] = overview.ConfigVersion
	}
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), fields)
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", fields)
	response.Success(c, overview)
}

func (h *InstructionAdminHandler) ListHashes(c *gin.Context) {
	items, err := h.service.ListHashes(c.Request.Context(), c.Query("status"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *InstructionAdminHandler) CreateHash(c *gin.Context) {
	var request CreateInstructionHashRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_hash_request", nil)
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_hash_request", "哈希请求无效"))
		return
	}
	item, err := h.service.CreateHash(c.Request.Context(), request, adminID(c))
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"status": request.Status})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"hash_id": item.ID, "status": item.Status})
	response.Success(c, item)
}

func (h *InstructionAdminHandler) UpdateHash(c *gin.Context) {
	id, ok := instructionIDParam(c, "hash")
	if !ok {
		return
	}
	var request UpdateInstructionHashRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_hash_request", map[string]any{"hash_id": id})
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_hash_request", "哈希请求无效"))
		return
	}
	item, err := h.service.UpdateHash(c.Request.Context(), id, request)
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"hash_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"hash_id": id, "status": item.Status})
	response.Success(c, item)
}

func (h *InstructionAdminHandler) ListRuleSets(c *gin.Context) {
	items, err := h.service.ListRuleSets(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *InstructionAdminHandler) CreateRuleSet(c *gin.Context) {
	h.saveRuleSet(c, 0)
}

func (h *InstructionAdminHandler) UpdateRuleSet(c *gin.Context) {
	id, ok := instructionIDParam(c, "rule_set")
	if !ok {
		return
	}
	h.saveRuleSet(c, id)
}

func (h *InstructionAdminHandler) saveRuleSet(c *gin.Context, id int64) {
	var request SaveInstructionRuleSetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_rule_set_request", map[string]any{"rule_set_id": id})
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_rule_set_request", "规则集请求无效"))
		return
	}
	item, err := h.service.SaveRuleSet(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"rule_set_id": id, "hash_count": len(request.HashIDs)})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"rule_set_id": item.ID, "hash_count": len(item.Hashes), "enabled": item.Enabled})
	response.Success(c, item)
}

func (h *InstructionAdminHandler) ListGroupBindings(c *gin.Context) {
	items, err := h.service.ListGroupBindings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *InstructionAdminHandler) SaveGroupBindings(c *gin.Context) {
	var request SaveInstructionGroupBindingsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_group_binding_request", nil)
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_group_binding_request", "分组绑定请求无效"))
		return
	}
	items, err := h.service.SaveGroupBindings(c.Request.Context(), request, adminID(c))
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"group_count": len(request.GroupIDs), "rule_set_id": request.RuleSetID})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"binding_count": len(items), "group_ids": request.GroupIDs, "rule_set_id": request.RuleSetID})
	response.Success(c, items)
}

func (h *InstructionAdminHandler) DeleteGroupBinding(c *gin.Context) {
	id, ok := instructionIDParam(c, "group_binding")
	if !ok {
		return
	}
	if err := h.service.DeleteGroupBinding(c.Request.Context(), id); err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"group_binding_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"group_binding_id": id})
	response.Success(c, gin.H{"deleted": true})
}

func (h *InstructionAdminHandler) ListGroupOptions(c *gin.Context) {
	items, err := h.service.ListGroupOptions(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *InstructionAdminHandler) ListEvents(c *gin.Context) {
	page, err := positiveIntQuery(c, "page", 1, 0)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	pageSize, err := positiveIntQuery(c, "page_size", 20, 100)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	userID, err := optionalInstructionUserID(c.Query("user_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items, err := h.service.ListEvents(c.Request.Context(), page, pageSize, userID, c.Query("model"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *InstructionAdminHandler) GetEvent(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	item, err := h.service.GetEvent(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *InstructionAdminHandler) CreateCandidate(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	var request CreateInstructionCandidateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_candidate_request", map[string]any{"event_id": id})
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_candidate_request", "候选哈希请求无效"))
		return
	}
	item, err := h.service.CreateCandidateFromEvent(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"event_id": id, "source": request.Source})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"event_id": id, "hash_id": item.ID, "source": request.Source})
	response.Success(c, item)
}

func instructionIDParam(c *gin.Context, resource string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_"+resource+"_id", "ID 无效"))
		return 0, false
	}
	return id, true
}

func optionalInstructionUserID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, infraerrors.BadRequest("instruction_audit_invalid_user_id", "用户 ID 无效")
	}
	return id, nil
}

func setInstructionAdminAudit(c *gin.Context, result, errorCode string, fields map[string]any) {
	details := make(map[string]any, len(fields)+2)
	details["result"] = result
	if errorCode != "" {
		details["error_code"] = errorCode
	}
	for key, value := range fields {
		details[key] = value
	}
	middleware.SetAuditExtra(c, details)
}

func (h *InstructionAdminHandler) setAdminAudit(c *gin.Context, result, errorCode string, fields map[string]any) {
	details := make(map[string]any, len(fields)+1)
	for key, value := range fields {
		details[key] = value
	}
	if h != nil && h.service != nil {
		details["config_version"] = h.service.ConfigVersion()
	}
	setInstructionAdminAudit(c, result, errorCode, details)
}
