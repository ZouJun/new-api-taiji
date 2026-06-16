package channel

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func startDisconnectAwareHTTPServer(t *testing.T) (string, <-chan error, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	disconnected := make(chan error, 1)
	serverDone := make(chan struct{})

	go func() {
		defer close(serverDone)

		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			disconnected <- acceptErr
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		req, readErr := http.ReadRequest(reader)
		if readErr != nil {
			disconnected <- readErr
			return
		}
		if req.Body != nil {
			_, _ = io.ReadAll(req.Body)
			_ = req.Body.Close()
		}

		deadlineAt := time.Now().Add(5 * time.Second)
		buf := make([]byte, 1)
		for time.Now().Before(deadlineAt) {
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			_, readErr = conn.Read(buf)
			if readErr == nil {
				continue
			}
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				continue
			}
			disconnected <- readErr
			return
		}

		disconnected <- fmt.Errorf("client connection was not closed before timeout")
	}()

	cleanup := func() {
		_ = listener.Close()
		<-serverDone
	}

	return "http://" + listener.Addr().String(), disconnected, cleanup
}

func TestDoRequest_NonStreamTimeoutCancelsUpstreamConnection(t *testing.T) {
	serverURL, disconnected, cleanup := startDisconnectAwareHTTPServer(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mock-gpt"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	timeoutSeconds := 1
	info := &relaycommon.RelayInfo{
		IsStream:                false,
		RelayFormat:             "openai",
		FinalRequestRelayFormat: "openai",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				NonStreamTimeoutSeconds: &timeoutSeconds,
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx.Request.Context(), http.MethodPost, serverURL, strings.NewReader(`{"model":"mock-gpt"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	startedAt := time.Now()
	resp, err := DoRequest(ctx, req, info)
	elapsed := time.Since(startedAt)
	require.Nil(t, resp)
	require.Error(t, err)

	var apiErr *types.NewAPIError
	require.True(t, errors.As(err, &apiErr), "expected NewAPIError, got %T", err)
	require.Equal(t, http.StatusGatewayTimeout, apiErr.StatusCode)
	require.Equal(t, types.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
	require.Less(t, elapsed, 2500*time.Millisecond)

	select {
	case disconnectErr := <-disconnected:
		require.ErrorIs(t, disconnectErr, io.EOF)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("expected upstream request context to be canceled by non-stream timeout")
	}

	timeoutMeta := relaycommon.AppendTimeoutMeta(map[string]interface{}{}, ctx)
	require.Equal(t, relaycommon.TimeoutTypeNonStreamTotal, timeoutMeta["timeout_type"])
	require.Equal(t, relaycommon.TimeoutSourceChannelSetting, timeoutMeta["timeout_source"])
	require.Equal(t, timeoutSeconds, timeoutMeta["timeout_seconds"])
	require.Equal(t, "http_request", timeoutMeta["timeout_stage"])
}

func TestDoRequest_StreamFirstByteTimeoutCancelsUpstreamConnection(t *testing.T) {
	serverURL, disconnected, cleanup := startDisconnectAwareHTTPServer(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mock-gpt","stream":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	timeoutSeconds := 1
	info := &relaycommon.RelayInfo{
		IsStream:                true,
		RelayFormat:             "openai",
		FinalRequestRelayFormat: "openai",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				StreamFirstByteTimeoutSeconds: &timeoutSeconds,
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx.Request.Context(), http.MethodPost, serverURL, strings.NewReader(`{"model":"mock-gpt","stream":true}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	startedAt := time.Now()
	resp, err := DoRequest(ctx, req, info)
	elapsed := time.Since(startedAt)
	require.Nil(t, resp)
	require.Error(t, err)

	var apiErr *types.NewAPIError
	require.True(t, errors.As(err, &apiErr), "expected NewAPIError, got %T", err)
	require.Equal(t, http.StatusGatewayTimeout, apiErr.StatusCode)
	require.Equal(t, types.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
	require.Less(t, elapsed, 2500*time.Millisecond)

	select {
	case disconnectErr := <-disconnected:
		require.ErrorIs(t, disconnectErr, io.EOF)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("expected upstream request context to be canceled by stream first-byte timeout")
	}

	timeoutMeta := relaycommon.AppendTimeoutMeta(map[string]interface{}{}, ctx)
	require.Equal(t, relaycommon.TimeoutTypeStreamFirstByte, timeoutMeta["timeout_type"])
	require.Equal(t, relaycommon.TimeoutSourceChannelSetting, timeoutMeta["timeout_source"])
	require.Equal(t, timeoutSeconds, timeoutMeta["timeout_seconds"])
	require.Equal(t, "stream_first_byte_wait", timeoutMeta["timeout_stage"])
}

func TestDoRequest_ClientCanceledRequestStopsRetryClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	requestCtx, cancel := context.WithCancel(context.Background())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"mock-gpt","stream":true}`)).WithContext(requestCtx)
	ctx.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		IsStream:                true,
		RelayFormat:             "claude",
		FinalRequestRelayFormat: "claude",
		ChannelMeta:             &relaycommon.ChannelMeta{},
	}

	req, err := http.NewRequestWithContext(ctx.Request.Context(), http.MethodPost, "http://127.0.0.1:1/v1/messages", strings.NewReader(`{"model":"mock-gpt","stream":true}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	cancel()

	resp, err := DoRequest(ctx, req, info)
	require.Nil(t, resp)
	require.Error(t, err)

	var apiErr *types.NewAPIError
	require.True(t, errors.As(err, &apiErr), "expected NewAPIError, got %T", err)
	require.Equal(t, 499, apiErr.StatusCode)
	require.Equal(t, types.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
	require.True(t, types.IsSkipRetryError(apiErr))
	require.Equal(t, "client canceled request", apiErr.ClientMessage())
	require.ErrorIs(t, apiErr, context.Canceled)
}
