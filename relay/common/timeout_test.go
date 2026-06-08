package common

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
)

func TestResolveNonStreamTimeoutSeconds(t *testing.T) {
	oldDefault := rootcommon.RelayDefaultNonStreamTimeout
	oldLegacy := rootcommon.RelayTimeout
	t.Cleanup(func() {
		rootcommon.RelayDefaultNonStreamTimeout = oldDefault
		rootcommon.RelayTimeout = oldLegacy
	})

	channelTimeout := 18
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				NonStreamTimeoutSeconds: &channelTimeout,
			},
		},
	}
	rootcommon.RelayDefaultNonStreamTimeout = 25
	rootcommon.RelayTimeout = 40

	timeout, source := ResolveNonStreamTimeoutSeconds(info)
	if timeout != 18 || source != TimeoutSourceChannelSetting {
		t.Fatalf("expected channel timeout to win, got timeout=%d source=%s", timeout, source)
	}

	info.ChannelSetting.NonStreamTimeoutSeconds = nil
	timeout, source = ResolveNonStreamTimeoutSeconds(info)
	if timeout != 25 || source != TimeoutSourceProviderGlobal {
		t.Fatalf("expected provider default to win, got timeout=%d source=%s", timeout, source)
	}

	rootcommon.RelayDefaultNonStreamTimeout = 0
	timeout, source = ResolveNonStreamTimeoutSeconds(info)
	if timeout != 40 || source != TimeoutSourceLegacyGlobal {
		t.Fatalf("expected legacy timeout to win, got timeout=%d source=%s", timeout, source)
	}
}

func TestResolveAWSHTTPClientTimeouts(t *testing.T) {
	oldNonStream := rootcommon.AWSHTTPClientNonStreamTimeoutSeconds
	oldStream := rootcommon.AWSHTTPClientStreamFirstByteTimeoutSeconds
	oldDefaultNonStream := rootcommon.RelayDefaultNonStreamTimeout
	oldDefaultStream := rootcommon.RelayDefaultStreamFirstByteTimeout
	t.Cleanup(func() {
		rootcommon.AWSHTTPClientNonStreamTimeoutSeconds = oldNonStream
		rootcommon.AWSHTTPClientStreamFirstByteTimeoutSeconds = oldStream
		rootcommon.RelayDefaultNonStreamTimeout = oldDefaultNonStream
		rootcommon.RelayDefaultStreamFirstByteTimeout = oldDefaultStream
	})

	channelNonStream := 19
	channelStream := 9
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				AwsHTTPClientNonStreamTimeoutSeconds:       &channelNonStream,
				AwsHTTPClientStreamFirstByteTimeoutSeconds: &channelStream,
			},
		},
	}
	rootcommon.AWSHTTPClientNonStreamTimeoutSeconds = 33
	rootcommon.AWSHTTPClientStreamFirstByteTimeoutSeconds = 17

	timeout, source := ResolveAWSHTTPClientNonStreamTimeoutSeconds(info)
	if timeout != 19 || source != TimeoutSourceChannelSetting {
		t.Fatalf("expected channel aws non-stream timeout to win, got timeout=%d source=%s", timeout, source)
	}

	timeout, source = ResolveAWSHTTPClientStreamFirstByteTimeoutSeconds(info)
	if timeout != 9 || source != TimeoutSourceChannelSetting {
		t.Fatalf("expected channel aws stream timeout to win, got timeout=%d source=%s", timeout, source)
	}

	info.ChannelSetting.AwsHTTPClientNonStreamTimeoutSeconds = nil
	info.ChannelSetting.AwsHTTPClientStreamFirstByteTimeoutSeconds = nil

	timeout, source = ResolveAWSHTTPClientNonStreamTimeoutSeconds(info)
	if timeout != 33 || source != TimeoutSourceProviderGlobal {
		t.Fatalf("expected global aws non-stream timeout to win, got timeout=%d source=%s", timeout, source)
	}

	timeout, source = ResolveAWSHTTPClientStreamFirstByteTimeoutSeconds(info)
	if timeout != 17 || source != TimeoutSourceProviderGlobal {
		t.Fatalf("expected global aws stream timeout to win, got timeout=%d source=%s", timeout, source)
	}
}

func TestResolveAWSInvokeTimeoutSeconds(t *testing.T) {
	oldInvoke := rootcommon.AWSInvokeTimeoutSeconds
	oldDefaultNonStream := rootcommon.RelayDefaultNonStreamTimeout
	oldLegacy := rootcommon.RelayTimeout
	t.Cleanup(func() {
		rootcommon.AWSInvokeTimeoutSeconds = oldInvoke
		rootcommon.RelayDefaultNonStreamTimeout = oldDefaultNonStream
		rootcommon.RelayTimeout = oldLegacy
	})

	channelInvoke := 21
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				AwsInvokeTimeoutSeconds: &channelInvoke,
			},
		},
	}
	rootcommon.AWSInvokeTimeoutSeconds = 33
	rootcommon.RelayDefaultNonStreamTimeout = 44
	rootcommon.RelayTimeout = 55

	timeout, source := ResolveAWSInvokeTimeoutSeconds(info)
	if timeout != 21 || source != TimeoutSourceChannelSetting {
		t.Fatalf("expected channel aws invoke timeout to win, got timeout=%d source=%s", timeout, source)
	}

	info.ChannelSetting.AwsInvokeTimeoutSeconds = nil
	timeout, source = ResolveAWSInvokeTimeoutSeconds(info)
	if timeout != 33 || source != TimeoutSourceProviderGlobal {
		t.Fatalf("expected global aws invoke timeout to win, got timeout=%d source=%s", timeout, source)
	}

	rootcommon.AWSInvokeTimeoutSeconds = 0
	timeout, source = ResolveAWSInvokeTimeoutSeconds(info)
	if timeout != 44 || source != TimeoutSourceProviderGlobal {
		t.Fatalf("expected non-stream default to win, got timeout=%d source=%s", timeout, source)
	}

	rootcommon.RelayDefaultNonStreamTimeout = 0
	timeout, source = ResolveAWSInvokeTimeoutSeconds(info)
	if timeout != 55 || source != TimeoutSourceLegacyGlobal {
		t.Fatalf("expected legacy timeout to win, got timeout=%d source=%s", timeout, source)
	}
}

func TestFirstByteTimeoutController(t *testing.T) {
	ctx, controller := NewFirstByteTimeoutContext(t.Context(), 20*time.Millisecond)
	body := controller.WrapBody(io.NopCloser(strings.NewReader("")), 1)
	defer body.Close()

	<-ctx.Done()
	buf := make([]byte, 1)
	_, err := body.Read(buf)
	if !errors.Is(err, ErrStreamFirstByteTimeout) {
		t.Fatalf("expected first-byte timeout error, got %v", err)
	}
}

func TestAppendTimeoutMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("retry", 2)

	SetTimeoutMeta(c, TimeoutMeta{
		Type:     TimeoutTypeStreamFirstByte,
		Source:   TimeoutSourceChannelSetting,
		Seconds:  12,
		Provider: "aws",
		Stage:    "aws_stream_first_byte_wait",
		IsStream: true,
	})

	other := AppendTimeoutMeta(map[string]interface{}{}, c)
	if other["timeout_type"] != TimeoutTypeStreamFirstByte {
		t.Fatalf("expected timeout_type to be set, got %v", other["timeout_type"])
	}
	if other["retry_index"] != 2 {
		t.Fatalf("expected retry_index to be 2, got %v", other["retry_index"])
	}
}
