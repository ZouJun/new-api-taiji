package common

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetArchiveRequestBodyStoragePrefersRawStorage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	rawStorage, err := CreateBodyStorage([]byte("raw-body"))
	require.NoError(t, err)
	defer rawStorage.Close()
	cookedStorage, err := CreateBodyStorage([]byte("cooked-body"))
	require.NoError(t, err)
	defer cookedStorage.Close()

	ctx.Set(KeyRawRequestBodyStorage, rawStorage)
	ctx.Set(KeyBodyStorage, cookedStorage)

	storage, isRaw, err := GetArchiveRequestBodyStorage(ctx)
	require.NoError(t, err)
	require.True(t, isRaw)

	body, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, []byte("raw-body"), body)
}

func TestGetArchiveRequestBodyStorageFallsBackToBodyStorage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	cookedStorage, err := CreateBodyStorage([]byte("cooked-body"))
	require.NoError(t, err)
	defer cookedStorage.Close()

	ctx.Set(KeyBodyStorage, cookedStorage)

	storage, isRaw, err := GetArchiveRequestBodyStorage(ctx)
	require.NoError(t, err)
	require.False(t, isRaw)

	body, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, []byte("cooked-body"), body)
}

func TestCleanupBodyStorageClearsRawAndCookedStorage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	rawStorage, err := CreateBodyStorage([]byte("raw"))
	require.NoError(t, err)
	cookedStorage, err := CreateBodyStorage([]byte("cooked"))
	require.NoError(t, err)

	ctx.Set(KeyRawRequestBodyStorage, rawStorage)
	ctx.Set(KeyBodyStorage, cookedStorage)

	CleanupBodyStorage(ctx)

	rawValue, rawExists := ctx.Get(KeyRawRequestBodyStorage)
	cookedValue, cookedExists := ctx.Get(KeyBodyStorage)
	require.True(t, rawExists)
	require.True(t, cookedExists)
	require.Nil(t, rawValue)
	require.Nil(t, cookedValue)
}
