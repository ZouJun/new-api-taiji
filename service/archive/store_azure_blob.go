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
)

type azureBlobBackend struct {
	client    *azblob.Client
	container string
	tmpDir    string
}

type segmentUploadBackend interface {
	UploadSegmentFiles(segmentID string, dataPath string, indexPath string) (string, string, error)
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

func (b *azureBlobBackend) WriteFinal(manifest Manifest, objects []backendObject) error {
	for _, obj := range objects {
		if obj.LocalPath == "" && !obj.HasInlineData {
			if obj.AllowMissing {
				continue
			}
			return fmt.Errorf("missing spool path for %s", obj.ObjectType)
		}
		if err := b.uploadObject(manifest, obj.Name, obj.ObjectType, obj.ContentType, obj.ContentEncoding, obj.Metadata, obj.LocalPath, obj.Data, obj.HasInlineData); err != nil {
			return err
		}
	}
	tmpPath, err := writeManifestTemp(b.tmpDir, manifest)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	return b.uploadObject(manifest, "manifest.json.gz", ObjectManifest, "application/json", "gzip", metadataFor(manifest, ObjectManifest, ObjectInfo{ContentType: "application/json", ContentEncoding: "gzip"}), tmpPath, nil, false)
}

func (b *azureBlobBackend) uploadObject(manifest Manifest, name string, objectType string, contentType string, contentEncoding string, metadata map[string]string, localPath string, data []byte, hasInlineData bool) error {
	if metadata == nil {
		metadata = map[string]string{}
	}
	if _, ok := metadata["object_type"]; !ok {
		for k, v := range metadataFor(manifest, objectType, ObjectInfo{ContentType: contentType, ContentEncoding: contentEncoding}) {
			metadata[k] = v
		}
	}
	blobName := path.Join(manifest.RequestID, name)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	headers := &blob.HTTPHeaders{
		BlobContentType: &contentType,
	}
	if contentEncoding != "" {
		headers.BlobContentEncoding = &contentEncoding
	}
	options := &azblob.UploadFileOptions{
		HTTPHeaders: headers,
		Metadata:    sanitizeAzureMetadata(metadata),
	}
	if hasInlineData {
		_, err := b.client.UploadBuffer(ctx, b.container, blobName, data, &azblob.UploadBufferOptions{
			HTTPHeaders: headers,
			Metadata:    sanitizeAzureMetadata(metadata),
		})
		return err
	}
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = b.client.UploadFile(ctx, b.container, blobName, file, options)
	return err
}

func (b *azureBlobBackend) UploadSegmentFiles(segmentID string, dataPath string, indexPath string) (string, string, error) {
	dataBlobName := path.Join("segments", segmentID+".data")
	indexBlobName := path.Join("segments", segmentID+".index.jsonl")
	if err := b.uploadStandaloneFile(dataBlobName, dataPath, "application/octet-stream", map[string]string{
		"segment_id":  segmentID,
		"object_type": "segment_data",
	}); err != nil {
		return "", "", err
	}
	if err := b.uploadStandaloneFile(indexBlobName, indexPath, "application/jsonl", map[string]string{
		"segment_id":  segmentID,
		"object_type": "segment_index",
	}); err != nil {
		return "", "", err
	}
	return dataBlobName, indexBlobName, nil
}

func (b *azureBlobBackend) uploadStandaloneFile(blobName string, localPath string, contentType string, metadata map[string]string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_, err = b.client.UploadFile(ctx, b.container, blobName, file, &azblob.UploadFileOptions{
		HTTPHeaders: &blob.HTTPHeaders{
			BlobContentType: &contentType,
		},
		Metadata: sanitizeAzureMetadata(metadata),
	})
	return err
}

type failingBackend struct {
	reason string
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
