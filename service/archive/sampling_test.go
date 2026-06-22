package archive

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestNormalizeSamplePercent(t *testing.T) {
	t.Parallel()

	if got := normalizeSamplePercent(-1); got != 0 {
		t.Fatalf("normalizeSamplePercent(-1) = %d, want 0", got)
	}
	if got := normalizeSamplePercent(0); got != 0 {
		t.Fatalf("normalizeSamplePercent(0) = %d, want 0", got)
	}
	if got := normalizeSamplePercent(37); got != 37 {
		t.Fatalf("normalizeSamplePercent(37) = %d, want 37", got)
	}
	if got := normalizeSamplePercent(100); got != 100 {
		t.Fatalf("normalizeSamplePercent(100) = %d, want 100", got)
	}
	if got := normalizeSamplePercent(101); got != 100 {
		t.Fatalf("normalizeSamplePercent(101) = %d, want 100", got)
	}
}

func TestShouldSampleRequest(t *testing.T) {
	t.Parallel()

	if shouldSampleRequest("req-1", 0) {
		t.Fatal("shouldSampleRequest should return false when samplePercent=0")
	}
	if !shouldSampleRequest("req-1", 100) {
		t.Fatal("shouldSampleRequest should return true when samplePercent=100")
	}
	if shouldSampleRequest("", 50) {
		t.Fatal("shouldSampleRequest should return false when requestID is empty and samplePercent is partial")
	}

	got1 := shouldSampleRequest("req-stable-1", 35)
	got2 := shouldSampleRequest("req-stable-1", 35)
	if got1 != got2 {
		t.Fatal("shouldSampleRequest should be stable for the same requestID and samplePercent")
	}
}

func TestShouldSampleCachesResultInContext(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Set(common.RequestIdKey, "req-cache-1")

	manager := &Manager{cfg: Config{SamplePercent: 100}}
	if !ShouldSample(ctx, manager) {
		t.Fatal("expected sampled request")
	}

	manager.cfg.SamplePercent = 0
	if !ShouldSample(ctx, manager) {
		t.Fatal("expected cached sampled result to be reused")
	}
}
