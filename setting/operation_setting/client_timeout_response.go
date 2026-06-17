package operation_setting

import "strings"

const (
	DefaultClientTimeoutResponseHTTPStatus = 0
	DefaultClientTimeoutResponseMessage    = ""
)

var (
	// ClientTimeoutResponseHTTPStatus controls the client-facing HTTP status code
	// used only when a timeout is caused by channel-level timeout settings.
	ClientTimeoutResponseHTTPStatus = DefaultClientTimeoutResponseHTTPStatus
	// ClientTimeoutResponseMessage controls the client-facing error message used
	// only when a timeout is caused by channel-level timeout settings.
	ClientTimeoutResponseMessage = DefaultClientTimeoutResponseMessage
)

func ResolveClientTimeoutResponse() (int, string) {
	return ClientTimeoutResponseHTTPStatus, strings.TrimSpace(ClientTimeoutResponseMessage)
}
