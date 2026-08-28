package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type LotteryHandler struct {
	service *service.LotteryService
}

func NewLotteryHandler(lotteryService *service.LotteryService) *LotteryHandler {
	return &LotteryHandler{service: lotteryService}
}

func adminLotteryListParams(c *gin.Context) service.LotteryListParams {
	page, pageSize := response.ParsePagination(c)
	return service.LotteryListParams{
		Page: page, PageSize: pageSize, Status: c.Query("status"), Mode: c.Query("mode"), Search: c.Query("search"),
	}
}

func adminLotteryID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid lottery campaign ID")
		return 0, false
	}
	return id, true
}

func lotteryActorID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Admin not authenticated")
		return 0, false
	}
	return subject.UserID, true
}

func (h *LotteryHandler) List(c *gin.Context) {
	params := adminLotteryListParams(c).Normalized()
	items, total, err := h.service.ListForAdmin(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, params.Page, params.PageSize)
}

func (h *LotteryHandler) Get(c *gin.Context) {
	id, ok := adminLotteryID(c)
	if !ok {
		return
	}
	item, err := h.service.GetForAdmin(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *LotteryHandler) Create(c *gin.Context) {
	actorID, ok := lotteryActorID(c)
	if !ok {
		return
	}
	var input service.LotteryCampaignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.service.Create(c.Request.Context(), actorID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *LotteryHandler) Update(c *gin.Context) {
	id, ok := adminLotteryID(c)
	if !ok {
		return
	}
	actorID, ok := lotteryActorID(c)
	if !ok {
		return
	}
	var input service.LotteryCampaignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, actorID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *LotteryHandler) SetStatus(c *gin.Context) {
	id, ok := adminLotteryID(c)
	if !ok {
		return
	}
	actorID, ok := lotteryActorID(c)
	if !ok {
		return
	}
	var request struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.service.SetStatus(c.Request.Context(), id, actorID, strings.TrimSpace(request.Status))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *LotteryHandler) Delete(c *gin.Context) {
	id, ok := adminLotteryID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *LotteryHandler) Entries(c *gin.Context) {
	id, ok := adminLotteryID(c)
	if !ok {
		return
	}
	params := adminLotteryListParams(c).Normalized()
	items, total, err := h.service.ListAdminEntries(c.Request.Context(), id, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, params.Page, params.PageSize)
}

func (h *LotteryHandler) Draw(c *gin.Context) {
	id, ok := adminLotteryID(c)
	if !ok {
		return
	}
	actorID, ok := lotteryActorID(c)
	if !ok {
		return
	}
	result, err := h.service.DrawScheduled(c.Request.Context(), id, &actorID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
