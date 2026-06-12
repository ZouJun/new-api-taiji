package archive

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const contextKey = "archive_state"

var (
	managerMu sync.RWMutex
	manager   *Manager
)

type Manager struct {
	cfg     Config
	backend Backend
	queue   chan Job
}

type State struct {
	Manifest Manifest
	runtime  runtimeState
	mu       sync.Mutex
}

func Init() {
	cfg := loadConfig()
	if !cfg.Enabled {
		setManager(nil)
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

	var backend Backend
	switch cfg.Backend {
	case "", "local":
		cfg.Backend = "local"
		backend = newLocalBackend(cfg)
	case "azure_blob":
		backend = newAzureBlobBackend(cfg)
	default:
		common.SysError("archive disabled: unsupported ARCHIVE_BACKEND=" + cfg.Backend)
		setManager(nil)
		return
	}

	if err := ensureDir(cfg.SpoolDir); err != nil {
		common.SysError("archive disabled: failed to create spool dir: " + err.Error())
		setManager(nil)
		return
	}
	if cfg.Backend == "local" {
		if err := ensureDir(cfg.ObjectsDir); err != nil {
			common.SysError("archive disabled: failed to create objects dir: " + err.Error())
			setManager(nil)
			return
		}
	}

	m := &Manager{
		cfg:     cfg,
		backend: backend,
		queue:   make(chan Job, cfg.QueueSize),
	}
	for i := 0; i < cfg.WorkerCount; i++ {
		go m.worker()
	}
	go m.cleanupLoop()
	setManager(m)
	common.SysLog(fmt.Sprintf("archive initialized: backend=%s queue_size=%d worker_count=%d spool=%s", cfg.Backend, cfg.QueueSize, cfg.WorkerCount, cfg.SpoolDir))
}

func setManager(m *Manager) {
	managerMu.Lock()
	defer managerMu.Unlock()
	manager = m
}

func currentManager() *Manager {
	managerMu.RLock()
	defer managerMu.RUnlock()
	return manager
}

func Current() *Manager {
	return currentManager()
}

func Enabled() bool {
	return currentManager() != nil
}

func (m *Manager) SpoolDir() string {
	return m.cfg.SpoolDir
}

func (m *Manager) MaxResponseBytes() int64 {
	return m.cfg.MaxResponseBytes
}

func (m *Manager) CaptureRequest(src io.Reader) (ObjectInfo, error) {
	return copyToSpool(m.cfg.SpoolDir, ObjectRequest, src, m.cfg.MaxRequestBytes)
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

func NewState(c *gin.Context, routeMode string) *State {
	now := time.Now()
	requestID := c.GetString(common.RequestIdKey)
	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	return &State{
		Manifest: Manifest{
			RequestID:   requestID,
			Method:      c.Request.Method,
			Path:        path,
			Backend:     currentManager().cfg.Backend,
			StorageMode: routeMode,
			Status:      StatusPending,
			CreatedAt:   now.UTC().Format(time.RFC3339Nano),
			Request:     ObjectInfo{Status: StatusPending},
			Response:    ObjectInfo{Status: StatusPending},
		},
		runtime: runtimeState{startedAt: now},
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

func (s *State) Snapshot(statusCode int, isStream bool) Manifest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Manifest.StatusCode = statusCode
	s.Manifest.IsStream = isStream
	s.Manifest.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.Manifest.Status, s.Manifest.Reason = summarizeStatus(s.Manifest.Request, s.Manifest.Response)
	return s.Manifest
}

func (m *Manager) WritePlaceholder(state *State) {
	if state == nil {
		return
	}
	manifest := state.Snapshot(0, false)
	if err := m.backend.WritePlaceholder(manifest); err != nil {
		logArchiveEvent(nil, manifest, StatusFailed, ReasonBackendWriteFailed, err.Error())
	}
}

func (m *Manager) Enqueue(c *gin.Context, manifest Manifest) {
	queued, manifest := m.tryEnqueue(manifest)
	if queued {
		patchArchive(c, manifest)
		return
	}
	logArchiveEvent(c, manifest, StatusSkipped, ReasonQueueFull, "archive queue is full")
	patchArchive(c, manifest)
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
	for job := range m.queue {
		m.process(job)
	}
}

func (m *Manager) process(job Job) {
	manifest := job.Manifest
	objects := []backendObject{
		{
			Name:         "request.data.gz",
			ObjectType:   ObjectRequest,
			LocalPath:    manifest.Request.SpoolPath,
			ContentType:  manifest.Request.ContentType,
			Metadata:     metadataFor(manifest, ObjectRequest, manifest.Request),
			AllowMissing: manifest.Request.Status == StatusSkipped,
		},
		{
			Name:         "response.data.gz",
			ObjectType:   ObjectResponse,
			LocalPath:    manifest.Response.SpoolPath,
			ContentType:  manifest.Response.ContentType,
			Metadata:     metadataFor(manifest, ObjectResponse, manifest.Response),
			AllowMissing: manifest.Response.Status == StatusSkipped,
		},
	}
	if err := m.backend.WriteFinal(manifest, objects); err != nil {
		manifest.Status = StatusFailed
		manifest.Reason = ReasonBackendWriteFailed
		logArchiveEvent(nil, manifest, StatusFailed, ReasonBackendWriteFailed, err.Error())
		patchArchive(nil, manifest)
		return
	}
	removeIfPresent(manifest.Request.SpoolPath)
	removeIfPresent(manifest.Response.SpoolPath)
	patchArchive(nil, manifest)
	if manifest.Request.Status == StatusSkipped {
		logArchiveEvent(nil, manifest, StatusSkipped, manifest.Request.Reason, "request object skipped")
	}
	if manifest.Response.Status == StatusSkipped {
		logArchiveEvent(nil, manifest, StatusSkipped, manifest.Response.Reason, "response object skipped")
	}
}

func (m *Manager) cleanupLoop() {
	ttl := time.Duration(m.cfg.SpoolTTLHours) * time.Hour
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		cleanupExpiredSpoolFiles(m.cfg.SpoolDir, ttl)
		<-ticker.C
	}
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
	return map[string]string{
		"request_id":       manifest.RequestID,
		"object_type":      objectType,
		"is_stream":        fmt.Sprintf("%t", manifest.IsStream),
		"storage_mode":     manifest.StorageMode,
		"content_encoding": "gzip",
		"content_type":     object.ContentType,
		"created_at":       manifest.CreatedAt,
	}
}

func patchArchive(c *gin.Context, manifest Manifest) {
	info := map[string]interface{}{
		"status":       manifest.Status,
		"reason":       manifest.Reason,
		"backend":      manifest.Backend,
		"storage_mode": manifest.StorageMode,
		"request_id":   manifest.RequestID,
		"request": map[string]interface{}{
			"status": manifest.Request.Status,
			"reason": manifest.Request.Reason,
			"bytes":  manifest.Request.Bytes,
			"sha256": manifest.Request.SHA256,
		},
		"response": map[string]interface{}{
			"status": manifest.Response.Status,
			"reason": manifest.Response.Reason,
			"bytes":  manifest.Response.Bytes,
			"sha256": manifest.Response.SHA256,
		},
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
