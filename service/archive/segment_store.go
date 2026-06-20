package archive

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type segmentStore struct {
	cfg     Config
	backend Backend
	shards  []*segmentShard
}

type segmentShard struct {
	cfg     Config
	backend Backend
	mu      sync.Mutex
	index   int
	active  *activeSegment
}

type activeSegment struct {
	id          string
	createdAt   time.Time
	recordCount int
	dataBytes   int64
	dir         string
	dataPath    string
	indexPath   string
	dataFile    *os.File
	indexFile   *os.File
	indexBuf    *bufio.Writer
	requestIDs  []string
}

func newSegmentStore(cfg Config, backend Backend) (*segmentStore, error) {
	store := &segmentStore{
		cfg:     cfg,
		backend: backend,
		shards:  make([]*segmentShard, cfg.SegmentShardCount),
	}
	for i := 0; i < cfg.SegmentShardCount; i++ {
		store.shards[i] = &segmentShard{cfg: cfg, backend: backend, index: i}
	}
	return store, nil
}

func (s *segmentStore) Append(manifest Manifest) (Manifest, error) {
	shard := s.shards[segmentShardIndex(manifest.RequestID, len(s.shards))]
	return shard.append(manifest)
}

func (s *segmentStore) FlushExpired() {
	for _, shard := range s.shards {
		shard.flushExpired()
	}
}

func segmentShardIndex(requestID string, shardCount int) int {
	if shardCount <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(requestID))
	return int(h.Sum32() % uint32(shardCount))
}

func (s *segmentShard) append(manifest Manifest) (Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureActiveLocked(); err != nil {
		return manifest, err
	}
	if s.shouldSealLocked(manifest) {
		if err := s.sealLocked(); err != nil {
			return manifest, err
		}
		if err := s.ensureActiveLocked(); err != nil {
			return manifest, err
		}
	}

	requestOffset, requestLength, err := appendFileContents(manifest.Request.SpoolPath, s.active.dataFile)
	if err != nil {
		return manifest, err
	}
	responseOffset, responseLength, err := appendFileContents(manifest.Response.SpoolPath, s.active.dataFile)
	if err != nil {
		return manifest, err
	}
	entry := map[string]any{
		"request_id":      manifest.RequestID,
		"created_at":      manifest.CreatedAt,
		"method":          manifest.Method,
		"path":            manifest.Path,
		"status_code":     manifest.StatusCode,
		"is_stream":       manifest.IsStream,
		"request_offset":  requestOffset,
		"request_length":  requestLength,
		"response_offset": responseOffset,
		"response_length": responseLength,
		"request_sha256":  manifest.Request.SHA256,
		"response_sha256": manifest.Response.SHA256,
		"request_header":  manifest.RequestHeader,
		"upstream_usage":  manifest.UpstreamUsage,
		"payload_capture": manifest.PayloadCapture,
		"channel_id":      manifest.ChannelID,
		"channel_type":    manifest.ChannelType,
		"channel_name":    manifest.ChannelName,
		"provider":        manifest.Provider,
		"model":           manifest.Model,
	}
	line, err := common.Marshal(entry)
	if err != nil {
		return manifest, err
	}
	if _, err = s.active.indexBuf.Write(line); err != nil {
		return manifest, err
	}
	if err = s.active.indexBuf.WriteByte('\n'); err != nil {
		return manifest, err
	}
	if err = s.active.indexBuf.Flush(); err != nil {
		return manifest, err
	}
	if err = s.active.dataFile.Sync(); err != nil {
		return manifest, err
	}
	if err = s.active.indexFile.Sync(); err != nil {
		return manifest, err
	}

	manifest.Segment = &SegmentReference{
		SegmentID:      s.active.id,
		SegmentData:    s.active.dataPath,
		SegmentIndex:   s.active.indexPath,
		RequestOffset:  requestOffset,
		RequestLength:  requestLength,
		ResponseOffset: responseOffset,
		ResponseLength: responseLength,
		Shard:          s.index,
	}
	s.active.recordCount++
	s.active.dataBytes += requestLength + responseLength
	s.active.requestIDs = append(s.active.requestIDs, manifest.RequestID)
	manifest.Status = StatusSucceeded
	return manifest, nil
}

func (s *segmentShard) shouldSealLocked(manifest Manifest) bool {
	if s.active == nil {
		return false
	}
	if s.cfg.SegmentMaxRecords > 0 && s.active.recordCount >= s.cfg.SegmentMaxRecords {
		return true
	}
	if s.cfg.SegmentMaxAgeSeconds > 0 && time.Since(s.active.createdAt) >= time.Duration(s.cfg.SegmentMaxAgeSeconds)*time.Second {
		return true
	}
	estimatedBytes := s.active.dataBytes + manifest.Request.Bytes + manifest.Response.Bytes
	return s.cfg.SegmentMaxBytes > 0 && estimatedBytes >= s.cfg.SegmentMaxBytes
}

func (s *segmentShard) ensureActiveLocked() error {
	if s.active != nil {
		return nil
	}
	now := time.Now().UTC()
	segmentID := fmt.Sprintf("%s-shard-%02d", now.Format("20060102T150405.000000000"), s.index)
	dir := filepath.Join(s.cfg.SegmentsDir, fmt.Sprintf("shard-%02d", s.index))
	if err := ensureDir(dir); err != nil {
		return err
	}
	dataPath := filepath.Join(dir, segmentID+".data")
	indexPath := filepath.Join(dir, segmentID+".index.jsonl")
	dataFile, err := os.OpenFile(dataPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	indexFile, err := os.OpenFile(indexPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		_ = dataFile.Close()
		return err
	}
	s.active = &activeSegment{
		id:        segmentID,
		createdAt: now,
		dir:       dir,
		dataPath:  dataPath,
		indexPath: indexPath,
		dataFile:  dataFile,
		indexFile: indexFile,
		indexBuf:  bufio.NewWriter(indexFile),
	}
	return nil
}

func (s *segmentShard) sealLocked() error {
	if s.active == nil {
		return nil
	}
	sealed := s.active
	if err := s.active.indexBuf.Flush(); err != nil {
		return err
	}
	if err := s.active.dataFile.Close(); err != nil {
		return err
	}
	if err := s.active.indexFile.Close(); err != nil {
		return err
	}
	s.active = nil
	if uploader, ok := s.backend.(segmentUploadBackend); ok {
		go uploadSealedSegment(uploader, sealed)
	}
	return nil
}

func (s *segmentShard) flushExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil || s.cfg.SegmentMaxAgeSeconds <= 0 {
		return
	}
	if time.Since(s.active.createdAt) < time.Duration(s.cfg.SegmentMaxAgeSeconds)*time.Second {
		return
	}
	_ = s.sealLocked()
}

func uploadSealedSegment(uploader segmentUploadBackend, sealed *activeSegment) {
	if sealed == nil {
		return
	}
	dataRemote, indexRemote, err := uploader.UploadSegmentFiles(sealed.id, sealed.dataPath, sealed.indexPath)
	archivePatch := map[string]interface{}{
		"segment": map[string]interface{}{
			"segment_id":           sealed.id,
			"segment_data_remote":  dataRemote,
			"segment_index_remote": indexRemote,
		},
		"segment_upload": map[string]interface{}{
			"status": "uploaded",
		},
	}
	if err != nil {
		archivePatch["segment_upload"] = map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		}
	}
	for _, requestID := range sealed.requestIDs {
		_ = model.PatchLogOtherArchiveByRequestID(requestID, archivePatch)
	}
}

func appendFileContents(srcPath string, dst *os.File) (int64, int64, error) {
	in, err := os.Open(srcPath)
	if err != nil {
		return 0, 0, err
	}
	defer in.Close()
	offset, err := dst.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, 0, err
	}
	written, err := io.Copy(dst, in)
	if err != nil {
		return 0, 0, err
	}
	return offset, written, nil
}
