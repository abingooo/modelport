//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func bindGroupPricingRequest(t *testing.T, method, body string, target any) error {
	t.Helper()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(method, "/admin/groups/1", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx.ShouldBindJSON(target)
}

func TestGroupRequestsValidateNestedModelPricing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, mode := range []string{"token", "per_request", "image", "video"} {
		t.Run("create accepts "+mode, func(t *testing.T) {
			body := `{"name":"priced","platform":"openai","model_pricing":[{"platform":"openai","models":["model-1"],"billing_mode":"` + mode + `","per_request_price":0}]}`
			var req CreateGroupRequest
			require.NoError(t, bindGroupPricingRequest(t, http.MethodPost, body, &req))
			require.Len(t, req.ModelPricing, 1)
			require.Equal(t, mode, req.ModelPricing[0].BillingMode)
		})

		t.Run("update accepts "+mode, func(t *testing.T) {
			body := `{"model_pricing":[{"platform":"openai","models":["model-1"],"billing_mode":"` + mode + `","per_request_price":0}]}`
			var req UpdateGroupRequest
			require.NoError(t, bindGroupPricingRequest(t, http.MethodPut, body, &req))
			require.NotNil(t, req.ModelPricing)
			require.Len(t, *req.ModelPricing, 1)
			require.Equal(t, mode, (*req.ModelPricing)[0].BillingMode)
		})
	}

	t.Run("create rejects unknown mode", func(t *testing.T) {
		var req CreateGroupRequest
		err := bindGroupPricingRequest(t, http.MethodPost, `{"name":"priced","platform":"openai","model_pricing":[{"platform":"openai","models":["model-1"],"billing_mode":"hourly"}]}`, &req)
		require.Error(t, err)
	})

	t.Run("update rejects unknown mode", func(t *testing.T) {
		var req UpdateGroupRequest
		err := bindGroupPricingRequest(t, http.MethodPut, `{"model_pricing":[{"platform":"openai","models":["model-1"],"billing_mode":"hourly"}]}`, &req)
		require.Error(t, err)
	})

	t.Run("rejects missing nested models", func(t *testing.T) {
		var req CreateGroupRequest
		err := bindGroupPricingRequest(t, http.MethodPost, `{"name":"priced","platform":"openai","model_pricing":[{"platform":"openai","billing_mode":"token"}]}`, &req)
		require.Error(t, err)
	})
}
