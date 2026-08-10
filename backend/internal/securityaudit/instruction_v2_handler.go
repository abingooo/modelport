package securityaudit

import (
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type InstructionV2AdminHandler struct {
	service *InstructionV2Service
}

func NewInstructionV2AdminHandler(service *InstructionV2Service) *InstructionV2AdminHandler {
	return &InstructionV2AdminHandler{service: service}
}

func (h *InstructionV2AdminHandler) GetOverview(c *gin.Context) {
	item, err := h.service.AdminConfig(c.Request.Context())
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) GetConfig(c *gin.Context) {
	item, err := h.service.AdminConfig(c.Request.Context())
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) UpdateConfig(c *gin.Context) {
	var request UpdateInstructionV2ConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.audit(c, "failed", "instruction_audit_v2_invalid_config", nil)
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_config", "指令审核配置请求无效"))
		return
	}
	item, err := h.service.UpdateAdminConfig(c.Request.Context(), request, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"mode": request.Mode})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"mode": item.Mode, "config_version": item.ConfigVersion})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) ListEvents(c *gin.Context) {
	page, pageSize, err := instructionV2Pagination(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	filter, err := instructionV2EventFilterFromQuery(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items, err := h.service.ListAdminEvents(c.Request.Context(), page, pageSize, filter)
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) GetStatistics(c *gin.Context) {
	filter, err := instructionV2EventFilterFromQuery(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	item, err := h.service.AdminStatistics(c.Request.Context(), filter)
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) GetEvent(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	item, err := h.service.GetAdminEvent(c.Request.Context(), id)
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) DeleteEvent(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	deleted, err := h.service.DeleteAdminEvents(c.Request.Context(), []int64{id})
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"event_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"event_id": id, "deleted": deleted})
	response.Success(c, gin.H{"deleted": deleted})
}

func (h *InstructionV2AdminHandler) BatchDeleteEvents(c *gin.Context) {
	var request InstructionV2DeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_event_ids", "审核事件删除请求无效"))
		return
	}
	deleted, err := h.service.DeleteAdminEvents(c.Request.Context(), request.IDs)
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"event_ids": request.IDs})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"event_ids": request.IDs, "deleted": deleted})
	response.Success(c, gin.H{"deleted": deleted})
}

func (h *InstructionV2AdminHandler) RevealEventEvidence(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	item, err := h.service.RevealAdminEventEvidence(c.Request.Context(), id, instructionV2RawAccess(c, "reveal", ""))
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) RecordEventEvidenceCopy(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	var request struct {
		FieldName string `json:"field_name"`
	}
	if c.Request.ContentLength != 0 {
		_ = c.ShouldBindJSON(&request)
	}
	err := h.service.RecordAdminEventEvidenceCopy(
		c.Request.Context(), id, instructionV2RawAccess(c, "copy", request.FieldName),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"recorded": true})
}

func (h *InstructionV2AdminHandler) TrustEvent(c *gin.Context) {
	id, ok := instructionIDParam(c, "event")
	if !ok {
		return
	}
	var request InstructionV2TrustEventRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_trust_request", "一键加入可信指令的请求无效"))
		return
	}
	item, err := h.service.TrustAdminEvent(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"event_id": id, "fields": request.Fields})
		response.ErrorFrom(c, err)
		return
	}
	hashIDs := make([]int64, 0, len(item.Hashes))
	for _, hash := range item.Hashes {
		hashIDs = append(hashIDs, hash.ID)
	}
	h.audit(c, "success", "", map[string]any{"event_id": id, "fields": request.Fields, "hash_ids": hashIDs})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) ListHashes(c *gin.Context) {
	page, pageSize, err := instructionV2Pagination(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items, err := h.service.ListAdminHashes(c.Request.Context(), page, pageSize, c.Query("status"), c.Query("q"))
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) GetHash(c *gin.Context) {
	id, ok := instructionIDParam(c, "hash")
	if !ok {
		return
	}
	item, err := h.service.GetAdminHash(c.Request.Context(), id)
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) CreateHash(c *gin.Context) {
	var request SaveInstructionV2HashRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_hash", "可信指令请求无效"))
		return
	}
	item, err := h.service.CreateAdminHash(c.Request.Context(), request, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"sha256": request.SHA256, "scope_ids": request.ScopeIDs})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"hash_id": item.ID, "sha256": item.SHA256, "scope_ids": item.ScopeIDs})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) UpdateHash(c *gin.Context) {
	id, ok := instructionIDParam(c, "hash")
	if !ok {
		return
	}
	var request UpdateInstructionV2HashRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_hash", "可信指令更新请求无效"))
		return
	}
	item, err := h.service.UpdateAdminHash(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"hash_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"hash_id": id, "status": item.Status, "scope_ids": item.ScopeIDs})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) DeleteHash(c *gin.Context) {
	id, ok := instructionIDParam(c, "hash")
	if !ok {
		return
	}
	err := h.service.DeleteAdminHash(c.Request.Context(), id, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"hash_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"hash_id": id})
	response.Success(c, gin.H{"deleted": true})
}

func (h *InstructionV2AdminHandler) RevealHashRaw(c *gin.Context) {
	id, ok := instructionIDParam(c, "hash")
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	item, err := h.service.RevealAdminHashRaw(c.Request.Context(), id, instructionV2RawAccess(c, "reveal", ""))
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) RecordHashRawCopy(c *gin.Context) {
	id, ok := instructionIDParam(c, "hash")
	if !ok {
		return
	}
	err := h.service.RecordAdminHashRawCopy(c.Request.Context(), id, instructionV2RawAccess(c, "copy", ""))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"recorded": true})
}

func (h *InstructionV2AdminHandler) ListRiskHashes(c *gin.Context) {
	page, pageSize, err := instructionV2Pagination(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items, err := h.service.ListAdminRiskHashes(c.Request.Context(), page, pageSize, c.Query("status"), c.Query("q"))
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) GetRiskHash(c *gin.Context) {
	id, ok := instructionIDParam(c, "risk_hash")
	if !ok {
		return
	}
	item, err := h.service.GetAdminRiskHash(c.Request.Context(), id)
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) CreateRiskHash(c *gin.Context) {
	var request SaveInstructionV2RiskHashRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_risk_hash", "风险哈希请求无效"))
		return
	}
	item, err := h.service.CreateAdminRiskHash(c.Request.Context(), request, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"sha256": request.SHA256})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"risk_hash_id": item.ID, "sha256": item.SHA256})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) UpdateRiskHash(c *gin.Context) {
	id, ok := instructionIDParam(c, "risk_hash")
	if !ok {
		return
	}
	var request UpdateInstructionV2RiskHashRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_risk_action", "风险哈希操作无效"))
		return
	}
	item, err := h.service.UpdateAdminRiskHash(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"risk_hash_id": id, "action": request.Action})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"risk_hash_id": id, "action": request.Action})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) DeleteRiskHash(c *gin.Context) {
	id, ok := instructionIDParam(c, "risk_hash")
	if !ok {
		return
	}
	if err := h.service.DeleteAdminRiskHash(c.Request.Context(), id, adminID(c)); err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"risk_hash_id": id})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"risk_hash_id": id})
	response.Success(c, gin.H{"deleted": true})
}

func (h *InstructionV2AdminHandler) RevealRiskHashRaw(c *gin.Context) {
	id, ok := instructionIDParam(c, "risk_hash")
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	item, err := h.service.RevealAdminRiskHashRaw(c.Request.Context(), id, instructionV2RawAccess(c, "reveal", ""))
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) RecordRiskHashRawCopy(c *gin.Context) {
	id, ok := instructionIDParam(c, "risk_hash")
	if !ok {
		return
	}
	err := h.service.RecordAdminRiskHashRawCopy(c.Request.Context(), id, instructionV2RawAccess(c, "copy", ""))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"recorded": true})
}

func (h *InstructionV2AdminHandler) ListReviewJobs(c *gin.Context) {
	page, pageSize, err := instructionV2Pagination(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items, err := h.service.ListAdminReviewJobs(c.Request.Context(), page, pageSize, c.Query("status"), c.Query("q"))
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) GetReviewJob(c *gin.Context) {
	id, ok := instructionIDParam(c, "review_job")
	if !ok {
		return
	}
	item, err := h.service.GetAdminReviewJob(c.Request.Context(), id)
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) RetryReviewJob(c *gin.Context) {
	id, ok := instructionIDParam(c, "review_job")
	if !ok {
		return
	}
	if err := h.service.RetryAdminReviewJob(c.Request.Context(), id); err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"review_job_id": id, "action": "retry"})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"review_job_id": id, "action": "retry"})
	response.Success(c, gin.H{"queued": true})
}

func (h *InstructionV2AdminHandler) RevealReviewJobRaw(c *gin.Context) {
	id, ok := instructionIDParam(c, "review_job")
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	item, err := h.service.RevealAdminReviewJobRaw(c.Request.Context(), id, instructionV2RawAccess(c, "reveal", ""))
	respondInstructionV2(c, item, err)
}

func (h *InstructionV2AdminHandler) RecordReviewJobRawCopy(c *gin.Context) {
	id, ok := instructionIDParam(c, "review_job")
	if !ok {
		return
	}
	err := h.service.RecordAdminReviewJobRawCopy(c.Request.Context(), id, instructionV2RawAccess(c, "copy", ""))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"recorded": true})
}

func (h *InstructionV2AdminHandler) ListScopes(c *gin.Context) {
	items, err := h.service.ListAdminScopes(c.Request.Context())
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) SaveScope(c *gin.Context) {
	id, err := optionalInstructionV2PathID(c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var request SaveInstructionV2ScopeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_scope", "审核范围请求无效"))
		return
	}
	item, err := h.service.SaveAdminScope(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"scope_id": id, "group_id": request.GroupID})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"scope_id": item.ID, "group_id": item.GroupID, "client_profile_id": item.ClientProfileID})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) DeleteScope(c *gin.Context) {
	id, ok := instructionIDParam(c, "scope")
	if !ok {
		return
	}
	err := h.service.DeleteAdminScope(c.Request.Context(), id, adminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"scope_id": id})
	response.Success(c, gin.H{"deleted": true})
}

func (h *InstructionV2AdminHandler) ListGroups(c *gin.Context) {
	items, err := h.service.ListAdminGroups(c.Request.Context())
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) ListClientProfiles(c *gin.Context) {
	items, err := h.service.ListAdminClientProfiles(c.Request.Context())
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) SaveClientProfile(c *gin.Context) {
	id, err := optionalInstructionV2PathID(c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var request SaveInstructionV2ClientProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_client", "客户端识别规则请求无效"))
		return
	}
	item, err := h.service.SaveAdminClientProfile(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"client_profile_id": item.ID, "profile_key": item.ProfileKey})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) DeleteClientProfile(c *gin.Context) {
	id, ok := instructionIDParam(c, "client_profile")
	if !ok {
		return
	}
	err := h.service.DeleteAdminClientProfile(c.Request.Context(), id, adminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"client_profile_id": id})
	response.Success(c, gin.H{"deleted": true})
}

func (h *InstructionV2AdminHandler) ListUserAllowlist(c *gin.Context) {
	items, err := h.service.ListAdminUserAllowlist(c.Request.Context())
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) ListUsers(c *gin.Context) {
	items, err := h.service.ListAdminUserOptions(c.Request.Context(), c.Query("q"))
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) SaveUserAllowlist(c *gin.Context) {
	var request SaveInstructionV2UserAllowlistRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_user_allowlist", "用户白名单请求无效"))
		return
	}
	item, err := h.service.SaveAdminUserAllowlist(c.Request.Context(), request, adminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"allowlist_id": item.ID, "user_id": item.UserID})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) DeleteUserAllowlist(c *gin.Context) {
	id, ok := instructionIDParam(c, "user_allowlist")
	if !ok {
		return
	}
	err := h.service.DeleteAdminUserAllowlist(c.Request.Context(), id, adminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"allowlist_id": id})
	response.Success(c, gin.H{"deleted": true})
}

func (h *InstructionV2AdminHandler) ListAINodes(c *gin.Context) {
	items, err := h.service.ListAdminAINodes(c.Request.Context())
	respondInstructionV2(c, items, err)
}

func (h *InstructionV2AdminHandler) SaveAINode(c *gin.Context) {
	id, err := optionalInstructionV2PathID(c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var request SaveInstructionV2AINodeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("instruction_audit_v2_invalid_ai_node", "AI 审核节点请求无效"))
		return
	}
	item, err := h.service.SaveAdminAINode(c.Request.Context(), id, request, adminID(c))
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"ai_node_id": id, "name": request.Name})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"ai_node_id": item.ID, "name": item.Name, "enabled": item.Enabled})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) DeleteAINode(c *gin.Context) {
	id, ok := instructionIDParam(c, "ai_node")
	if !ok {
		return
	}
	err := h.service.DeleteAdminAINode(c.Request.Context(), id, adminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"ai_node_id": id})
	response.Success(c, gin.H{"deleted": true})
}

func (h *InstructionV2AdminHandler) TestAINode(c *gin.Context) {
	id, ok := instructionIDParam(c, "ai_node")
	if !ok {
		return
	}
	item, err := h.service.TestAdminAINode(c.Request.Context(), id)
	if err != nil {
		h.audit(c, "failed", infraerrors.Reason(err), map[string]any{"ai_node_id": id, "action": "test"})
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "success", "", map[string]any{"ai_node_id": id, "action": "test", "result": item.Result})
	response.Success(c, item)
}

func (h *InstructionV2AdminHandler) audit(c *gin.Context, result, errorCode string, fields map[string]any) {
	if fields == nil {
		fields = make(map[string]any)
	}
	if h != nil && h.service != nil {
		fields["config_version"] = h.service.ConfigVersion()
	}
	setInstructionAdminAudit(c, result, errorCode, fields)
}

func respondInstructionV2(c *gin.Context, value any, err error) {
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, value)
}

func instructionV2Pagination(c *gin.Context) (int, int, error) {
	page, pageSize := 1, 20
	var err error
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			return 0, 0, infraerrors.BadRequest("instruction_audit_v2_invalid_page", "页码无效")
		}
	}
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil || pageSize < 1 || pageSize > 100 {
			return 0, 0, infraerrors.BadRequest("instruction_audit_v2_invalid_page_size", "每页数量无效")
		}
	}
	return page, pageSize, nil
}

func optionalInstructionV2PathID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, infraerrors.BadRequest("instruction_audit_v2_invalid_id", "ID 无效")
	}
	return id, nil
}

func instructionV2EventFilterFromQuery(c *gin.Context) (InstructionV2EventFilter, error) {
	filter := InstructionV2EventFilter{
		Query: c.Query("q"), Model: strings.TrimSpace(c.Query("model")),
		ClientKeys: splitInstructionQuery(c.Query("client_keys")),
		Outcomes:   splitInstructionQuery(c.Query("outcomes")),
		Reasons:    splitInstructionQuery(c.Query("reasons")),
		AIResults:  splitInstructionQuery(c.Query("ai_results")),
	}
	var err error
	if raw := strings.TrimSpace(c.Query("event_id")); raw != "" {
		filter.EventID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || filter.EventID <= 0 {
			return InstructionV2EventFilter{}, infraerrors.BadRequest("instruction_audit_v2_invalid_event_id", "事件编号无效")
		}
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		filter.UserID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || filter.UserID <= 0 {
			return InstructionV2EventFilter{}, infraerrors.BadRequest("instruction_audit_v2_invalid_user_id", "用户筛选无效")
		}
	}
	for _, raw := range splitInstructionQuery(c.Query("group_ids")) {
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || id <= 0 {
			return InstructionV2EventFilter{}, infraerrors.BadRequest("instruction_audit_v2_invalid_group_id", "分组筛选无效")
		}
		filter.GroupIDs = append(filter.GroupIDs, id)
	}
	if len(filter.GroupIDs) > 50 || len(filter.ClientKeys) > 50 || len(filter.Outcomes) > 50 || len(filter.Reasons) > 50 || len(filter.AIResults) > 50 {
		return InstructionV2EventFilter{}, infraerrors.BadRequest("instruction_audit_v2_filter_too_large", "筛选条件过多")
	}
	if filter.From, err = optionalInstructionTime(c.Query("from")); err != nil {
		return InstructionV2EventFilter{}, err
	}
	if filter.To, err = optionalInstructionTime(c.Query("to")); err != nil {
		return InstructionV2EventFilter{}, err
	}
	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return InstructionV2EventFilter{}, infraerrors.BadRequest("instruction_audit_v2_invalid_time_range", "时间范围无效")
	}
	return filter, nil
}

func instructionV2RawAccess(c *gin.Context, action, fieldName string) InstructionV2RawAccess {
	requestID := strings.TrimSpace(c.GetHeader("X-Request-Id"))
	if requestID == "" {
		requestID = strings.TrimSpace(c.Writer.Header().Get("X-Request-Id"))
	}
	return InstructionV2RawAccess{
		ActorID: adminID(c), Action: action, FieldName: strings.TrimSpace(fieldName),
		RequestID: requestID, ClientIP: c.ClientIP(), UserAgent: c.GetHeader("User-Agent"),
	}
}
