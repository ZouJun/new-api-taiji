package archive

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const contextKey = "archive_state"
const managerContextKey = "archive_manager"
const retiredManagerShutdownDelay = 30 * time.Second

var (
	managerMu sync.RWMutex
	manager   *Manager
)

type Manager struct {
	cfg        Config
	backend    Backend
	queue      chan Job
	patchQueue chan Manifest
	segmenter  *segmentStore
	stopCh     chan struct{}
	stopOnce   sync.Once
	retired    atomic.Bool
	refs       atomic.Int64
}

type State struct {
	Manifest Manifest
	runtime  runtimeState
	mu       sync.Mutex
}

func Init() {
	cfg := loadConfig()
	if !cfg.Enabled {
		retireManager(setManager(nil))
		return
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 50000
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 32
	}
	if cfg.MaxRequestBytes <= 0 {
		cfg.MaxRequestBytes = int64(128) << 20
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = int64(128) << 20
	}
	if cfg.SpoolTTLHours <= 0 {
		cfg.SpoolTTLHours = 24
	}
	if cfg.SmallPayloadMaxBytes <= 0 {
		cfg.SmallPayloadMaxBytes = int64(64) << 10
	}
	if cfg.SegmentMaxBytes <= 0 {
		cfg.SegmentMaxBytes = int64(256) << 20
	}
	if cfg.SegmentMaxAgeSeconds <= 0 {
		cfg.SegmentMaxAgeSeconds = 60
	}
	if cfg.SegmentMaxRecords <= 0 {
		cfg.SegmentMaxRecords = 50000
	}
	if cfg.SegmentShardCount <= 0 {
		cfg.SegmentShardCount = 16
	}

	var backend Backend
	switch cfg.Backend {
	case "", "local":
		cfg.Backend = "local"
		backend = newLocalBackend(cfg)
	case "azure_blob":
		backend = newAzureBlobBackend(cfg)
	default:
		common.SysError("archive disabled: unsupported ARCHIVE_BACKEND=" + cfg.Backend)
		retireManager(setManager(nil))
		return
	}

	if err := ensureDir(cfg.SpoolDir); err != nil {
		common.SysError("archive disabled: failed to create spool dir: " + err.Error())
		retireManager(setManager(nil))
		return
	}
	if cfg.Backend == "local" {
		if err := ensureDir(cfg.ObjectsDir); err != nil {
			common.SysError("archive disabled: failed to create objects dir: " + err.Error())
			retireManager(setManager(nil))
			return
		}
	}
	if err := ensureDir(cfg.SegmentsDir); err != nil {
		common.SysError("archive disabled: failed to create segments dir: " + err.Error())
		retireManager(setManager(nil))
		return
	}

	segmenter, err := newSegmentStore(cfg, backend)
	if err != nil {
		common.SysError("archive disabled: failed to initialize segment store: " + err.Error())
		retireManager(setManager(nil))
		return
	}
	m := &Manager{
		cfg:        cfg,
		backend:    backend,
		queue:      make(chan Job, cfg.QueueSize),
		patchQueue: make(chan Manifest, cfg.QueueSize),
		segmenter:  segmenter,
		stopCh:     make(chan struct{}),
	}
	for i := 0; i < cfg.WorkerCount; i++ {
		go m.worker()
	}
	for i := 0; i < patchWorkerCount(cfg.WorkerCount); i++ {
		go m.patchWorker()
	}
	go m.cleanupLoop()
	retireManager(setManager(m))
	common.SysLog(fmt.Sprintf("archive initialized: backend=%s queue_size=%d worker_count=%d spool=%s", cfg.Backend, cfg.QueueSize, cfg.WorkerCount, cfg.SpoolDir))
}

func setManager(m *Manager) *Manager {
	managerMu.Lock()
	previous := manager
	manager = m
	managerMu.Unlock()
	return previous
}

func retireManager(m *Manager) {
	if m == nil {
		return
	}
	m.retired.Store(true)
	if m.refs.Load() == 0 {
		go func(retired *Manager) {
			time.Sleep(retiredManagerShutdownDelay)
			if retired.refs.Load() == 0 {
				retired.stop()
			}
		}(m)
	}
}

func (m *Manager) stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

func (m *Manager) Retain() {
	if m == nil {
		return
	}
	m.refs.Add(1)
}

func (m *Manager) Release() {
	if m == nil {
		return
	}
	remaining := m.refs.Add(-1)
	if remaining <= 0 && m.retired.Load() {
		go func(retired *Manager) {
			time.Sleep(retiredManagerShutdownDelay)
			if retired.refs.Load() == 0 {
				retired.stop()
			}
		}(m)
	}
}

func BindManager(c *gin.Context, m *Manager) {
	if c != nil && m != nil {
		c.Set(managerContextKey, m)
	}
}

func ManagerForContext(c *gin.Context) *Manager {
	if c != nil {
		if value, ok := c.Get(managerContextKey); ok {
			if m, ok := value.(*Manager); ok && m != nil {
				return m
			}
		}
	}
	return Current()
}

func currentManager() *Manager {
	managerMu.RLock()
	defer managerMu.RUnlock()
	return manager
}

func Current() *Manager {
	return currentManager()
}

func (m *Manager) SpoolDir() string {
	return m.cfg.SpoolDir
}

func (m *Manager) MaxResponseBytes() int64 {
	return m.cfg.MaxResponseBytes
}

func (m *Manager) SmallPayloadMaxBytes() int64 {
	return m.cfg.SmallPayloadMaxBytes
}

func (m *Manager) HeaderValueMaxLength() int {
	return m.cfg.HeaderValueMaxLength
}

func (m *Manager) CaptureRequest(src io.Reader) (ObjectInfo, error) {
	return captureObject(m.cfg.SpoolDir, ObjectRequest, src, m.cfg.MaxRequestBytes, m.cfg.SmallPayloadMaxBytes)
}

func Attach(c *gin.Context, state *State) {
	c.Set(contextKey, state)
}

func FromContext(c *gin.Context) (*State, bool) {
	value, ok := c.Get(contextKey)
	if !ok || value == nil {
		return nil, false
	}
	state, ok := value.(*State)
	return state, ok
}

func NewState(c *gin.Context, routeMode string, manager *Manager) *State {
	now := time.Now()
	requestID := c.GetString(common.RequestIdKey)
	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	backend := ""
	smallPayloadMaxBytes := int64(0)
	if manager != nil {
		backend = manager.cfg.Backend
		smallPayloadMaxBytes = manager.cfg.SmallPayloadMaxBytes
	}
	return &State{
		Manifest: Manifest{
			RequestID:   requestID,
			Method:      c.Request.Method,
			Path:        path,
			Backend:     backend,
			StorageMode: routeMode,
			Status:      StatusPending,
			CreatedAt:   now.UTC().Format(time.RFC3339Nano),
			Request:     ObjectInfo{Status: StatusPending},
			Response:    ObjectInfo{Status: StatusPending},
		},
		runtime: runtimeState{
			startedAt:            now,
			backend:              backend,
			smallPayloadMaxBytes: smallPayloadMaxBytes,
		},
	}
}

func (s *State) AddAttempt(attempt AttemptSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Manifest.Attempts = append(s.Manifest.Attempts, attempt)
}

func (s *State) SetRequest(info ObjectInfo, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info.ContentType = contentType
	s.Manifest.Request = info
}

func (s *State) SetResponse(info ObjectInfo, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info.ContentType = contentType
	s.Manifest.Response = info
}

func (s *State) SetClientResponse(info ObjectInfo, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info.ContentType = contentType
	s.runtime.clientResponse = &info
}

func (s *State) SetUpstreamResponse(info ObjectInfo, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info.ContentType = contentType
	s.runtime.upstreamResponse = &info
}

func (s *State) SetSkip(reason string, detail string, cpuExceeded bool, memoryExceeded bool, diskExceeded bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runtime.skipArchive = true
	s.runtime.skipReason = reason
	s.runtime.skipDetail = detail
	s.runtime.cpuThresholdExceeded = cpuExceeded
	s.runtime.memoryThresholdExceeded = memoryExceeded
	s.runtime.diskThresholdExceeded = diskExceeded
	s.Manifest.Status = StatusSkipped
	s.Manifest.Reason = reason
	s.Manifest.SkipDetail = detail
	s.Manifest.CPUThresholdExceeded = cpuExceeded
	s.Manifest.MemoryThresholdExceeded = memoryExceeded
	s.Manifest.DiskThresholdExceeded = diskExceeded
}

func (s *State) SetRequestHeader(snapshot map[string]string, truncated map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Manifest.RequestHeader = snapshot
	s.Manifest.RequestHeaderTruncated = truncated
}

func (s *State) SetUpstreamUsage(usage map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Manifest.UpstreamUsage = usage
}

func (s *State) PopulateFromContext(c *gin.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Manifest.Provider = c.GetString("base_url")
	s.Manifest.Model = c.GetString("original_model")
	s.Manifest.ChannelID = c.GetInt("channel_id")
	s.Manifest.ChannelType = c.GetInt("channel_type")
	s.Manifest.ChannelName = c.GetString("channel_name")
}

func (s *State) Snapshot(statusCode int, isStream bool) Manifest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Manifest.StatusCode = statusCode
	s.Manifest.IsStream = isStream
	s.Manifest.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if s.runtime.upstreamResponse != nil {
		s.Manifest.Response = *s.runtime.upstreamResponse
		s.Manifest.PayloadCapture.ResponseStage = s.runtime.upstreamResponse.Stage
		s.Manifest.PayloadCapture.ResponseFallback = s.runtime.upstreamResponse.Fallback
	} else if s.runtime.clientResponse != nil {
		s.Manifest.Response = *s.runtime.clientResponse
		s.Manifest.PayloadCapture.ResponseStage = s.runtime.clientResponse.Stage
		s.Manifest.PayloadCapture.ResponseFallback = s.runtime.clientResponse.Fallback
	}
	s.Manifest.PayloadCapture.RequestStage = s.Manifest.Request.Stage
	s.Manifest.PayloadCapture.RequestFallback = s.Manifest.Request.Fallback
	if s.runtime.skipArchive {
		s.Manifest.Status = StatusSkipped
		s.Manifest.Reason = s.runtime.skipReason
		s.Manifest.SkipDetail = s.runtime.skipDetail
		return s.Manifest
	}
	if shouldUseSegmentStrategy(s.Manifest, s.runtime.backend, s.runtime.smallPayloadMaxBytes) {
		s.Manifest.Strategy = "segmented"
	} else {
		s.Manifest.Strategy = "per_request"
	}
	s.Manifest.Status, s.Manifest.Reason = summarizeStatus(s.Manifest.Request, s.Manifest.Response)
	return s.Manifest
}

func (m *Manager) Enqueue(c *gin.Context, manifest Manifest) {
	if !requiresArchiveProcessing(manifest) {
		m.enqueuePatch(manifest)
		return
	}
	queued, manifest := m.tryEnqueue(manifest)
	if queued {
		return
	}
	logArchiveEvent(c, manifest, StatusSkipped, ReasonQueueFull, "archive queue is full")
	m.enqueuePatch(manifest)
}

func (m *Manager) tryEnqueue(manifest Manifest) (bool, Manifest) {
	select {
	case m.queue <- Job{Manifest: manifest}:
		return true, manifest
	default:
		manifest.Status = StatusSkipped
		manifest.Reason = ReasonQueueFull
		return false, manifest
	}
}

func (m *Manager) worker() {
	for {
		select {
		case <-m.stopCh:
			return
		case job := <-m.queue:
			m.process(job)
		}
	}
}

func (m *Manager) patchWorker() {
	for {
		select {
		case <-m.stopCh:
			return
		case manifest := <-m.patchQueue:
			patchArchive(nil, manifest)
		}
	}
}

func (m *Manager) process(job Job) {
	manifest := job.Manifest
	if manifest.Strategy == "segmented" && m.segmenter != nil {
		updatedManifest, err := m.segmenter.Append(manifest)
		if err != nil {
			manifest.Status = StatusFailed
			manifest.Reason = ReasonBackendWriteFailed
			logArchiveEvent(nil, manifest, StatusFailed, ReasonBackendWriteFailed, err.Error())
			m.enqueuePatch(manifest)
			return
		}
		manifest = updatedManifest
		if err := m.backend.WriteFinal(manifest, nil); err != nil {
			manifest.Status = StatusFailed
			manifest.Reason = ReasonBackendWriteFailed
			logArchiveEvent(nil, manifest, StatusFailed, ReasonBackendWriteFailed, err.Error())
			m.enqueuePatch(manifest)
			return
		}
		removeIfPresent(manifest.Request.SpoolPath)
		removeIfPresent(manifest.Response.SpoolPath)
		m.enqueuePatch(manifest)
		return
	}
	objects := []backendObject{
		{
			Name:            "request.data",
			ObjectType:      ObjectRequest,
			LocalPath:       manifest.Request.SpoolPath,
			Data:            manifest.Request.Payload,
			HasInlineData:   manifest.Request.Payload != nil,
			ContentType:     manifest.Request.ContentType,
			ContentEncoding: manifest.Request.ContentEncoding,
			Metadata:        metadataFor(manifest, ObjectRequest, manifest.Request),
			AllowMissing:    manifest.Request.Status == StatusSkipped,
		},
		{
			Name:            "response.data",
			ObjectType:      ObjectResponse,
			LocalPath:       manifest.Response.SpoolPath,
			Data:            manifest.Response.Payload,
			HasInlineData:   manifest.Response.Payload != nil,
			ContentType:     manifest.Response.ContentType,
			ContentEncoding: manifest.Response.ContentEncoding,
			Metadata:        metadataFor(manifest, ObjectResponse, manifest.Response),
			AllowMissing:    manifest.Response.Status == StatusSkipped,
		},
	}
	if err := m.backend.WriteFinal(manifest, objects); err != nil {
		manifest.Status = StatusFailed
		manifest.Reason = ReasonBackendWriteFailed
		logArchiveEvent(nil, manifest, StatusFailed, ReasonBackendWriteFailed, err.Error())
		m.enqueuePatch(manifest)
		return
	}
	removeIfPresent(manifest.Request.SpoolPath)
	removeIfPresent(manifest.Response.SpoolPath)
	m.enqueuePatch(manifest)
	if manifest.Request.Status == StatusSkipped {
		logArchiveEvent(nil, manifest, StatusSkipped, manifest.Request.Reason, "request object skipped")
	}
	if manifest.Response.Status == StatusSkipped {
		logArchiveEvent(nil, manifest, StatusSkipped, manifest.Response.Reason, "response object skipped")
	}
}

func (m *Manager) cleanupLoop() {
	ttl := time.Duration(m.cfg.SpoolTTLHours) * time.Hour
	ticker := time.NewTicker(segmentFlushInterval(m.cfg))
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			cleanupExpiredSpoolFiles(m.cfg.SpoolDir, ttl)
			if m.segmenter != nil {
				m.segmenter.FlushExpired()
			}
		}
	}
}

func segmentFlushInterval(cfg Config) time.Duration {
	if cfg.SegmentMaxAgeSeconds <= 0 {
		return time.Minute
	}
	interval := time.Duration(cfg.SegmentMaxAgeSeconds) * time.Second / 2
	if interval < time.Second {
		return time.Second
	}
	if interval > time.Minute {
		return time.Minute
	}
	return interval
}

func (m *Manager) enqueuePatch(manifest Manifest) {
	if m == nil || m.patchQueue == nil {
		go patchArchive(nil, manifest)
		return
	}
	select {
	case m.patchQueue <- manifest:
	default:
		common.SysError(fmt.Sprintf("archive patch queue full: request_id=%s", manifest.RequestID))
		go patchArchive(nil, manifest)
	}
}

func patchWorkerCount(workerCount int) int {
	if workerCount <= 4 {
		return 1
	}
	if workerCount <= 16 {
		return 2
	}
	if workerCount <= 32 {
		return 4
	}
	return 8
}

func requiresArchiveProcessing(manifest Manifest) bool {
	return !(manifest.Status == StatusSkipped && manifest.Reason == ReasonHighLoad)
}

func summarizeStatus(request ObjectInfo, response ObjectInfo) (string, string) {
	if request.Status == StatusFailed {
		return StatusFailed, request.Reason
	}
	if response.Status == StatusFailed {
		return StatusFailed, response.Reason
	}
	if request.Status == StatusSkipped {
		return StatusPartial, request.Reason
	}
	if response.Status == StatusSkipped {
		return StatusPartial, response.Reason
	}
	return StatusSucceeded, ""
}

func metadataFor(manifest Manifest, objectType string, object ObjectInfo) map[string]string {
	metadata := map[string]string{
		"request_id":   manifest.RequestID,
		"object_type":  objectType,
		"is_stream":    fmt.Sprintf("%t", manifest.IsStream),
		"storage_mode": manifest.StorageMode,
		"content_type": object.ContentType,
		"created_at":   manifest.CreatedAt,
	}
	if object.ContentEncoding != "" {
		metadata["content_encoding"] = object.ContentEncoding
	}
	return metadata
}

func patchArchive(c *gin.Context, manifest Manifest) {
	info := map[string]interface{}{
		"status":                    manifest.Status,
		"reason":                    manifest.Reason,
		"skip_detail":               manifest.SkipDetail,
		"cpu_threshold_exceeded":    manifest.CPUThresholdExceeded,
		"memory_threshold_exceeded": manifest.MemoryThresholdExceeded,
		"disk_threshold_exceeded":   manifest.DiskThresholdExceeded,
		"backend":                   manifest.Backend,
		"storage_mode":              manifest.StorageMode,
		"strategy":                  manifest.Strategy,
		"request_id":                manifest.RequestID,
		"request": map[string]interface{}{
			"status":   manifest.Request.Status,
			"reason":   manifest.Request.Reason,
			"bytes":    manifest.Request.Bytes,
			"stage":    manifest.Request.Stage,
			"fallback": manifest.Request.Fallback,
		},
		"response": map[string]interface{}{
			"status":   manifest.Response.Status,
			"reason":   manifest.Response.Reason,
			"bytes":    manifest.Response.Bytes,
			"stage":    manifest.Response.Stage,
			"fallback": manifest.Response.Fallback,
		},
	}
	if manifest.Segment != nil {
		info["segment"] = map[string]interface{}{
			"segment_id":           manifest.Segment.SegmentID,
			"segment_data":         manifest.Segment.SegmentData,
			"segment_index":        manifest.Segment.SegmentIndex,
			"segment_data_remote":  manifest.Segment.SegmentDataRemote,
			"segment_index_remote": manifest.Segment.SegmentIndexRemote,
			"request_offset":       manifest.Segment.RequestOffset,
			"request_length":       manifest.Segment.RequestLength,
			"response_offset":      manifest.Segment.ResponseOffset,
			"response_length":      manifest.Segment.ResponseLength,
			"shard":                manifest.Segment.Shard,
		}
	}
	if manifest.RequestHeader != nil {
		info["request_header"] = manifest.RequestHeader
	}
	if manifest.RequestHeaderTruncated != nil {
		info["request_header_truncated"] = manifest.RequestHeaderTruncated
	}
	if manifest.UpstreamUsage != nil {
		info["upstream_usage"] = manifest.UpstreamUsage
	}
	if manifest.SkipDetail != "" {
		info["skip_detail"] = manifest.SkipDetail
	}
	for i := 0; i < 5; i++ {
		if err := model.PatchLogOtherArchiveByRequestID(manifest.RequestID, info); err != nil {
			if i == 4 && c != nil {
				logger.LogWarn(c, "failed to patch archive log metadata: "+err.Error())
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return
	}
}

func shouldUseSegmentStrategy(manifest Manifest, backend string, smallPayloadMaxBytes int64) bool {
	if backend != "local" && backend != "azure_blob" {
		return false
	}
	if smallPayloadMaxBytes <= 0 {
		return false
	}
	if manifest.Request.Status != StatusPending || manifest.Response.Status != StatusPending {
		return false
	}
	return manifest.Request.Bytes <= smallPayloadMaxBytes && manifest.Response.Bytes <= smallPayloadMaxBytes
}

func logArchiveEvent(c *gin.Context, manifest Manifest, status string, reason string, detail string) {
	msg := fmt.Sprintf("archive %s: request_id=%s backend=%s reason=%s detail=%s", status, manifest.RequestID, manifest.Backend, reason, detail)
	if status == StatusSkipped {
		logger.LogWarn(c, msg)
		return
	}
	if status == StatusFailed {
		logger.LogError(c, msg)
	}
}

func removeIfPresent(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
