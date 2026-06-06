# Research: Architecture

**Date:** 2026-06-06

## Recommended Components

### Trace Middleware

- Extend or add middleware near `middleware.RequestId`.
- Extract configured header names, defaulting to `Trace-Id`.
- Sanitize value to a safe subset for logs/object names, keep original only if explicitly allowed.
- Put `customer_trace_id` in Gin context and request context.

### Archive Capture Layer

- Request capture should reuse existing request body storage instead of rereading the body.
- Non-stream response capture can wrap or centralize response bytes where handlers already parse response data.
- Streaming response capture should tee chunks as they are written, using bounded buffering and never blocking client flush on object storage.

### Archive Queue

- Relay path emits an archive job with request bytes, response chunks or temp file references, metadata, and checksums.
- Worker pool uploads to local disk or Azure Blob.
- Queue must be bounded; overflow policy should be explicit: drop capture, spill to disk, or fail request only in strict mode.

### Manifest

- Store one manifest JSON per request with request/response object names, sizes, hashes, content type, channel/model, request ID, customer trace ID, timestamps, stream flag, archival result, and errors.
- Store request and response as separate compressed objects to avoid rebuilding large zip archives under high RPM.

## Blob Naming Rules

- Azure Blob names can be up to 1024 characters.
- Blob containers have stricter lowercase and dash rules; use a stable configured container name rather than dynamic per-request containers.
- Object paths should be virtual directories inside one container.

## Sources

- Azure Blob naming rules: https://learn.microsoft.com/en-us/rest/api/storageservices/Naming-and-Referencing-Containers--Blobs--and-Metadata
- Azure Blob upload with Go: https://learn.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-go
