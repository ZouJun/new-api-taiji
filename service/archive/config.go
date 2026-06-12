package archive

import (
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type Config struct {
	Enabled          bool
	Backend          string
	LocalDir         string
	SpoolDir         string
	ObjectsDir       string
	QueueSize        int
	WorkerCount      int
	MaxRequestBytes  int64
	MaxResponseBytes int64
	SpoolTTLHours    int
	AzureAccountURL  string
	AzureContainer   string
	AzureAccountName string
	AzureAccountKey  string
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
		Enabled:          common.ArchiveEnabled,
		Backend:          strings.ToLower(strings.TrimSpace(common.ArchiveBackend)),
		LocalDir:         localDir,
		SpoolDir:         spoolDir,
		ObjectsDir:       filepath.Join(localDir, "objects"),
		QueueSize:        common.ArchiveQueueSize,
		WorkerCount:      common.ArchiveWorkerCount,
		MaxRequestBytes:  common.ArchiveMaxRequestBytes,
		MaxResponseBytes: common.ArchiveMaxResponseBytes,
		SpoolTTLHours:    common.ArchiveSpoolTTLHours,
		AzureAccountURL:  strings.TrimSpace(common.ArchiveAzureAccountURL),
		AzureContainer:   strings.TrimSpace(common.ArchiveAzureContainer),
		AzureAccountName: strings.TrimSpace(common.ArchiveAzureAccountName),
		AzureAccountKey:  strings.TrimSpace(common.ArchiveAzureAccountKey),
	}
}
