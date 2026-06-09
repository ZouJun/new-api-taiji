package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayTaggedRoutesRejectInvalidTraceIDBeforeHandler(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()

	modelsRouter := engine.Group("/v1/models")
	modelsRouter.Use(middleware.RouteTag("relay"))
	modelsRouter.Use(middleware.TraceID())
	modelsRouter.GET("", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Trace-Id", "bad-trace-id")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "only alphanumeric characters are allowed")
}

func TestRelayTaggedRoutesAllowMissingTraceID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()

	videoProxyRouter := engine.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(middleware.TraceID())
	videoProxyRouter.GET("/videos/:task_id/content", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/videos/task-123/content", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}
