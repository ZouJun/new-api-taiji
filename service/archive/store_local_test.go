package archive

import (
	"compress/gzip"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLocalBackendWritesObjects(t *testing.T) {
	dir := t.TempDir()
	spoolDir := filepath.Join(dir, "spool")
	objectsDir := filepath.Join(dir, "objects")
	if err := os.MkdirAll(spoolDir, 0755); err != nil {
		t.Fatal(err)
	}
	reqPath := filepath.Join(spoolDir, "request.tmp")
	respPath := filepath.Join(spoolDir, "response.tmp")
	if err := os.WriteFile(reqPath, []byte("request-body"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(respPath, []byte("response-body"), 0600); err != nil {
		t.Fatal(err)
	}

	backend := newLocalBackend(Config{ObjectsDir: objectsDir})
	manifest := Manifest{
		RequestID:   "req-local",
		Backend:     "local",
		StorageMode: "relay",
		Status:      StatusSucceeded,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Request:     ObjectInfo{Status: StatusPending, SpoolPath: reqPath, ContentType: "application/json"},
		Response:    ObjectInfo{Status: StatusPending, SpoolPath: respPath, ContentType: "application/json"},
	}

	err := backend.WriteFinal(manifest, []backendObject{
		{Name: "request.data", ObjectType: ObjectRequest, LocalPath: reqPath, ContentType: "application/json"},
		{Name: "response.data", ObjectType: ObjectResponse, LocalPath: respPath, ContentType: "application/json"},
	})
	require.NoError(t, err)

	assertText(t, filepath.Join(objectsDir, "req-local", "request.data"), "request-body")
	assertText(t, filepath.Join(objectsDir, "req-local", "response.data"), "response-body")
	manifestText := readGzipText(t, filepath.Join(objectsDir, "req-local", "manifest.json.gz"))
	if !strings.Contains(manifestText, `"request_id":"req-local"`) {
		t.Fatalf("manifest missing request_id: %s", manifestText)
	}
}

func TestLocalBackendWritesInlinePayloadObjects(t *testing.T) {
	dir := t.TempDir()
	objectsDir := filepath.Join(dir, "objects")

	backend := newLocalBackend(Config{ObjectsDir: objectsDir})
	manifest := Manifest{
		RequestID:   "req-inline",
		Backend:     "local",
		StorageMode: "relay",
		Status:      StatusSucceeded,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Request:     ObjectInfo{Status: StatusPending, Bytes: 12, Payload: []byte("request-body"), ContentType: "application/json"},
		Response:    ObjectInfo{Status: StatusPending, Bytes: 13, Payload: []byte("response-body"), ContentType: "application/json"},
	}

	err := backend.WriteFinal(manifest, []backendObject{
		{Name: "request.data", ObjectType: ObjectRequest, Data: []byte("request-body"), HasInlineData: true, ContentType: "application/json"},
		{Name: "response.data", ObjectType: ObjectResponse, Data: []byte("response-body"), HasInlineData: true, ContentType: "application/json"},
	})
	require.NoError(t, err)

	assertText(t, filepath.Join(objectsDir, "req-inline", "request.data"), "request-body")
	assertText(t, filepath.Join(objectsDir, "req-inline", "response.data"), "response-body")
}

func TestTeeWriterSkipsOversizeWithoutTruncatingObject(t *testing.T) {
	dir := t.TempDir()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	writer := NewTeeWriter(ctx.Writer, dir, 4, 2)
	ctx.Writer = writer

	if _, err := ctx.Writer.Write([]byte("123")); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.Writer.Write([]byte("45")); err != nil {
		t.Fatal(err)
	}
	info := writer.Finish()

	if rec.Body.String() != "12345" {
		t.Fatalf("client response corrupted: %q", rec.Body.String())
	}
	if info.Status != StatusSkipped || info.Reason != ReasonResponseSizeLimit {
		t.Fatalf("expected size skip, got status=%s reason=%s", info.Status, info.Reason)
	}
	if info.SpoolPath != "" {
		t.Fatalf("oversize object should not keep a spool path")
	}
}

func TestTeeWriterCapturesStreamingChunksInOrder(t *testing.T) {
	dir := t.TempDir()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	writer := NewTeeWriter(ctx.Writer, dir, 128, 4)
	ctx.Writer = writer

	chunks := []string{"data: one\n\n", "data: two\n\n", "data: [DONE]\n\n"}
	for _, chunk := range chunks {
		if _, err := ctx.Writer.WriteString(chunk); err != nil {
			t.Fatal(err)
		}
		ctx.Writer.Flush()
	}
	info := writer.Finish()

	expected := strings.Join(chunks, "")
	if rec.Body.String() != expected {
		t.Fatalf("client stream corrupted: %q", rec.Body.String())
	}
	if info.Status != StatusPending {
		t.Fatalf("expected captured stream pending, got %s", info.Status)
	}
	data, err := os.ReadFile(info.SpoolPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("spool stream order mismatch: %q", string(data))
	}
}

func TestCaptureRequestSkipsOversizeWithoutSpoolPath(t *testing.T) {
	dir := t.TempDir()
	manager := &Manager{
		cfg: Config{
			SpoolDir:        dir,
			MaxRequestBytes: 4,
		},
	}

	info, err := manager.CaptureRequest(strings.NewReader("12345"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != StatusSkipped || info.Reason != ReasonRequestSizeLimit {
		t.Fatalf("expected request size skip, got status=%s reason=%s", info.Status, info.Reason)
	}
	if info.SpoolPath != "" {
		t.Fatalf("oversize request should not keep a spool path")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("oversize request left spool files: %d", len(entries))
	}
}

func TestCaptureRequestStoresSmallPayloadInline(t *testing.T) {
	dir := t.TempDir()
	manager := &Manager{
		cfg: Config{
			SpoolDir:             dir,
			MaxRequestBytes:      128,
			SmallPayloadMaxBytes: 64,
		},
	}

	info, err := manager.CaptureRequest(strings.NewReader("small-body"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != StatusPending {
		t.Fatalf("expected pending, got %s", info.Status)
	}
	if info.SpoolPath != "" {
		t.Fatalf("small request should not create spool path")
	}
	if string(info.Payload) != "small-body" {
		t.Fatalf("unexpected inline payload: %q", string(info.Payload))
	}
}

func TestTeeWriterKeepsSmallPayloadInline(t *testing.T) {
	dir := t.TempDir()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	writer := NewTeeWriter(ctx.Writer, dir, 128, 64)
	ctx.Writer = writer

	_, err := ctx.Writer.Write([]byte("tiny-response"))
	require.NoError(t, err)

	info := writer.Finish()
	require.Equal(t, StatusPending, info.Status)
	require.Empty(t, info.SpoolPath)
	require.Equal(t, []byte("tiny-response"), info.Payload)
}

func TestAppendObjectContentsSupportsInlinePayload(t *testing.T) {
	dir := t.TempDir()
	dstPath := filepath.Join(dir, "segment.data")
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	require.NoError(t, err)
	defer dst.Close()

	offset, written, err := appendObjectContents(ObjectInfo{
		Status:  StatusPending,
		Bytes:   7,
		Payload: []byte("inline1"),
	}, dst)
	require.NoError(t, err)
	require.EqualValues(t, 0, offset)
	require.EqualValues(t, 7, written)

	offset, written, err = appendObjectContents(ObjectInfo{
		Status:  StatusPending,
		Bytes:   0,
		Payload: []byte{},
	}, dst)
	require.NoError(t, err)
	require.EqualValues(t, 7, offset)
	require.EqualValues(t, 0, written)

	data, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	require.Equal(t, "inline1", string(data))
}

func TestTryEnqueueMarksQueueFull(t *testing.T) {
	manager := &Manager{queue: make(chan Job, 1)}
	first := Manifest{RequestID: "req-1", Status: StatusSucceeded}
	second := Manifest{RequestID: "req-2", Status: StatusSucceeded}

	queued, updated := manager.tryEnqueue(first)
	if !queued {
		t.Fatalf("expected first job queued")
	}
	if updated.Status != StatusSucceeded {
		t.Fatalf("queued manifest should be unchanged, got %s", updated.Status)
	}

	queued, updated = manager.tryEnqueue(second)
	if queued {
		t.Fatalf("expected full queue")
	}
	if updated.Status != StatusSkipped || updated.Reason != ReasonQueueFull {
		t.Fatalf("expected queue full skip, got status=%s reason=%s", updated.Status, updated.Reason)
	}
}

func TestSummarizeStatusPrioritizesFailuresAndSkips(t *testing.T) {
	status, reason := summarizeStatus(
		ObjectInfo{Status: StatusPending},
		ObjectInfo{Status: StatusSkipped, Reason: ReasonResponseSizeLimit},
	)
	if status != StatusPartial || reason != ReasonResponseSizeLimit {
		t.Fatalf("expected partial response skip, got status=%s reason=%s", status, reason)
	}

	status, reason = summarizeStatus(
		ObjectInfo{Status: StatusFailed, Reason: ReasonRequestSpoolFailed},
		ObjectInfo{Status: StatusSkipped, Reason: ReasonResponseSizeLimit},
	)
	if status != StatusFailed || reason != ReasonRequestSpoolFailed {
		t.Fatalf("expected request failure priority, got status=%s reason=%s", status, reason)
	}
}

func assertGzipText(t *testing.T, path string, expected string) {
	t.Helper()
	actual := readGzipText(t, path)
	if actual != expected {
		t.Fatalf("unexpected gzip text for %s: %q", path, actual)
	}
}

func assertText(t *testing.T, path string, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("unexpected text for %s: %q", path, string(data))
	}
}

func readGzipText(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, gr); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
