package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func rejectUnpricedGrokGatewayCapability(c *gin.Context, capability string) {
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
		"error": gin.H{
			"type":    "not_found_error",
			"message": capability + " API is not available for this group",
		},
	})
}

func rejectUnpricedGrokGatewayCapabilityAnthropic(c *gin.Context, capability string) {
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "not_found_error",
			"message": capability + " API is not available for this group",
		},
	})
}
