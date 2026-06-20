package common

import "github.com/gin-gonic/gin"

const consumeLogUpstreamUsageContextKey = "consume_log_upstream_usage"

func SetConsumeLogUpstreamUsage(c *gin.Context, usage map[string]interface{}) {
	if c == nil || usage == nil {
		return
	}
	c.Set(consumeLogUpstreamUsageContextKey, usage)
}

func GetConsumeLogUpstreamUsage(c *gin.Context) map[string]interface{} {
	if c == nil {
		return nil
	}
	value, ok := c.Get(consumeLogUpstreamUsageContextKey)
	if !ok || value == nil {
		return nil
	}
	usage, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return usage
}
