package archive

import (
	"bytes"
	"os"

	"github.com/gin-gonic/gin"
)

type TeeWriter struct {
	gin.ResponseWriter
	spoolDir    string
	file        *os.File
	path        string
	inlineLimit int64
	payload     bytes.Buffer
	maxBytes    int64
	written     int64
	disabled    bool
	skipReason  string
	contentType string
}

func NewTeeWriter(base gin.ResponseWriter, spoolDir string, maxBytes int64, inlineLimit int64) *TeeWriter {
	tw := &TeeWriter{
		ResponseWriter: base,
		spoolDir:       spoolDir,
		maxBytes:       maxBytes,
		inlineLimit:    inlineLimit,
	}
	return tw
}

func (w *TeeWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.capture(data[:n])
	}
	return n, err
}

func (w *TeeWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	if n > 0 {
		w.capture([]byte(s[:n]))
	}
	return n, err
}

func (w *TeeWriter) capture(data []byte) {
	if w.disabled {
		return
	}
	if w.contentType == "" {
		w.contentType = w.Header().Get("Content-Type")
	}
	next := w.written + int64(len(data))
	if next > w.maxBytes {
		w.disabled = true
		w.skipReason = ReasonResponseSizeLimit
		w.closeAndRemove()
		return
	}
	if w.file == nil && w.inlineLimit > 0 && next <= w.inlineLimit {
		_, _ = w.payload.Write(data)
		w.written = next
		return
	}
	if w.file == nil {
		path, file, err := createSpoolFile(w.spoolDir, ObjectResponse)
		if err != nil {
			w.disabled = true
			w.skipReason = ReasonResponseSpoolFailed
			return
		}
		w.file = file
		w.path = path
		if w.payload.Len() > 0 {
			if _, err = w.file.Write(w.payload.Bytes()); err != nil {
				w.disabled = true
				w.skipReason = ReasonResponseSpoolFailed
				w.closeAndRemove()
				return
			}
			w.payload.Reset()
		}
	}
	if _, err := w.file.Write(data); err != nil {
		w.disabled = true
		w.skipReason = ReasonResponseSpoolFailed
		w.closeAndRemove()
		return
	}
	w.written = next
}

func (w *TeeWriter) Finish() ObjectInfo {
	if w.contentType == "" {
		w.contentType = w.Header().Get("Content-Type")
	}
	if w.disabled {
		reason := w.skipReason
		status := StatusSkipped
		if reason == ReasonResponseSpoolFailed {
			status = StatusFailed
		}
		return ObjectInfo{Status: status, Reason: reason, Bytes: w.written, ContentType: w.contentType, Stage: "client_response", Fallback: true}
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			w.closeAndRemove()
			return ObjectInfo{Status: StatusFailed, Reason: ReasonResponseSpoolFailed, Bytes: w.written, ContentType: w.contentType}
		}
	}
	return ObjectInfo{
		Status:      StatusPending,
		Bytes:       w.written,
		ContentType: w.contentType,
		Stage:       "client_response",
		Fallback:    true,
		Payload:     cloneBufferedPayload(&w.payload),
		SpoolPath:   w.path,
	}
}

func (w *TeeWriter) closeAndRemove() {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.path != "" {
		_ = os.Remove(w.path)
	}
}
