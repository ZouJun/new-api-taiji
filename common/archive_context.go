package common

import "github.com/gin-gonic/gin"

const consumeLogUpstreamUsageContextKey = "consume_log_upstream_usage"
const consumeLogClientUsageContextKey = "consume_log_client_usage"

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

func SetConsumeLogClientUsage(c *gin.Context, usage any) {
	if c == nil || usage == nil {
		return
	}
	c.Set(consumeLogClientUsageContextKey, usage)
}

func GetConsumeLogClientUsage(c *gin.Context) any {
	if c == nil {
		return nil
	}
	value, ok := c.Get(consumeLogClientUsageContextKey)
	if !ok {
		return nil
	}
	return value
}
