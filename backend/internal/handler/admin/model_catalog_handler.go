package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelCatalogHandler struct {
	service *service.ModelCatalogService
}

func NewModelCatalogHandler(service *service.ModelCatalogService) *ModelCatalogHandler {
	return &ModelCatalogHandler{service: service}
}

func (h *ModelCatalogHandler) List(c *gin.Context) {
	items, err := h.service.ListForAdmin(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *ModelCatalogHandler) Upsert(c *gin.Context) {
	var metadata service.ModelCatalogMetadata
	if err := c.ShouldBindJSON(&metadata); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.service.UpsertMetadata(c.Request.Context(), &metadata); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, metadata)
}

func (h *ModelCatalogHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid metadata ID")
		return
	}
	if err := h.service.DeleteMetadata(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
