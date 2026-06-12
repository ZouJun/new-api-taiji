package archive

import "time"

const (
	StatusPending   = "pending"
	StatusQueued    = "queued"
	StatusSucceeded = "succeeded"
	StatusPartial   = "partial"
	StatusSkipped   = "skipped"
	StatusFailed    = "failed"
)

const (
	ObjectManifest = "manifest"
	ObjectRequest  = "request"
	ObjectResponse = "response"
)

const (
	ReasonDisabled            = "disabled"
	ReasonQueueFull           = "queue_full"
	ReasonRequestSizeLimit    = "request_size_limit"
	ReasonResponseSizeLimit   = "response_size_limit"
	ReasonRequestSpoolFailed  = "request_spool_failed"
	ReasonResponseSpoolFailed = "response_spool_failed"
	ReasonBackendWriteFailed  = "backend_write_failed"
)

type ObjectInfo struct {
	Status          string `json:"status"`
	Reason          string `json:"reason,omitempty"`
	Bytes           int64  `json:"bytes"`
	ContentType     string `json:"content_type,omitempty"`
	ContentEncoding string `json:"content_encoding,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
	SpoolPath       string `json:"-"`
}

type AttemptSummary struct {
	Index         int    `json:"index"`
	ChannelID     int    `json:"channel_id,omitempty"`
	ChannelName   string `json:"channel_name,omitempty"`
	ChannelType   int    `json:"channel_type,omitempty"`
	Outcome       string `json:"outcome"`
	HTTPStatus    int    `json:"http_status,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	TimeoutReason string `json:"timeout_reason,omitempty"`
	StartedAt     int64  `json:"started_at"`
	DurationMs    int64  `json:"duration_ms"`
}

type Manifest struct {
	RequestID   string           `json:"request_id"`
	Method      string           `json:"method"`
	Path        string           `json:"path"`
	StatusCode  int              `json:"status_code"`
	IsStream    bool             `json:"is_stream"`
	Backend     string           `json:"backend"`
	StorageMode string           `json:"storage_mode"`
	Status      string           `json:"status"`
	Reason      string           `json:"reason,omitempty"`
	CreatedAt   string           `json:"created_at"`
	CompletedAt string           `json:"completed_at,omitempty"`
	Request     ObjectInfo       `json:"request"`
	Response    ObjectInfo       `json:"response"`
	Attempts    []AttemptSummary `json:"attempts,omitempty"`
}

type Job struct {
	Manifest Manifest
}

type backendObject struct {
	Name         string
	ObjectType   string
	LocalPath    string
	ContentType  string
	Metadata     map[string]string
	AllowMissing bool
}

type Backend interface {
	WritePlaceholder(manifest Manifest) error
	WriteFinal(manifest Manifest, objects []backendObject) error
}

type runtimeState struct {
	startedAt        time.Time
	responseFilePath string
}
