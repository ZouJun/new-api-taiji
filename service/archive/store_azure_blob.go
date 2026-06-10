package archive

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/QuantumNous/new-api/common"
)

type azureBlobBackend struct {
	client    *azblob.Client
	container string
	tmpDir    string
}

func newAzureBlobBackend(cfg Config) Backend {
	serviceURL := cfg.AzureAccountURL
	if serviceURL == "" && cfg.AzureAccountName != "" {
		serviceURL = fmt.Sprintf("https://%s.blob.core.windows.net/", cfg.AzureAccountName)
	}
	if serviceURL == "" || cfg.AzureContainer == "" {
		return &failingBackend{reason: "missing azure blob archive configuration"}
	}

	var (
		client *azblob.Client
		err    error
	)
	if cfg.AzureAccountName != "" && cfg.AzureAccountKey != "" {
		cred, credErr := azblob.NewSharedKeyCredential(cfg.AzureAccountName, cfg.AzureAccountKey)
		if credErr != nil {
			return &failingBackend{reason: credErr.Error()}
		}
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
	} else {
		client, err = azblob.NewClientWithNoCredential(serviceURL, nil)
	}
	if err != nil {
		return &failingBackend{reason: err.Error()}
	}
	return &azureBlobBackend{client: client, container: cfg.AzureContainer, tmpDir: cfg.SpoolDir}
}

func (b *azureBlobBackend) WritePlaceholder(manifest Manifest) error {
	manifest.Status = StatusPending
	tmpPath, err := writeManifestTemp(b.tmpDir, manifest)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	return b.uploadFile(manifest, "manifest.json.gz", tmpPath, ObjectManifest, "application/json", map[string]string{})
}

func (b *azureBlobBackend) WriteFinal(manifest Manifest, objects []backendObject) error {
	for _, obj := range objects {
		if obj.LocalPath == "" {
			if obj.AllowMissing {
				continue
			}
			return fmt.Errorf("missing spool path for %s", obj.ObjectType)
		}
		tmpPath := obj.LocalPath + ".gz"
		if err := gzipFile(obj.LocalPath, tmpPath); err != nil {
			return err
		}
		err := b.uploadFile(manifest, obj.Name, tmpPath, obj.ObjectType, obj.ContentType, obj.Metadata)
		_ = os.Remove(tmpPath)
		if err != nil {
			return err
		}
	}
	tmpPath, err := writeManifestTemp(b.tmpDir, manifest)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	return b.uploadFile(manifest, "manifest.json.gz", tmpPath, ObjectManifest, "application/json", metadataFor(manifest, ObjectManifest, ObjectInfo{ContentType: "application/json"}))
}

func (b *azureBlobBackend) uploadFile(manifest Manifest, name string, localPath string, objectType string, contentType string, metadata map[string]string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	if metadata == nil {
		metadata = map[string]string{}
	}
	if _, ok := metadata["object_type"]; !ok {
		for k, v := range metadataFor(manifest, objectType, ObjectInfo{ContentType: contentType}) {
			metadata[k] = v
		}
	}
	blobName := path.Join(manifest.RequestID, name)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_, err = b.client.UploadFile(ctx, b.container, blobName, file, &azblob.UploadFileOptions{
		HTTPHeaders: &blob.HTTPHeaders{
			BlobContentType:     &contentType,
			BlobContentEncoding: common.GetPointer("gzip"),
		},
		Metadata: sanitizeAzureMetadata(metadata),
	})
	return err
}

type failingBackend struct {
	reason string
}

func (b *failingBackend) WritePlaceholder(Manifest) error {
	return fmt.Errorf("%s", b.reason)
}

func (b *failingBackend) WriteFinal(Manifest, []backendObject) error {
	return fmt.Errorf("%s", b.reason)
}

func sanitizeAzureMetadata(metadata map[string]string) map[string]string {
	clean := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.ReplaceAll(key, "-", "_")
		if key == "" {
			continue
		}
		clean[key] = value
	}
	return clean
}
