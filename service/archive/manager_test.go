package archive

import (
	"testing"

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
