package handler

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

func lotteryListParams(c *gin.Context) service.LotteryListParams {
	page, pageSize := response.ParsePagination(c)
	return service.LotteryListParams{
		Page: page, PageSize: pageSize, Status: c.Query("status"), Mode: c.Query("mode"), Search: c.Query("search"),
	}
}

func parseLotteryID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid lottery campaign ID")
		return 0, false
	}
	return id, true
}

func (h *LotteryHandler) List(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	params := lotteryListParams(c).Normalized()
	items, total, err := h.service.ListForUser(c.Request.Context(), subject.UserID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, params.Page, params.PageSize)
}

func (h *LotteryHandler) Get(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, ok := parseLotteryID(c)
	if !ok {
		return
	}
	item, err := h.service.GetForUser(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *LotteryHandler) Participate(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, ok := parseLotteryID(c)
	if !ok {
		return
	}
	var request struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			response.BadRequest(c, "Invalid request: "+err.Error())
			return
		}
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(request.IdempotencyKey)
	}
	entry, err := h.service.Participate(c.Request.Context(), subject.UserID, id, key)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, entry)
}

func (h *LotteryHandler) History(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	params := lotteryListParams(c).Normalized()
	items, total, err := h.service.ListUserEntries(c.Request.Context(), subject.UserID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, params.Page, params.PageSize)
}
