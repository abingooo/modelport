package securityaudit

import (
	"strconv"
	"strings"
	"time"

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

func (h *InstructionAdminHandler) UpdateEvidenceRetention(c *gin.Context) {
	var request UpdateInstructionEvidenceRetentionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_evidence_retention", nil)
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_evidence_retention", "明文保留配置无效"))
		return
	}
	overview, err := h.service.UpdateEvidenceRetention(c.Request.Context(), request.Days)
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"days": request.Days})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"days": request.Days})
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

func (h *InstructionAdminHandler) DeleteHash(c *gin.Context) {
	id, ok := instructionIDParam(c, "hash")
	if !ok {
		return
	}
	if err := h.service.DeleteHash(c.Request.Context(), id); err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"hash_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"hash_id": id})
	response.Success(c, gin.H{"deleted": true})
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

func (h *InstructionAdminHandler) DeleteRuleSet(c *gin.Context) {
	id, ok := instructionIDParam(c, "rule_set")
	if !ok {
		return
	}
	if err := h.service.DeleteRuleSet(c.Request.Context(), id); err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"rule_set_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"rule_set_id": id})
	response.Success(c, gin.H{"deleted": true})
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
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{
			"rule_set_id": id, "hash_count": len(request.HashIDs),
			"allowed_user_count": len(request.AllowedUserIDs), "allow_empty_fields": request.AllowEmptyFields,
		})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{
		"rule_set_id": item.ID, "hash_count": len(item.Hashes), "enabled": item.Enabled,
		"allowed_user_count": len(item.AllowedUsers), "allow_empty_fields": item.AllowEmptyFields,
	})
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
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"group_count": len(request.GroupIDs), "rule_set_id": request.RuleSetID, "client_types": request.ClientTypes})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"binding_count": len(items), "group_ids": request.GroupIDs, "rule_set_id": request.RuleSetID, "client_types": request.ClientTypes})
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
	filter, err := instructionEventFilterFromQuery(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items, err := h.service.ListEvents(c.Request.Context(), page, pageSize, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *InstructionAdminHandler) DeleteEvent(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	result, err := h.service.DeleteEvent(c.Request.Context(), id)
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"event_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"event_id": id, "deleted_events": result.DeletedEvents})
	response.Success(c, result)
}

func (h *InstructionAdminHandler) BatchDeleteEvents(c *gin.Context) {
	var request DeleteInstructionEventsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_delete_batch", nil)
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_delete_batch", "批量删除必须包含 1-500 个事件 ID"))
		return
	}
	result, err := h.service.DeleteEventsByIDs(c.Request.Context(), request.IDs)
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"requested_count": len(request.IDs)})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{
		"requested_count": len(request.IDs), "deleted_events": result.DeletedEvents,
	})
	response.Success(c, result)
}

func (h *InstructionAdminHandler) PreviewDeleteEvents(c *gin.Context) {
	var filter InstructionEventFilter
	if err := c.ShouldBindJSON(&filter); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_delete_preview_invalid", nil)
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_delete_preview_invalid", "删除预览筛选无效"))
		return
	}
	preview, err := h.service.PreviewDeleteEvents(c.Request.Context(), filter)
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), nil)
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{
		"matched_count": preview.MatchedCount, "snapshot_max_id": preview.SnapshotMaxID,
		"filter_hash": preview.FilterHash,
	})
	response.Success(c, preview)
}

func (h *InstructionAdminHandler) DeleteEventsByFilter(c *gin.Context) {
	var request DeleteInstructionEventsByFilterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_delete_confirmation_invalid", nil)
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_delete_confirmation_invalid", "删除确认无效或已过期"))
		return
	}
	result, err := h.service.DeleteEventsByFilter(c.Request.Context(), request)
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{
			"snapshot_max_id": request.SnapshotMaxID, "filter_hash": request.FilterHash,
		})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{
		"snapshot_max_id": request.SnapshotMaxID, "filter_hash": request.FilterHash,
		"deleted_events": result.DeletedEvents,
	})
	response.Success(c, result)
}

func (h *InstructionAdminHandler) RevealEvidence(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	item, err := h.service.RevealEvidence(c.Request.Context(), id, instructionEvidenceAccess(c, "reveal", "all"))
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"event_id": id, "action": "reveal"})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"event_id": id, "action": "reveal", "evidence_status": item.Status})
	response.Success(c, item)
}

func (h *InstructionAdminHandler) RecordEvidenceCopy(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	var request RecordInstructionEvidenceAccessRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_copy_source", "复制内容类型无效"))
		return
	}
	access := instructionEvidenceAccess(c, "copy", strings.TrimSpace(request.Source))
	if err := h.service.RecordEvidenceCopy(c.Request.Context(), id, access); err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{"event_id": id, "action": "copy", "source": request.Source})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{"event_id": id, "action": "copy", "source": request.Source})
	response.Success(c, gin.H{"recorded": true})
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

func (h *InstructionAdminHandler) AddEventToRuleSet(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	var request AddInstructionEventToRuleSetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.setAdminAudit(c, "failed", "instruction_audit_invalid_rule_sources", map[string]any{"event_id": id})
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_invalid_rule_sources", "加入规则集请求无效"))
		return
	}
	result, err := h.service.AddEventToRuleSet(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		h.setAdminAudit(c, "failed", infraerrors.Reason(err), map[string]any{
			"event_id": id, "rule_set_id": request.RuleSetID, "sources": request.Sources,
		})
		response.ErrorFrom(c, err)
		return
	}
	h.setAdminAudit(c, "success", "", map[string]any{
		"event_id": id, "rule_set_id": result.RuleSetID, "hash_ids": result.HashIDs,
		"created_hashes": result.CreatedHashes, "activated_hashes": result.ActivatedHashes,
		"attached_hashes": result.AttachedHashes,
	})
	response.Success(c, result)
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

func instructionEventFilterFromQuery(c *gin.Context) (InstructionEventFilter, error) {
	filter := InstructionEventFilter{
		Query: c.Query("q"), Model: c.Query("model"), Reasons: splitInstructionQuery(c.Query("reasons")),
		InstructionResults: splitInstructionQuery(c.Query("instructions_results")),
		Input1Results:      splitInstructionQuery(c.Query("input1_results")),
		UserNotifications:  splitInstructionQuery(c.Query("user_notifications")),
		OpsNotifications:   splitInstructionQuery(c.Query("ops_notifications")),
		ClientTypes:        splitInstructionQuery(c.Query("client_types")),
	}
	var err error
	if raw := strings.TrimSpace(c.Query("event_id")); raw != "" {
		filter.EventID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || filter.EventID <= 0 {
			return InstructionEventFilter{}, infraerrors.BadRequest("instruction_audit_invalid_event_id", "审核事件 ID 无效")
		}
	}
	if filter.UserID, err = optionalInstructionUserID(c.Query("user_id")); err != nil {
		return InstructionEventFilter{}, err
	}
	for _, raw := range splitInstructionQuery(c.Query("group_ids")) {
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || id <= 0 {
			return InstructionEventFilter{}, infraerrors.BadRequest("instruction_audit_invalid_group_filter", "分组筛选无效")
		}
		filter.GroupIDs = append(filter.GroupIDs, id)
	}
	if filter.From, err = optionalInstructionTime(c.Query("from")); err != nil {
		return InstructionEventFilter{}, err
	}
	if filter.To, err = optionalInstructionTime(c.Query("to")); err != nil {
		return InstructionEventFilter{}, err
	}
	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return InstructionEventFilter{}, infraerrors.BadRequest("instruction_audit_invalid_time_range", "时间范围无效")
	}
	return filter, nil
}

func splitInstructionQuery(value string) []string {
	parts := strings.Split(strings.TrimSpace(value), ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func optionalInstructionTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_time_range", "时间范围无效")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func instructionEvidenceAccess(c *gin.Context, action, source string) InstructionEvidenceAccess {
	requestID := strings.TrimSpace(c.GetHeader("X-Request-Id"))
	if requestID == "" {
		requestID = strings.TrimSpace(c.Writer.Header().Get("X-Request-Id"))
	}
	return InstructionEvidenceAccess{
		ActorID: adminID(c), Action: action, Source: source, RequestID: requestID,
		ClientIP: c.ClientIP(), UserAgent: c.GetHeader("User-Agent"),
	}
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
