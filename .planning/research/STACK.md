# Research: Stack

**Date:** 2026-06-06

## Current Stack Fit

- Backend is Go with Gin, GORM, Redis, and provider adaptors. This is suitable for high-throughput relay changes because request middleware, context propagation, and response writers are already centralized.
- AWS Bedrock support already uses AWS SDK for Go v2 in `relay/channel/aws/`, so timeout and stream behavior can be audited in place.
- Existing logs table already has `request_id`, `upstream_request_id`, and `other`, making trace metadata additive rather than a large schema redesign.

## Azure Blob SDK Direction

- Use the current Azure SDK module `github.com/Azure/azure-sdk-for-go/sdk/storage/azblob`, not the older `github.com/Azure/azure-storage-blob-go/azblob`.
- Official Microsoft Learn quickstart and Azure SDK docs currently point Go users at `azblob` from `azure-sdk-for-go`.
- Authentication options should support connection string, account key/SAS where operationally required, and managed identity or workload identity where deployed in Azure.

## Storage Implementation Shape

- Define a small internal storage interface such as `ArchiveStore.Put(ctx, object ArchiveObject) (ArchiveResult, error)`.
- Implement `local` and `azure_blob` stores behind the same interface.
- Keep compression, object naming, metadata, retries, and queueing in a separate archive service so relay code does not know Azure-specific details.

## Sources

- Azure Blob Storage Go quickstart: https://learn.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-go
- Azure SDK for Go data plane docs: https://learn.microsoft.com/en-us/azure/developer/go/data-plane
- Azure SDK Go package: https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/storage/azblob
