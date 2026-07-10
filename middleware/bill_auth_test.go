package middleware

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

func TestBillAccessTokenAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{
		model.BillAccessTokenOptionKey: "billing-secret",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	router := gin.New()
	router.GET("/bill", BillAccessTokenAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name           string
		token          string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "accepts configured token",
			token:          "billing-secret",
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "rejects missing token",
			expectedStatus: http.StatusOK,
			expectedBody:   `"code":401`,
		},
		{
			name:           "rejects invalid token",
			token:          "wrong-secret",
			expectedStatus: http.StatusOK,
			expectedBody:   `"code":401`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/bill", nil)
			if test.token != "" {
				request.Header.Set("X-Access-Token", test.token)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			assert.Equal(t, test.expectedStatus, recorder.Code)
			if test.expectedBody != "" {
				require.Contains(t, recorder.Body.String(), test.expectedBody)
				assert.Contains(t, recorder.Body.String(), `"message":"未授权访问"`)
			}
		})
	}
}
