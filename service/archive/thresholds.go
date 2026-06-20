package archive

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type loadSnapshot struct {
	checkedAt               time.Time
	shouldSkip              bool
	reason                  string
	detail                  string
	cpuThresholdExceeded    bool
	memoryThresholdExceeded bool
	diskThresholdExceeded   bool
}

type LoadSnapshot struct {
	ShouldSkip              bool
	Reason                  string
	Detail                  string
	CPUThresholdExceeded    bool
	MemoryThresholdExceeded bool
	DiskThresholdExceeded   bool
}

var (
	loadSnapshotMu sync.Mutex
	lastLoadCheck  loadSnapshot
)

func evaluateLoad(cfg Config) loadSnapshot {
	if !cfg.SkipOnHighLoad {
		return loadSnapshot{}
	}

	loadSnapshotMu.Lock()
	defer loadSnapshotMu.Unlock()

	interval := time.Duration(cfg.LoadCheckIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if !lastLoadCheck.checkedAt.IsZero() && time.Since(lastLoadCheck.checkedAt) < interval {
		return lastLoadCheck
	}

	status := common.GetSystemStatus()
	disk := common.GetDiskSpaceInfoForPath(cfg.LocalDir)
	snapshot := loadSnapshot{checkedAt: time.Now()}

	if cfg.MaxCPUPercent > 0 && status.CPUUsage >= float64(cfg.MaxCPUPercent) {
		snapshot.shouldSkip = true
		snapshot.reason = "high_load"
		snapshot.detail = fmt.Sprintf("cpu usage %.2f%% >= %d%%", status.CPUUsage, cfg.MaxCPUPercent)
		snapshot.cpuThresholdExceeded = true
	}
	if cfg.MaxMemoryPercent > 0 && status.MemoryUsage >= float64(cfg.MaxMemoryPercent) {
		snapshot.shouldSkip = true
		snapshot.reason = "high_load"
		snapshot.detail = appendThresholdDetail(snapshot.detail, fmt.Sprintf("memory usage %.2f%% >= %d%%", status.MemoryUsage, cfg.MaxMemoryPercent))
		snapshot.memoryThresholdExceeded = true
	}
	if disk.Total > 0 {
		freePercent := 100 - disk.UsedPercent
		if cfg.MinFreeDiskPercent > 0 && freePercent <= float64(cfg.MinFreeDiskPercent) {
			snapshot.shouldSkip = true
			snapshot.reason = "high_load"
			snapshot.detail = appendThresholdDetail(snapshot.detail, fmt.Sprintf("disk free %.2f%% <= %d%%", freePercent, cfg.MinFreeDiskPercent))
			snapshot.diskThresholdExceeded = true
		}
		if cfg.MinFreeDiskBytes > 0 && int64(disk.Free) <= cfg.MinFreeDiskBytes {
			snapshot.shouldSkip = true
			snapshot.reason = "high_load"
			snapshot.detail = appendThresholdDetail(snapshot.detail, fmt.Sprintf("disk free bytes %d <= %d", disk.Free, cfg.MinFreeDiskBytes))
			snapshot.diskThresholdExceeded = true
		}
	}

	lastLoadCheck = snapshot
	return snapshot
}

func appendThresholdDetail(current string, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}

func EvaluateLoad(m *Manager) LoadSnapshot {
	if m == nil {
		return LoadSnapshot{}
	}
	snapshot := evaluateLoad(m.cfg)
	return LoadSnapshot{
		ShouldSkip:              snapshot.shouldSkip,
		Reason:                  snapshot.reason,
		Detail:                  snapshot.detail,
		CPUThresholdExceeded:    snapshot.cpuThresholdExceeded,
		MemoryThresholdExceeded: snapshot.memoryThresholdExceeded,
		DiskThresholdExceeded:   snapshot.diskThresholdExceeded,
	}
}
