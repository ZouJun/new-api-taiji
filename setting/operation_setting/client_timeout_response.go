package operation_setting

import "strings"

const (
	DefaultClientTimeoutResponseHTTPStatus = 0
	DefaultClientTimeoutResponseMessage    = ""
)

var (
	ClientTimeoutResponseHTTPStatus = DefaultClientTimeoutResponseHTTPStatus
	ClientTimeoutResponseMessage    = DefaultClientTimeoutResponseMessage
)

func ResolveClientTimeoutResponse() (int, string) {
	return ClientTimeoutResponseHTTPStatus, strings.TrimSpace(ClientTimeoutResponseMessage)
}
