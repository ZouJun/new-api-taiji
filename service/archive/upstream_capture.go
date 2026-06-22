package archive

import (
	"bytes"
	"io"
	"os"

	"github.com/gin-gonic/gin"
)

type upstreamBodyCapture struct {
	reader      io.ReadCloser
	state       *State
	spoolDir    string
	file        *os.File
	path        string
	inlineLimit int64
	payload     bytes.Buffer
	maxBytes    int64
	contentType string
	written     int64
	disabled    bool
	reason      string
}

func WrapUpstreamResponse(c *gin.Context, reader io.ReadCloser, contentType string, spoolDir string, maxBytes int64, inlineLimit int64) io.ReadCloser {
	if reader == nil {
		return nil
	}
	state, ok := FromContext(c)
	if !ok || state == nil {
		return reader
	}
	return &upstreamBodyCapture{
		reader:      reader,
		state:       state,
		spoolDir:    spoolDir,
		maxBytes:    maxBytes,
		inlineLimit: inlineLimit,
		contentType: contentType,
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
		Payload:     cloneBufferedPayload(&u.payload),
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
				_ = os.Remove(u.path)
			}
			u.file = nil
		}
	}
	u.state.SetUpstreamResponse(info, u.contentType)
	return readErr
}

func (u *upstreamBodyCapture) capture(data []byte) {
	if u.disabled {
		return
	}
	next := u.written + int64(len(data))
	if next > u.maxBytes {
		u.disabled = true
		u.reason = ReasonResponseSizeLimit
		if u.file != nil {
			_ = u.file.Close()
			_ = os.Remove(u.path)
			u.file = nil
		}
		u.payload.Reset()
		return
	}
	if u.file == nil && u.inlineLimit > 0 && next <= u.inlineLimit {
		_, _ = u.payload.Write(data)
		u.written = next
		return
	}
	if u.file == nil {
		path, file, err := createSpoolFile(u.spoolDir, "upstream-response")
		if err != nil {
			u.disabled = true
			u.reason = ReasonResponseSpoolFailed
			return
		}
		u.file = file
		u.path = path
		if u.payload.Len() > 0 {
			if _, err = u.file.Write(u.payload.Bytes()); err != nil {
				u.disabled = true
				u.reason = ReasonResponseSpoolFailed
				_ = u.file.Close()
				_ = os.Remove(u.path)
				u.file = nil
				return
			}
			u.payload.Reset()
		}
	}
	if _, err := u.file.Write(data); err != nil {
		u.disabled = true
		u.reason = ReasonResponseSpoolFailed
		_ = u.file.Close()
		_ = os.Remove(u.path)
		u.file = nil
		return
	}
	u.written = next
}
