package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/gin-gonic/gin"
)

type upstreamBodyCapture struct {
	reader      io.ReadCloser
	state       *State
	file        *os.File
	path        string
	maxBytes    int64
	contentType string
	written     int64
	hasher      hashWriter
	disabled    bool
	reason      string
}

func WrapUpstreamResponse(c *gin.Context, reader io.ReadCloser, contentType string, spoolDir string, maxBytes int64) io.ReadCloser {
	if reader == nil {
		return nil
	}
	state, ok := FromContext(c)
	if !ok || state == nil {
		return reader
	}
	path, file, err := createSpoolFile(spoolDir, "upstream-response")
	if err != nil {
		state.SetUpstreamResponse(ObjectInfo{
			Status:      StatusFailed,
			Reason:      ReasonResponseSpoolFailed,
			ContentType: contentType,
			Stage:       "upstream_raw",
		}, contentType)
		return reader
	}
	return &upstreamBodyCapture{
		reader:      reader,
		state:       state,
		file:        file,
		path:        path,
		maxBytes:    maxBytes,
		contentType: contentType,
		hasher:      sha256.New(),
	}
}

func (u *upstreamBodyCapture) Read(p []byte) (int, error) {
	n, err := u.reader.Read(p)
	if n > 0 {
		u.capture(p[:n])
	}
	return n, err
}

func (u *upstreamBodyCapture) Close() error {
	readErr := u.reader.Close()
	info := ObjectInfo{
		Status:      StatusPending,
		Bytes:       u.written,
		ContentType: u.contentType,
		Stage:       "upstream_raw",
		SpoolPath:   u.path,
	}
	if u.disabled {
		info.Status = StatusSkipped
		info.Reason = u.reason
		info.Fallback = true
		info.SpoolPath = ""
	} else {
		if u.file != nil {
			if err := u.file.Close(); err != nil {
				info.Status = StatusFailed
				info.Reason = ReasonResponseSpoolFailed
				info.Fallback = true
				info.SpoolPath = ""
			} else {
				info.SHA256 = hex.EncodeToString(u.hasher.Sum(nil))
			}
			u.file = nil
		}
	}
	u.state.SetUpstreamResponse(info, u.contentType)
	return readErr
}

func (u *upstreamBodyCapture) capture(data []byte) {
	if u.disabled || u.file == nil {
		return
	}
	next := u.written + int64(len(data))
	if next > u.maxBytes {
		u.disabled = true
		u.reason = ReasonResponseSizeLimit
		_ = u.file.Close()
		_ = os.Remove(u.path)
		u.file = nil
		return
	}
	if _, err := u.file.Write(data); err != nil {
		u.disabled = true
		u.reason = ReasonResponseSpoolFailed
		_ = u.file.Close()
		_ = os.Remove(u.path)
		u.file = nil
		return
	}
	_, _ = u.hasher.Write(data)
	u.written = next
}
