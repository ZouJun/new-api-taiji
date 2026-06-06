# Research Summary

**Date:** 2026-06-06

## Key Findings

**Stack:** The existing Go/Gin relay architecture already has the right central points for timeout audit, trace propagation, and archival: `middleware/request-id.go`, `controller/relay.go`, `service/http_client.go`, `model/log.go`, and provider handlers under `relay/channel/`.

**Azure Blob:** Use `github.com/Azure/azure-sdk-for-go/sdk/storage/azblob`. Keep a configured container and use virtual path blob names. Blob names support up to 1024 characters, so object naming must be sanitized and bounded.

**Architecture:** Implement archival as an isolated service with a local/Azure backend interface, bounded queue, worker pool, per-request manifest, and request/response objects. Do not upload synchronously from the hot path by default.

**Traceability:** Preserve the distinction between server `request_id` and customer `Trace-Id`. Store customer trace ID in Gin/request context, structured logs, archive object names, manifest metadata, and `logs.other`.

**Pitfalls:** The main risks are blocking relay responses on storage, unbounded stream buffering, unsafe customer trace IDs in object names, and database bloat.

## Recommended Object Strategy

- Store one manifest per request.
- Store request and response as separate compressed objects.
- Use per-request object names rather than zip batching many requests together.
- Optional offline compaction can be a later feature, not part of the hot path.

## Sources

- Azure Blob Storage Go quickstart: https://learn.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-go
- Azure SDK for Go data plane docs: https://learn.microsoft.com/en-us/azure/developer/go/data-plane
- Azure Blob naming rules: https://learn.microsoft.com/en-us/rest/api/storageservices/Naming-and-Referencing-Containers--Blobs--and-Metadata
