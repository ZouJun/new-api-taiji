package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/gin-gonic/gin"
)

type TeeWriter struct {
	gin.ResponseWriter
	state       *State
	file        *os.File
	path        string
	maxBytes    int64
	written     int64
	hasher      hashWriter
	disabled    bool
	skipReason  string
	spoolErr    error
	contentType string
}

type hashWriter interface {
	Write([]byte) (int, error)
	Sum([]byte) []byte
}

func NewTeeWriter(base gin.ResponseWriter, state *State, spoolDir string, maxBytes int64) *TeeWriter {
	path, file, err := createSpoolFile(spoolDir, ObjectResponse)
	tw := &TeeWriter{
		ResponseWriter: base,
		state:          state,
		maxBytes:       maxBytes,
		hasher:         sha256.New(),
	}
	if err != nil {
		tw.disabled = true
		tw.spoolErr = err
		tw.skipReason = ReasonResponseSpoolFailed
		return tw
	}
	tw.file = file
	tw.path = path
	state.runtime.responseFilePath = path
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
	if _, err := w.file.Write(data); err != nil {
		w.disabled = true
		w.spoolErr = err
		w.skipReason = ReasonResponseSpoolFailed
		w.closeAndRemove()
		return
	}
	_, _ = w.hasher.Write(data)
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
		SHA256:      hex.EncodeToString(w.hasher.Sum(nil)),
		Stage:       "client_response",
		Fallback:    true,
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
