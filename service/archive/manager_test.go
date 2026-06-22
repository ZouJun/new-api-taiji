package archive

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShouldUseSegmentStrategy_LocalSmallPayloadOnly(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Request:  ObjectInfo{Status: StatusPending, Bytes: 1024},
		Response: ObjectInfo{Status: StatusPending, Bytes: 2048},
	}

	assert.True(t, shouldUseSegmentStrategy(manifest, "local", 64<<10))
	assert.True(t, shouldUseSegmentStrategy(manifest, "azure_blob", 64<<10))
	assert.False(t, shouldUseSegmentStrategy(manifest, "local", 1024))
}

func TestRequiresArchiveProcessingSkipsHighLoadOnly(t *testing.T) {
	t.Parallel()

	assert.False(t, requiresArchiveProcessing(Manifest{
		Status: StatusSkipped,
		Reason: ReasonHighLoad,
	}))
	assert.True(t, requiresArchiveProcessing(Manifest{
		Status: StatusSkipped,
		Reason: ReasonNoRequestBody,
	}))
	assert.True(t, requiresArchiveProcessing(Manifest{
		Status: StatusSucceeded,
	}))
}

func TestPatchWorkerCount(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1, patchWorkerCount(1))
	assert.Equal(t, 2, patchWorkerCount(8))
	assert.Equal(t, 4, patchWorkerCount(24))
	assert.Equal(t, 8, patchWorkerCount(64))
}

func TestEnqueuePatchFallsBackWhenQueueMissing(t *testing.T) {
	t.Parallel()

	manager := &Manager{}
	done := make(chan struct{})
	go func() {
		manager.enqueuePatch(Manifest{RequestID: "req-no-queue"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueuePatch should return promptly when patchQueue is missing")
	}
}

func TestSegmentFlushInterval(t *testing.T) {
	t.Parallel()

	assert.Equal(t, time.Minute, segmentFlushInterval(Config{}))
	assert.Equal(t, time.Second, segmentFlushInterval(Config{SegmentMaxAgeSeconds: 1}))
	assert.Equal(t, 30*time.Second, segmentFlushInterval(Config{SegmentMaxAgeSeconds: 60}))
	assert.Equal(t, time.Minute, segmentFlushInterval(Config{SegmentMaxAgeSeconds: 600}))
}
