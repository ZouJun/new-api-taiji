package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/archive"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDecompressRequestMiddlewarePreservesRawCompressedBodyForArchive(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	originalArchiveEnabled := common.ArchiveEnabled
	originalArchiveBackend := common.ArchiveBackend
	originalArchiveLocalDir := common.ArchiveLocalDir
	originalArchiveSpoolDir := common.ArchiveSpoolDir
	originalArchiveSamplePercent := common.ArchiveSamplePercent
	common.ArchiveEnabled = true
	common.ArchiveBackend = "local"
	common.ArchiveLocalDir = t.TempDir()
	common.ArchiveSpoolDir = ""
	common.ArchiveSamplePercent = 100
	archive.Init()
	defer func() {
		common.ArchiveEnabled = originalArchiveEnabled
		common.ArchiveBackend = originalArchiveBackend
		common.ArchiveLocalDir = originalArchiveLocalDir
		common.ArchiveSpoolDir = originalArchiveSpoolDir
		common.ArchiveSamplePercent = originalArchiveSamplePercent
		archive.Init()
	}()

	plainBody := []byte(`{"message":"hello"}`)
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write(plainBody)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	router := gin.New()
	router.Use(DecompressRequestMiddleware())
	router.POST("/archive-test", func(c *gin.Context) {
		decompressed, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, plainBody, decompressed)

		rawStorage, isRaw, err := common.GetArchiveRequestBodyStorage(c)
		require.NoError(t, err)
		require.True(t, isRaw)
		rawBody, err := rawStorage.Bytes()
		require.NoError(t, err)
		require.Equal(t, compressed.Bytes(), rawBody)
		require.Equal(t, "gzip", c.GetString(common.KeyRawRequestContentEncoding))

		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/archive-test", bytes.NewReader(compressed.Bytes()))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(compressed.Len())
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestDecompressRequestMiddlewareSkipsRawCompressedBodyWhenArchiveDisabled(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	originalArchiveEnabled := common.ArchiveEnabled
	common.ArchiveEnabled = false
	archive.Init()
	defer func() {
		common.ArchiveEnabled = originalArchiveEnabled
		archive.Init()
	}()

	plainBody := []byte(`{"message":"hello"}`)
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write(plainBody)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	router := gin.New()
	router.Use(DecompressRequestMiddleware())
	router.POST("/archive-test", func(c *gin.Context) {
		decompressed, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, plainBody, decompressed)

		_, exists := c.Get(common.KeyRawRequestBodyStorage)
		require.False(t, exists)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/archive-test", bytes.NewReader(compressed.Bytes()))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(compressed.Len())
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}
