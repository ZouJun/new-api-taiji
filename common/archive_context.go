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

func SetConsumeLogClientUsageFromJSON(c *gin.Context, payload []byte, paths ...string) bool {
	if c == nil || len(payload) == 0 || len(paths) == 0 {
		return false
	}
	var body map[string]any
	if err := Unmarshal(payload, &body); err != nil {
		return false
	}
	for _, path := range paths {
		if usage, ok := getJSONValueByPath(body, path); ok && usage != nil {
			SetConsumeLogClientUsage(c, usage)
			return true
		}
	}
	return false
}

func SetConsumeLogClientUsageFromJSONString(c *gin.Context, payload string, paths ...string) bool {
	if payload == "" {
		return false
	}
	return SetConsumeLogClientUsageFromJSON(c, StringToByteSlice(payload), paths...)
}

func getJSONValueByPath(body map[string]any, path string) (any, bool) {
	if body == nil || path == "" {
		return nil, false
	}
	current := any(body)
	segment := ""
	for i := 0; i <= len(path); i++ {
		if i < len(path) && path[i] != '.' {
			segment += string(path[i])
			continue
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := object[segment]
		if !ok {
			return nil, false
		}
		current = next
		segment = ""
	}
	return current, true
}
