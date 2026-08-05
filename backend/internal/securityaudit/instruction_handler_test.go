package securityaudit

import (
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
