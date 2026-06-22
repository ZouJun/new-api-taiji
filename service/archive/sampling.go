package archive

import (
	"hash/fnv"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const sampledContextKey = "archive_sampled"

func normalizeSamplePercent(samplePercent int) int {
	if samplePercent <= 0 {
		return 0
	}
	if samplePercent >= 100 {
		return 100
	}
	return samplePercent
}

func shouldSampleRequest(requestID string, samplePercent int) bool {
	samplePercent = normalizeSamplePercent(samplePercent)
	if samplePercent == 0 {
		return false
	}
	if samplePercent == 100 {
		return true
	}
	if requestID == "" {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(requestID))
	return int(h.Sum32()%100) < samplePercent
}

func ShouldSample(c *gin.Context, m *Manager) bool {
	if m == nil || c == nil {
		return false
	}
	if cached, exists := c.Get(sampledContextKey); exists {
		if sampled, ok := cached.(bool); ok {
			return sampled
		}
	}
	sampled := shouldSampleRequest(c.GetString(common.RequestIdKey), m.cfg.SamplePercent)
	c.Set(sampledContextKey, sampled)
	return sampled
}
