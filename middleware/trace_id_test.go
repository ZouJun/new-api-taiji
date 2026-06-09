package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTraceIDMiddleware_AllowsMissingHeaderAndPropagatesEmptyValue(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceID())
	router.GET("/", func(c *gin.Context) {
		require.Equal(t, "", c.GetString(common.TraceIDKey))
		ctxValue := c.Request.Context().Value(common.TraceIDKey)
		require.Equal(t, "", ctxValue)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestTraceIDMiddleware_RejectsEmptyValue(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceID())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header["Trace-Id"] = []string{""}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid Trace-Id: empty value")
}

func TestTraceIDMiddleware_RejectsMultipleValues(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceID())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header["Trace-Id"] = []string{"abc123", "xyz789"}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "multiple values are not allowed")
}

func TestTraceIDMiddleware_RejectsWhitespaceAndInvalidCharactersAndLength(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		value   string
		message string
	}{
		{name: "leading space", value: " abc123", message: "spaces are not allowed"},
		{name: "middle space", value: "abc 123", message: "spaces are not allowed"},
		{name: "invalid char", value: "abc-123", message: "only alphanumeric characters are allowed"},
		{name: "too long", value: strings.Repeat("A", 65), message: "length exceeds 64"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(TraceID())
			router.GET("/", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Trace-Id", tc.value)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), tc.message)
		})
	}
}

func TestTraceIDMiddleware_PreservesCaseForValidValue(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceID())
	router.GET("/", func(c *gin.Context) {
		require.Equal(t, "AbC123XYZ", c.GetString(common.TraceIDKey))
		require.Equal(t, "AbC123XYZ", c.Request.Context().Value(common.TraceIDKey))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Trace-Id", "AbC123XYZ")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestSetUpLogger_IncludesTraceID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	oldWriter := gin.DefaultWriter
	defer func() { gin.DefaultWriter = oldWriter }()

	recorder := httptest.NewRecorder()
	gin.DefaultWriter = recorder

	router := gin.New()
	SetUpLogger(router)
	router.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "req123")
		c.Set(common.TraceIDKey, "TraceABC")
		c.Next()
	})
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(recorder, req)

	logLine := recorder.Body.String()
	require.Contains(t, logLine, "request_id=req123")
	require.Contains(t, logLine, "trace_id=TraceABC")
}
