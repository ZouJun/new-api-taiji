package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSanitizeChannelForLimitedAdmin(t *testing.T) {
	modelMapping := `{"gpt-4":"azure-gpt-4"}`
	channel := &model.Channel{
		ModelMapping: &modelMapping,
	}

	sanitizeChannelForLimitedAdmin(channel)

	require.Nil(t, channel.ModelMapping)
}

func TestRejectLimitedAdminChannelMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleAdminUser)

	rejected := rejectLimitedAdminChannelMutation(ctx)

	require.True(t, rejected)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "当前管理员无权修改渠道配置")
}
