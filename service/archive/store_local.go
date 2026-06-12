package archive

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
)

type localBackend struct {
	objectsDir string
}

func newLocalBackend(cfg Config) Backend {
	return &localBackend{objectsDir: cfg.ObjectsDir}
}

func (b *localBackend) objectDir(requestID string) string {
	return filepath.Join(b.objectsDir, requestID)
}

func (b *localBackend) WritePlaceholder(manifest Manifest) error {
	manifest.Status = StatusPending
	return b.writeManifest(manifest)
}

func (b *localBackend) WriteFinal(manifest Manifest, objects []backendObject) error {
	dir := b.objectDir(manifest.RequestID)
	if err := ensureDir(dir); err != nil {
		return err
	}
	for _, obj := range objects {
		if obj.LocalPath == "" {
			if obj.AllowMissing {
				continue
			}
			return fmt.Errorf("missing spool path for %s", obj.ObjectType)
		}
		dst := filepath.Join(dir, obj.Name)
		if err := gzipFile(obj.LocalPath, dst); err != nil {
			return err
		}
	}
	return b.writeManifest(manifest)
}

func (b *localBackend) writeManifest(manifest Manifest) error {
	dir := b.objectDir(manifest.RequestID)
	if err := ensureDir(dir); err != nil {
		return err
	}
	data, err := common.Marshal(manifest)
	if err != nil {
		return err
	}
	dst := filepath.Join(dir, "manifest.json.gz")
	tmp := dst + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	gw := gzip.NewWriter(file)
	_, writeErr := gw.Write(data)
	closeErr := gw.Close()
	fileErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if fileErr != nil {
		_ = os.Remove(tmp)
		return fileErr
	}
	return os.Rename(tmp, dst)
}

func gzipFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	gw := gzip.NewWriter(out)
	_, copyErr := io.Copy(gw, in)
	closeErr := gw.Close()
	fileErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if fileErr != nil {
		_ = os.Remove(tmp)
		return fileErr
	}
	return os.Rename(tmp, dst)
}

func writeManifestTemp(dir string, manifest Manifest) (string, error) {
	data, err := common.Marshal(manifest)
	if err != nil {
		return "", err
	}
	path, file, err := createSpoolFile(dir, "manifest")
	if err != nil {
		return "", err
	}
	gw := gzip.NewWriter(file)
	_, writeErr := gw.Write(data)
	closeErr := gw.Close()
	fileErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return "", writeErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", closeErr
	}
	if fileErr != nil {
		_ = os.Remove(path)
		return "", fileErr
	}
	return path, nil
}
