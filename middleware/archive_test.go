package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/archive"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestArchiveMarksGetWithoutRequestBodyAsSkipped(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/archive-test", func(c *gin.Context) {
		requestInfo := archive.ObjectInfo{Status: archive.StatusPending, Stage: "downstream_raw"}
		_, transformedBody := c.Get(common.KeyRequestBody)
		_, hasRawBodyStorage := c.Get(common.KeyRawRequestBodyStorage)
		hasRequestBody := transformedBody || hasRawBodyStorage || c.Request.ContentLength > 0 || len(c.Request.TransferEncoding) > 0
		if !hasRequestBody && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) {
			requestInfo = archive.ObjectInfo{Status: archive.StatusSkipped, Reason: archive.ReasonNoRequestBody, Stage: "downstream_absent"}
		}

		require.Equal(t, archive.StatusSkipped, requestInfo.Status)
		require.Equal(t, archive.ReasonNoRequestBody, requestInfo.Reason)
		require.Equal(t, "downstream_absent", requestInfo.Stage)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/archive-test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}
