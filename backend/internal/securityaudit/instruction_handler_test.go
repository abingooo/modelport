package securityaudit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInstructionAdminAuditAlwaysIncludesConfigVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	service := &InstructionService{}
	service.snapshot.Store(&instructionSnapshot{ConfigVersion: 23})
	handler := &InstructionAdminHandler{service: service}

	handler.setAdminAudit(context, "success", "", map[string]any{"hash_id": int64(7)})
	value, exists := context.Get("audit_extra")
	require.True(t, exists)
	details, ok := value.(map[string]any)
	require.True(t, ok)
	require.EqualValues(t, 23, details["config_version"])
	require.EqualValues(t, 7, details["hash_id"])
}

func TestInstructionEventFilterFromQueryKeepsLegacyUserAndModelFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodGet, "/events?user_id=42&model=gpt-5&group_ids=7,8", nil)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request

	filter, err := instructionEventFilterFromQuery(context)
	require.NoError(t, err)
	require.EqualValues(t, 42, filter.UserID)
	require.Equal(t, "gpt-5", filter.Model)
	require.Equal(t, []int64{7, 8}, filter.GroupIDs)
}

func TestInstructionEventFilterFromQueryRejectsInvalidLegacyUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodGet, "/events?user_id=not-a-number", nil)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request

	_, err := instructionEventFilterFromQuery(context)
	require.Error(t, err)
	require.Contains(t, err.Error(), "instruction_audit_invalid_user_id")
}
