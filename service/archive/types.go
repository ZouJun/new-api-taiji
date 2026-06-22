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
	ReasonHighLoad            = "high_load"
	ReasonNoRequestBody       = "no_request_body"
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
	Stage           string `json:"stage,omitempty"`
	Fallback        bool   `json:"fallback,omitempty"`
	Payload         []byte `json:"-"`
	SpoolPath       string `json:"-"`
}

type PayloadCapture struct {
	RequestStage     string `json:"request_stage,omitempty"`
	ResponseStage    string `json:"response_stage,omitempty"`
	RequestFallback  bool   `json:"request_fallback,omitempty"`
	ResponseFallback bool   `json:"response_fallback,omitempty"`
}

type SegmentReference struct {
	SegmentID          string `json:"segment_id,omitempty"`
	SegmentData        string `json:"segment_data,omitempty"`
	SegmentIndex       string `json:"segment_index,omitempty"`
	SegmentDataRemote  string `json:"segment_data_remote,omitempty"`
	SegmentIndexRemote string `json:"segment_index_remote,omitempty"`
	RequestOffset      int64  `json:"request_offset,omitempty"`
	RequestLength      int64  `json:"request_length,omitempty"`
	ResponseOffset     int64  `json:"response_offset,omitempty"`
	ResponseLength     int64  `json:"response_length,omitempty"`
	Shard              int    `json:"shard,omitempty"`
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
	RequestID               string            `json:"request_id"`
	Method                  string            `json:"method"`
	Path                    string            `json:"path"`
	StatusCode              int               `json:"status_code"`
	IsStream                bool              `json:"is_stream"`
	Backend                 string            `json:"backend"`
	StorageMode             string            `json:"storage_mode"`
	Strategy                string            `json:"strategy,omitempty"`
	Status                  string            `json:"status"`
	Reason                  string            `json:"reason,omitempty"`
	SkipDetail              string            `json:"skip_detail,omitempty"`
	CPUThresholdExceeded    bool              `json:"cpu_threshold_exceeded,omitempty"`
	MemoryThresholdExceeded bool              `json:"memory_threshold_exceeded,omitempty"`
	DiskThresholdExceeded   bool              `json:"disk_threshold_exceeded,omitempty"`
	CreatedAt               string            `json:"created_at"`
	CompletedAt             string            `json:"completed_at,omitempty"`
	Provider                string            `json:"provider,omitempty"`
	Model                   string            `json:"model,omitempty"`
	ChannelID               int               `json:"channel_id,omitempty"`
	ChannelType             int               `json:"channel_type,omitempty"`
	ChannelName             string            `json:"channel_name,omitempty"`
	RequestHeader           map[string]string `json:"request_header,omitempty"`
	RequestHeaderTruncated  map[string]bool   `json:"request_header_truncated,omitempty"`
	UpstreamUsage           map[string]any    `json:"upstream_usage,omitempty"`
	PayloadCapture          PayloadCapture    `json:"payload_capture,omitempty"`
	Request                 ObjectInfo        `json:"request"`
	Response                ObjectInfo        `json:"response"`
	Segment                 *SegmentReference `json:"segment,omitempty"`
	Attempts                []AttemptSummary  `json:"attempts,omitempty"`
}

type Job struct {
	Manifest Manifest
}

type backendObject struct {
	Name            string
	ObjectType      string
	LocalPath       string
	Data            []byte
	HasInlineData   bool
	ContentType     string
	ContentEncoding string
	Metadata        map[string]string
	AllowMissing    bool
}

type Backend interface {
	WriteFinal(manifest Manifest, objects []backendObject) error
}

type runtimeState struct {
	startedAt               time.Time
	backend                 string
	smallPayloadMaxBytes    int64
	clientResponse          *ObjectInfo
	upstreamResponse        *ObjectInfo
	skipArchive             bool
	skipReason              string
	skipDetail              string
	cpuThresholdExceeded    bool
	memoryThresholdExceeded bool
	diskThresholdExceeded   bool
}
