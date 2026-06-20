# Phase 3 Context: Request Response Archive Pipeline

**Phase:** 3  
**Updated:** 2026-06-20

## User-Confirmed Scope

The storage phase must cover more than raw request/response files. It must also store the customer-provided correlation headers and upstream-native usage metadata needed for operations and billing reconciliation.

## Header Whitelist

Capture only:

- `Routify-Provider-Trace-Id`
- `X-Trace-Id`
- `Routify-Provider-Request-Id`
- `X-Request-ID`
- `X-Client-Request-Id`
- `X-Request-Id`
- `Routify-Provider-Color-Id`
- `x-conversation-id`
- `X-Session-Id`

Rules:

- Record all keys whether or not a request supplied a value.
- Store missing values as empty strings.
- Truncate oversized values for logs and database metadata.
- Do not reject requests due to these header values.
- Persist under `logs.other.request_header`.

## Usage Reconciliation

`logs.other.upstream_usage` is additive metadata for reconciliation. It must not replace existing billing fields.

For non-streaming responses, use upstream-native usage if present.

For streaming responses, capture usage when provider chunks expose it.

For interrupted streams:

- if native usage was already received, store it and mark the stream as interrupted;
- if native usage was not received, store estimated usage when safe and mark it as estimated and incomplete.

## Raw Payload Fidelity

Stability is the priority for storage strategy, but payload fidelity is a hard requirement when capture is possible.

Request payload storage must prefer original downstream request body bytes as received from the customer before gateway parsing, provider conversion, or retry replay mutation.

Response payload storage must prefer original upstream response body bytes as received from the provider before gateway conversion, normalization, or client response adaptation.

Fallback capture is allowed only when a specific relay path cannot expose the original bytes. Fallbacks must be explicit in metadata so operators can tell whether an object is true upstream/downstream raw data or a later-stage representation.

Required metadata:

- `payload_capture.request_stage`
- `payload_capture.response_stage`
- `payload_capture.request_fallback`
- `payload_capture.response_fallback`

Preferred stages:

- request: `downstream_raw`
- response: `upstream_raw`

Fallback examples:

- request: `replay_body`, `converted_request`
- response: `client_response`, `converted_response`

## Runtime Storage Protection

Archive storage can be enabled but still skipped when runtime pressure is too high.

Configurable defaults:

- CPU threshold: 80%
- Memory threshold: 80%
- Minimum free disk percentage: 15%
- Minimum free disk bytes: 10GB
- Runtime check interval: 5 seconds

Threshold skips must be visible in runtime logs and `logs.other.archive`.

## Current Implementation Reference

The split storage branch currently contains a first archive implementation under:

- `middleware/archive.go`
- `service/archive/`
- `model/log.go`
- `router/relay-router.go`
- `router/video-router.go`

The existing implementation already has async queueing, local/Azure backends, request/response spool files, manifests, and archive metadata backfill. The new scope adds header metadata, upstream usage metadata, and runtime threshold skip gates.

## Final Routing Summary

Confirmed storage routing:

- small payloads use `segmented`
- large payloads use `per_request`
- local segmented mode uses continuous append to shard-owned segment files
- Azure segmented mode uses the same local segment writer first, then uploads sealed segments asynchronously
- local per-request mode stores `manifest`, `request`, and `response` as separate objects
- Azure per-request mode spools locally first, then uploads `manifest`, `request`, and `response` as separate objects

Association rules:

- `logs.request_id` is the primary database correlation key
- `logs.other.archive` stores the exact object path or segment offset mapping needed to retrieve payloads
- segmented index files must also carry `request_id` and offsets so the storage layer has a non-database recovery path
