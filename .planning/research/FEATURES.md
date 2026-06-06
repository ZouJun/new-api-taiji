# Research: Features

**Date:** 2026-06-06

## Table Stakes for This Milestone

### AWS Claude Timeout Audit

- Document whether timeout exists.
- Identify all timeout sources: shared `http.Client.Timeout`, AWS SDK context timeout, proxy client behavior.
- Explain behavior for streaming and non-streaming calls.
- Add tests or implementation plan for timeout semantics if gaps are found.

### Customer Trace-Id

- Read customer trace ID from request headers.
- Sanitize and length-limit it before logging or file/object naming.
- Store it separately from server-generated request ID.
- Add it to context, structured logs, `logs.other`, and archive metadata.

### Request/Response Archival

- Capture request body and upstream/client response body for both streaming and non-streaming calls.
- Do not store full payloads in the database.
- Store object names, checksums, byte counts, and archival status in log metadata.
- Provide switchable local and Azure Blob backends.
- Include feature flags and per-route/per-model safety controls.

## Differentiators

- Asynchronous archival with bounded queues and explicit backpressure behavior.
- Per-request manifest that ties request, response, metadata, storage status, and hashes together.
- Beginner-readable design docs and operational runbook.
- Dead-letter storage for failed uploads without impacting successful customer responses.

## Anti-Features

- Synchronous Azure upload in the relay hot path.
- Large request/response payloads in `logs.other`.
- Unbounded in-memory buffering of streams.
- Raw customer `Trace-Id` in blob names.
- Storage failure causing successful upstream calls to be returned as errors by default.
