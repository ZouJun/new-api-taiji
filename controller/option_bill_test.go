package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOptionsHidesBillAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{
		model.BillAccessTokenOptionKey:  "billing-secret",
		model.BillSupplierNameOptionKey: "Example Supplier",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/option/", nil)

	GetOptions(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "billing-secret")

	var response struct {
		Data []model.Option `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))

	options := make(map[string]string, len(response.Data))
	for _, option := range response.Data {
		options[option.Key] = option.Value
	}
	assert.NotContains(t, options, model.BillAccessTokenOptionKey)
	assert.Equal(t, "true", options["BillAccessTokenConfigured"])
	assert.Equal(t, "Example Supplier", options[model.BillSupplierNameOptionKey])
}
