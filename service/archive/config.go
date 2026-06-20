package archive

import (
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type Config struct {
	Enabled                  bool
	Backend                  string
	LocalDir                 string
	SpoolDir                 string
	ObjectsDir               string
	SegmentsDir              string
	QueueSize                int
	WorkerCount              int
	MaxRequestBytes          int64
	MaxResponseBytes         int64
	SpoolTTLHours            int
	SmallPayloadMaxBytes     int64
	SegmentMaxBytes          int64
	SegmentMaxAgeSeconds     int
	SegmentMaxRecords        int
	SegmentShardCount        int
	HeaderValueMaxLength     int
	SkipOnHighLoad           bool
	MaxCPUPercent            int
	MaxMemoryPercent         int
	MinFreeDiskPercent       int
	MinFreeDiskBytes         int64
	LoadCheckIntervalSeconds int
	AzureAccountURL          string
	AzureContainer           string
	AzureAccountName         string
	AzureAccountKey          string
}

func loadConfig() Config {
	localDir := common.ArchiveLocalDir
	if localDir == "" {
		localDir = "./data/archive"
	}
	spoolDir := common.ArchiveSpoolDir
	if spoolDir == "" {
		spoolDir = filepath.Join(localDir, "spool")
	}
	return Config{
		Enabled:                  common.ArchiveEnabled,
		Backend:                  strings.ToLower(strings.TrimSpace(common.ArchiveBackend)),
		LocalDir:                 localDir,
		SpoolDir:                 spoolDir,
		ObjectsDir:               filepath.Join(localDir, "objects"),
		SegmentsDir:              filepath.Join(localDir, "segments"),
		QueueSize:                common.ArchiveQueueSize,
		WorkerCount:              common.ArchiveWorkerCount,
		MaxRequestBytes:          common.ArchiveMaxRequestBytes,
		MaxResponseBytes:         common.ArchiveMaxResponseBytes,
		SpoolTTLHours:            common.ArchiveSpoolTTLHours,
		SmallPayloadMaxBytes:     common.ArchiveSmallPayloadMaxBytes,
		SegmentMaxBytes:          common.ArchiveSegmentMaxBytes,
		SegmentMaxAgeSeconds:     common.ArchiveSegmentMaxAgeSeconds,
		SegmentMaxRecords:        common.ArchiveSegmentMaxRecords,
		SegmentShardCount:        common.ArchiveSegmentShardCount,
		HeaderValueMaxLength:     common.ArchiveHeaderValueMaxLength,
		SkipOnHighLoad:           common.ArchiveSkipOnHighLoad,
		MaxCPUPercent:            common.ArchiveMaxCPUPercent,
		MaxMemoryPercent:         common.ArchiveMaxMemoryPercent,
		MinFreeDiskPercent:       common.ArchiveMinFreeDiskPercent,
		MinFreeDiskBytes:         common.ArchiveMinFreeDiskBytes,
		LoadCheckIntervalSeconds: common.ArchiveLoadCheckIntervalSeconds,
		AzureAccountURL:          strings.TrimSpace(common.ArchiveAzureAccountURL),
		AzureContainer:           strings.TrimSpace(common.ArchiveAzureContainer),
		AzureAccountName:         strings.TrimSpace(common.ArchiveAzureAccountName),
		AzureAccountKey:          strings.TrimSpace(common.ArchiveAzureAccountKey),
	}
}
