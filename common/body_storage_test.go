package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateBodyStorageFromReaderUsesDiskForUnknownLengthWhenDiskCacheEnabled(t *testing.T) {
	t.Parallel()

	originalConfig := GetDiskCacheConfig()
	defer SetDiskCacheConfig(originalConfig)

	SetDiskCacheConfig(DiskCacheConfig{
		Enabled:     true,
		ThresholdMB: 1,
		MaxSizeMB:   32,
		Path:        t.TempDir(),
	})
	ResetDiskCacheUsage()

	storage, err := CreateBodyStorageFromReader(strings.NewReader("unknown-length-body"), -1, 1<<20)
	require.NoError(t, err)
	defer storage.Close()
	require.True(t, storage.IsDisk())
}
