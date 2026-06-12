package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

func createSpoolFile(dir string, objectType string) (string, *os.File, error) {
	if err := ensureDir(dir); err != nil {
		return "", nil, err
	}
	name := fmt.Sprintf("%s-%s-%d.tmp", objectType, uuid.New().String()[:8], time.Now().UnixNano())
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0600)
	if err != nil {
		return "", nil, err
	}
	return path, file, nil
}

func copyToSpool(dir string, objectType string, src io.Reader, limit int64) (ObjectInfo, error) {
	path, file, err := createSpoolFile(dir, objectType)
	if err != nil {
		return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, err
	}
	defer file.Close()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(src, limit+1))
	if err != nil {
		_ = os.Remove(path)
		return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, err
	}
	if written > limit {
		_ = os.Remove(path)
		reason := ReasonRequestSizeLimit
		if objectType == ObjectResponse {
			reason = ReasonResponseSizeLimit
		}
		return ObjectInfo{Status: StatusSkipped, Reason: reason, Bytes: written}, nil
	}

	return ObjectInfo{
		Status:    StatusPending,
		Bytes:     written,
		SHA256:    hex.EncodeToString(hasher.Sum(nil)),
		SpoolPath: path,
	}, nil
}

func cleanupExpiredSpoolFiles(dir string, ttl time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > ttl {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}
