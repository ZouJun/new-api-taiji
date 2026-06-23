package archive

import (
	"strings"
	"sync"
	"time"
)

var (
	reloadMu    sync.Mutex
	reloadTimer *time.Timer
)

var managedOptionKeys = map[string]struct{}{
	"ArchiveEnabled":                  {},
	"ArchiveBackend":                  {},
	"ArchiveLocalDir":                 {},
	"ArchiveSpoolDir":                 {},
	"ArchiveQueueSize":                {},
	"ArchiveWorkerCount":              {},
	"ArchiveMaxRequestMB":             {},
	"ArchiveMaxResponseMB":            {},
	"ArchiveSpoolTTLHours":            {},
	"ArchiveSmallPayloadMaxKB":        {},
	"ArchiveSegmentMaxMB":             {},
	"ArchiveSegmentMaxAgeSeconds":     {},
	"ArchiveSegmentMaxRecords":        {},
	"ArchiveSegmentShardCount":        {},
	"ArchiveHeaderValueMaxLength":     {},
	"ArchiveSamplePercent":            {},
	"ArchiveSkipOnHighLoad":           {},
	"ArchiveMaxCPUPercent":            {},
	"ArchiveMaxMemoryPercent":         {},
	"ArchiveMinFreeDiskPercent":       {},
	"ArchiveMinFreeDiskGB":            {},
	"ArchiveLoadCheckIntervalSeconds": {},
	"ArchiveAzureAccountURL":          {},
	"ArchiveAzureContainer":           {},
	"ArchiveAzureAccountName":         {},
	"ArchiveAzureAccountKey":          {},
}

func IsManagedOptionKey(key string) bool {
	_, ok := managedOptionKeys[strings.TrimSpace(key)]
	return ok
}

func HandleManagedOptionChange(key string, changed bool) {
	if !changed || !IsManagedOptionKey(key) {
		return
	}
	reloadMu.Lock()
	defer reloadMu.Unlock()
	if reloadTimer != nil {
		reloadTimer.Stop()
	}
	reloadTimer = time.AfterFunc(5*time.Second, func() {
		Init()
	})
}
