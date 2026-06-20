package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeArchiveMapsMergesNestedObjects(t *testing.T) {
	t.Parallel()

	existing := map[string]interface{}{
		"status": "queued",
		"segment": map[string]interface{}{
			"segment_id":   "seg-1",
			"segment_data": "/tmp/data",
		},
	}
	incoming := map[string]interface{}{
		"status": "succeeded",
		"segment": map[string]interface{}{
			"segment_data_remote": "segments/seg-1.data",
		},
	}

	merged := mergeArchiveMaps(existing, incoming)

	assert.Equal(t, "succeeded", merged["status"])
	segment := merged["segment"].(map[string]interface{})
	assert.Equal(t, "seg-1", segment["segment_id"])
	assert.Equal(t, "/tmp/data", segment["segment_data"])
	assert.Equal(t, "segments/seg-1.data", segment["segment_data_remote"])
}
