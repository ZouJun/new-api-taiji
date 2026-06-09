package middleware

import (
	"context"
	"errors"
	"net/http"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const traceIDHeader = "Trace-Id"

func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID, newAPIError := resolveTraceID(c.Request.Header.Values(traceIDHeader))
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
			c.Abort()
			return
		}
		c.Set(common.TraceIDKey, traceID)
		ctx := context.WithValue(c.Request.Context(), common.TraceIDKey, traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func resolveTraceID(values []string) (string, *types.NewAPIError) {
	switch len(values) {
	case 0:
		return "", nil
	case 1:
		return validateTraceID(values[0])
	default:
		return "", invalidTraceIDError("invalid Trace-Id: multiple values are not allowed")
	}
}

func validateTraceID(value string) (string, *types.NewAPIError) {
	if value == "" {
		return "", invalidTraceIDError("invalid Trace-Id: empty value")
	}
	if len(value) > 64 {
		return "", invalidTraceIDError("invalid Trace-Id: length exceeds 64")
	}
	for _, r := range value {
		if unicode.IsSpace(r) {
			return "", invalidTraceIDError("invalid Trace-Id: spaces are not allowed")
		}
		if r > unicode.MaxASCII || !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return "", invalidTraceIDError("invalid Trace-Id: only alphanumeric characters are allowed")
		}
	}
	return value, nil
}

func invalidTraceIDError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(message),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}
