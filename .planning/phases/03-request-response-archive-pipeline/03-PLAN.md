# Phase 3 Plan: Request Response Archive Pipeline

**Phase:** 3  
**Status:** Planned  
**Updated:** 2026-06-20

## Goal

Relay requests and upstream responses can be archived to local storage or Azure Blob without coupling customer latency to storage health. The storage phase must also preserve the confirmed customer correlation headers and upstream-native usage evidence needed for troubleshooting and reconciliation.

## Confirmed Decisions

- Only the following request headers are captured in this phase:
  - `Routify-Provider-Trace-Id`
  - `X-Trace-Id`
  - `Routify-Provider-Request-Id`
  - `X-Request-ID`
  - `X-Client-Request-Id`
  - `X-Request-Id`
  - `Routify-Provider-Color-Id`
  - `x-conversation-id`
  - `X-Session-Id`
- All listed header keys are recorded even when no value is present.
- Oversized header values are truncated for storage/logging; requests are not rejected because of these header lengths.
- Header snapshots are stored in `logs.other.request_header` for both consume logs and error logs.
- Error output logs print these header values at the same correlation level as server request ID.
- Consume logs store upstream-native usage metadata in `logs.other.upstream_usage`.
- Streaming interruptions may store estimated usage when native upstream usage is missing, but the record must be marked as estimated and incomplete.
- Raw request and upstream response storage remains behind an archive feature switch.
- Stability is the priority for high-RPM archival. Small payloads use segmented storage for stability and object-count control; large payloads use per-request storage.
- Archived request payloads must prefer the original downstream request body bytes as received by the gateway before parsing or conversion.
- Archived response payloads must prefer the original upstream response body bytes as received from the provider before gateway conversion or client adaptation.
- Fallback capture is allowed only when original downstream request bytes or upstream response bytes are unavailable on a specific relay path, and metadata must explicitly record the fallback capture stage.
- Archive storage is skipped when configurable CPU, memory, or disk thresholds are exceeded, even if archive storage is enabled.
- Default runtime safety thresholds are CPU 80%, memory 80%, minimum free disk 15%, minimum free disk bytes 10GB, and check interval 5 seconds. All are configurable.

## Metadata Shapes

Request header snapshot:

```json
{
  "request_header": {
    "Routify-Provider-Trace-Id": "",
    "X-Trace-Id": "",
    "Routify-Provider-Request-Id": "",
    "X-Request-ID": "",
    "X-Client-Request-Id": "",
    "X-Request-Id": "",
    "Routify-Provider-Color-Id": "",
    "x-conversation-id": "",
    "X-Session-Id": ""
  }
}
```

Upstream usage snapshot:

```json
{
  "upstream_usage": {
    "raw": {},
    "source": "upstream",
    "complete": true,
    "stream_interrupted": false,
    "provider": "",
    "model": ""
  }
}
```

Allowed `upstream_usage.source` values:

- `upstream`: provider returned native usage and it was captured.
- `estimated`: no native usage was available, so gateway-estimated usage was stored.
- `mixed`: partial provider usage was available and the gateway filled missing fields.
- `missing`: usage was unavailable and no estimate was safe to produce.

Archive skip metadata must preserve the reason, for example:

```json
{
  "archive": {
    "status": "skipped",
    "reason": "cpu_threshold_exceeded",
    "request_id": "",
    "request": {},
    "response": {}
  }
}
```

Payload capture metadata must preserve source fidelity:

```json
{
  "payload_capture": {
    "request_stage": "downstream_raw",
    "response_stage": "upstream_raw",
    "request_fallback": false,
    "response_fallback": false
  }
}
```

Allowed request stages:

- `downstream_raw`: original request body bytes received from the customer.
- `replay_body`: fallback bytes captured from the gateway replay body when raw downstream bytes are unavailable.
- `converted_request`: fallback bytes captured after gateway/provider request conversion.

Allowed response stages:

- `upstream_raw`: original response body bytes received from the upstream provider.
- `client_response`: fallback bytes captured from the final response written to the client.
- `converted_response`: fallback bytes captured after gateway response conversion.

## Final Storage Scheme

Storage strategy defaults:

```text
ARCHIVE_STORAGE_STRATEGY=auto
ARCHIVE_SMALL_PAYLOAD_MAX_BYTES=64KB
ARCHIVE_SEGMENT_MAX_BYTES=256MB
ARCHIVE_SEGMENT_MAX_AGE_SECONDS=60
ARCHIVE_SEGMENT_MAX_RECORDS=50000
```

Routing rule:

- small payload: request bytes + response bytes + meta bytes `<= 64KB` -> `segmented`
- large payload: request bytes + response bytes + meta bytes `> 64KB` -> `per_request`

### Local Storage: Small Payload

Small payloads use `segmented` storage for stability.

Write path:

- append continuously to active local segment files
- assign each request to a shard by `request_id hash % shard_count`
- one writer goroutine per shard
- rotate when any threshold is met: `256MB`, `60s`, `50000 records`

Objects:

- `segments/{date}/{segment_id}.data.gz`
- `segments/{date}/{segment_id}.index.json.gz`

Data contract:

- `data.gz` stores the original request bytes and original response bytes as raw payload records
- `index.json.gz` stores `request_id`, offsets, lengths, checksums, and `meta`

### Local Storage: Large Payload

Large payloads use `per_request` storage.

Write path:

- capture request and response to local spool files
- finish request
- async worker compresses and moves to final object location

Objects:

- `objects/{request_id}/manifest.json.gz`
- `objects/{request_id}/request.data.gz`
- `objects/{request_id}/response.data.gz`

### Azure Storage: Small Payload

Small payloads still write locally first, then upload sealed segments asynchronously.

Write path:

- write to local active segment files exactly as local segmented mode
- seal the segment locally
- upload sealed `data.gz` and `index.json.gz` to Azure Blob asynchronously

Objects:

- `segments/{date}/{segment_id}.data.gz`
- `segments/{date}/{segment_id}.index.json.gz`

### Azure Storage: Large Payload

Large payloads use local spool plus async per-request upload.

Write path:

- capture request and response to local spool files
- finish request
- async worker compresses and uploads per-request objects to Azure Blob

Objects:

- `objects/{request_id}/manifest.json.gz`
- `objects/{request_id}/request.data.gz`
- `objects/{request_id}/response.data.gz`

### Request Association

Every archive object set is keyed by the server `request_id`.

`logs.other.archive` is the primary lookup path from database log row to stored payload location.

Per-request example:

```json
{
  "archive": {
    "strategy": "per_request",
    "backend": "azure_blob",
    "status": "succeeded",
    "request_id": "req-123",
    "manifest": "objects/req-123/manifest.json.gz",
    "request": "objects/req-123/request.data.gz",
    "response": "objects/req-123/response.data.gz"
  }
}
```

Segmented example:

```json
{
  "archive": {
    "strategy": "segmented",
    "backend": "local",
    "status": "succeeded",
    "request_id": "req-123",
    "segment_data": "segments/20260620/seg-001.data.gz",
    "segment_index": "segments/20260620/seg-001.index.json.gz",
    "request_offset": 100,
    "request_length": 3000,
    "response_offset": 3100,
    "response_length": 5000
  }
}
```

## Meta Field Contract

The archive `meta` object must contain:

- `request_id`
- `method`
- `path`
- `status_code`
- `is_stream`
- `provider`
- `channel_id`
- `channel_type`
- `channel_name`
- `model`
- `request_header`
- `request_header_truncated`
- `upstream_usage`
- `payload_capture`
- `request`
- `response`
- `attempts`
- `archive`

The `request` object must contain:

- `bytes`
- `sha256`
- `content_type`
- `content_encoding`
- `stage`

The `response` object must contain:

- `bytes`
- `sha256`
- `content_type`
- `content_encoding`
- `stage`

The `archive` object must contain:

- `strategy`
- `backend`
- `status`
- `reason`
- `storage_mode`

The `payload_capture` object must contain:

- `request_stage`
- `response_stage`
- `request_fallback`
- `response_fallback`

## Plans

### 03-01: Storage Metadata Design

Design archive interfaces, object naming, manifests, metadata, and configuration for:

- local storage
- Azure Blob storage
- raw request and upstream response payloads
- payload capture stage metadata and fallback semantics
- whitelisted request-header snapshots
- upstream-native usage snapshots
- stream interruption fallback usage
- threshold-based archive skipping

### 03-02: Request Header Capture

Implement a shared helper that extracts the confirmed header whitelist from `gin.Context`.

Acceptance criteria:

- Both consume logs and error logs write `logs.other.request_header`.
- Every whitelisted key is present even when missing from the request.
- Oversized values are truncated and do not reject the request.
- Error logs print the values at the same correlation level as server request ID.
- Tests cover present, missing, mixed-case, and oversized header values.

### 03-03: Upstream Usage Capture

Capture upstream-native usage data for reconciliation.

Acceptance criteria:

- Non-streaming responses preserve provider-native usage when available.
- Streaming responses preserve provider-native usage from stream chunks when available.
- Stream interruption paths preserve already received usage when present.
- Stream interruption paths store estimated usage when native usage is missing and mark `source=estimated`, `complete=false`, and `stream_interrupted=true`.
- Existing billing token fields remain unchanged; `upstream_usage` is additive metadata.

### 03-04: Raw Payload Storage

Implement raw downstream request and raw upstream response archival.

Acceptance criteria:

- Archive storage remains controlled by configuration.
- Request capture prefers original downstream request body bytes before parsing/conversion.
- Response capture prefers original upstream response body bytes before gateway conversion or client adaptation.
- If a relay path cannot expose original bytes, fallback capture is allowed only with explicit `payload_capture` stage metadata.
- Request capture does not corrupt retry or replay behavior.
- Non-streaming upstream response capture does not alter client-visible output.
- Streaming upstream response capture preserves chunk order and flush behavior.
- Local and Azure Blob backends share the same manifest contract.
- Archive object metadata is stored in `logs.other.archive`, not full payloads.

### 03-05: Runtime Safety Thresholds

Add configurable threshold checks before archive storage work is accepted.

Default values:

- CPU: `80%`
- Memory: `80%`
- Minimum free disk: `15%`
- Minimum free disk bytes: `10GB`
- Check interval: `5s`

Acceptance criteria:

- Thresholds are configurable.
- If any enabled threshold is exceeded, archive storage is skipped.
- Skips do not fail otherwise successful customer requests.
- Skip reason is recorded in archive metadata and runtime logs.
- Disk checks apply to local spool usage even when final backend is Azure Blob.
