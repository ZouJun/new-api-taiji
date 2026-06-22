package archive

import (
	"bytes"
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
	return captureObject(dir, objectType, src, limit, 0)
}

func cloneBufferedPayload(buf *bytes.Buffer) []byte {
	if buf == nil {
		return nil
	}
	payload := make([]byte, buf.Len())
	copy(payload, buf.Bytes())
	return payload
}

func captureObject(dir string, objectType string, src io.Reader, limit int64, inlineLimit int64) (ObjectInfo, error) {
	if inlineLimit > 0 && inlineLimit > limit {
		inlineLimit = limit
	}
	var (
		written   int64
		buffered  bytes.Buffer
		useInline = inlineLimit > 0
		path      string
		file      *os.File
		err       error
	)
	closeAndRemove := func() {
		if file != nil {
			_ = file.Close()
			file = nil
		}
		if path != "" {
			_ = os.Remove(path)
			path = ""
		}
	}
	ensureSpoolFile := func() error {
		if file != nil {
			return nil
		}
		var err error
		path, file, err = createSpoolFile(dir, objectType)
		if err != nil {
			return err
		}
		return nil
	}
	limited := io.LimitReader(src, limit+1)
	chunk := make([]byte, 32<<10)
	for {
		n, readErr := limited.Read(chunk)
		if n > 0 {
			nextWritten := written + int64(n)
			if nextWritten > limit {
				closeAndRemove()
				reason := ReasonRequestSizeLimit
				if objectType == ObjectResponse {
					reason = ReasonResponseSizeLimit
				}
				return ObjectInfo{Status: StatusSkipped, Reason: reason, Bytes: nextWritten}, nil
			}
			if useInline && nextWritten <= inlineLimit {
				if _, err = buffered.Write(chunk[:n]); err != nil {
					closeAndRemove()
					return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, err
				}
			} else {
				if useInline {
					useInline = false
					if err = ensureSpoolFile(); err != nil {
						return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, err
					}
					if _, err = file.Write(buffered.Bytes()); err != nil {
						closeAndRemove()
						return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, err
					}
					buffered.Reset()
				}
				if err = ensureSpoolFile(); err != nil {
					return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, err
				}
				if _, err = file.Write(chunk[:n]); err != nil {
					closeAndRemove()
					return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, err
				}
			}
			written = nextWritten
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			closeAndRemove()
			return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, readErr
		}
	}

	if useInline {
		return ObjectInfo{
			Status:  StatusPending,
			Bytes:   written,
			Payload: cloneBufferedPayload(&buffered),
		}, nil
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed}, err
	}
	return ObjectInfo{
		Status:    StatusPending,
		Bytes:     written,
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
