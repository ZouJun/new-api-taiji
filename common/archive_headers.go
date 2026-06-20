package common

import (
	"net/http"
	"strings"
)

var ArchiveTrackedRequestHeaders = []string{
	"Routify-Provider-Trace-Id",
	"X-Trace-Id",
	"Routify-Provider-Request-Id",
	"X-Request-ID",
	"X-Client-Request-Id",
	"X-Request-Id",
	"Routify-Provider-Color-Id",
	"x-conversation-id",
	"X-Session-Id",
}

func BuildArchiveRequestHeaderSnapshot(header http.Header, maxLen int) (map[string]string, map[string]bool) {
	snapshot := make(map[string]string, len(ArchiveTrackedRequestHeaders))
	truncated := make(map[string]bool)
	for _, key := range ArchiveTrackedRequestHeaders {
		value := ""
		if header != nil {
			value = header.Get(key)
		}
		if maxLen > 0 && len(value) > maxLen {
			value = value[:maxLen]
			truncated[key] = true
		}
		snapshot[key] = value
	}
	if len(truncated) == 0 {
		truncated = nil
	}
	return snapshot, truncated
}

func FormatArchiveRequestHeaderSnapshot(snapshot map[string]string) string {
	if len(snapshot) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ArchiveTrackedRequestHeaders))
	for _, key := range ArchiveTrackedRequestHeaders {
		parts = append(parts, key+"="+snapshot[key])
	}
	return strings.Join(parts, " ")
}

func FormatNonEmptyArchiveRequestHeaderSnapshot(snapshot map[string]string) string {
	if len(snapshot) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ArchiveTrackedRequestHeaders))
	for _, key := range ArchiveTrackedRequestHeaders {
		value := snapshot[key]
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, " ")
}
