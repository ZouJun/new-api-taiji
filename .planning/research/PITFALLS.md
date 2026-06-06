# Research: Pitfalls

**Date:** 2026-06-06

## High-RPM Archival Pitfalls

### Blocking the Relay Hot Path

- Warning sign: p95/p99 latency rises when Azure is slow.
- Prevention: enqueue archive jobs and return customer responses independently.
- Phase impact: archival design and implementation must define timeout and queue overflow behavior before coding.

### Unbounded Memory Use

- Warning sign: memory grows with concurrent streams or large responses.
- Prevention: size limits, temp-file spill, bounded chunk buffers, and per-request max capture bytes.

### Breaking Streaming Semantics

- Warning sign: SSE chunks arrive late, out of order, or only at the end.
- Prevention: tee chunks after read/before write without waiting on storage; test exact client-visible stream.

### Unsafe Trace IDs in File Names

- Warning sign: customer-provided header contains path separators, Unicode confusables, very long values, or control characters.
- Prevention: sanitize to `[A-Za-z0-9._-]`, cap length, and use a fallback like `no-trace`.

### Blob Batching Misapplied

- Warning sign: implementation waits to pack many requests into one zip before upload.
- Prevention: avoid cross-request zip batching for the hot path; upload per-request objects and optionally compact offline later.

### Database Bloat

- Warning sign: `logs.other` stores payloads or large nested data.
- Prevention: store only metadata and object references in DB logs.
