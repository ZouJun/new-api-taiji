package common

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildArchiveRequestHeaderSnapshotIncludesMissingAndTruncation(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set("X-Trace-Id", "trace-123")
	header.Set("X-Session-Id", "abcdef")

	snapshot, truncated := BuildArchiveRequestHeaderSnapshot(header, 4)

	require.Len(t, snapshot, len(ArchiveTrackedRequestHeaders))
	assert.Equal(t, "", snapshot["Routify-Provider-Trace-Id"])
	assert.Equal(t, "trac", snapshot["X-Trace-Id"])
	assert.Equal(t, "abcd", snapshot["X-Session-Id"])
	require.NotNil(t, truncated)
	assert.True(t, truncated["X-Trace-Id"])
	assert.True(t, truncated["X-Session-Id"])
}
