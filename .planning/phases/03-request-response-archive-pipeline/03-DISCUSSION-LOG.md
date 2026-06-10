# Phase 3: Request Response Archive Pipeline - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-09
**Phase:** 3-request-response-archive-pipeline
**Areas discussed:** coverage scope, archive object structure, streaming capture semantics, failure isolation, archive metadata persistence, size limits, compression, queue and worker sizing, spool strategy, backend selection, local directory layout, spool cleanup, Azure metadata, object naming, configuration scope, runtime logging, manifest lifecycle, capture timing

---

## Coverage Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Main relay + task/video | Cover customer-facing main relay routes plus task/video generation and fetch paths | ✓ |
| Main relay only | Restrict first version to `controller.Relay` main paths | |
| All relay-like paths | Include MJ, realtime, channel-test, and other edge entry points in phase 1 of archival | |

**User's choice:** Main relay + task/video
**Notes:** The first version should handle the practical customer API surface without expanding into every edge route.

---

## Archive Object Structure

| Option | Description | Selected |
|--------|-------------|----------|
| Manifest + request + final response | Store one manifest, one request object, and one final client-visible response object | ✓ |
| Manifest + every attempt payload + final response | Persist full payloads for every retry attempt as well as final response | |
| Single bundle object | Put everything into one bundled archive object | |

**User's choice:** Manifest + request + final response
**Notes:** Retry attempt diagnostics should go into the manifest summary only; failed attempt bodies are out of scope for the first version.

---

## Streaming Capture Semantics

| Option | Description | Selected |
|--------|-------------|----------|
| Final client-visible SSE | Archive the exact SSE/chunk bytes sent to the customer after conversion | ✓ |
| Raw upstream SSE | Archive provider-native stream output only | |
| Both streams | Archive both upstream raw stream and final client-visible stream | |

**User's choice:** Final client-visible SSE
**Notes:** The archive should answer "what did the customer actually receive?" rather than "what did the provider originally emit?"

---

## Failure Isolation

| Option | Description | Selected |
|--------|-------------|----------|
| Non-blocking archive | Archive failures never fail otherwise successful requests | ✓ |
| Configurable fail-closed | Allow archive failure to fail customer requests under configuration | |
| Synchronous strong archive | Customer response waits for archive success | |

**User's choice:** Non-blocking archive
**Notes:** Archive failures, skips, and queue overflow must record explicit reasons and print runtime logs.

---

## Archive Metadata Persistence

| Option | Description | Selected |
|--------|-------------|----------|
| `logs.other.archive` + runtime log | Persist structured archive status in logs and print runtime failures/skips | ✓ |
| Runtime log only | Use logs without DB metadata | |
| DB metadata only | Use `logs.other` without runtime signal | |

**User's choice:** `logs.other.archive` + runtime log
**Notes:** The archive status object should be structured and query-friendly inside `logs.other`.

---

## Size Limits

| Option | Description | Selected |
|--------|-------------|----------|
| Skip oversized object | Do not truncate; skip the request/response object if it exceeds the limit | ✓ |
| Truncate oversized object | Keep only the first segment of a large payload | |
| Compress-first decision | Decide by compressed size instead of raw size | |

**User's choice:** Skip oversized object
**Notes:** Normal payloads may exceed 60MB. The locked defaults became `128MB` for both request and response objects, with explicit logs on skip.

---

## Compression

| Option | Description | Selected |
|--------|-------------|----------|
| Default gzip | Compress archive objects by default | ✓ |
| No compression | Store raw objects | |
| Configurable with gzip default | Support both modes but default to gzip | |

**User's choice:** Default gzip
**Notes:** Compression ratio and raw/gzip byte sizes should both be recorded.

---

## Queue and Worker Sizing

| Option | Description | Selected |
|--------|-------------|----------|
| Bounded non-blocking queue | Use a bounded queue and skip when full | ✓ |
| Short wait on enqueue | Allow a brief enqueue delay before skipping | |
| Unbounded queue | Never skip due to queue capacity | |

**User's choice:** Bounded non-blocking queue
**Notes:** The user asked to size for 8000-15000 RPM and accepted a larger default. Locked values: `queue_size = 50000`, `worker_count = 32`. Queue entries must stay lightweight.

---

## Spool Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Local spool first | Write request/response to local temporary files and enqueue only metadata + file paths | ✓ |
| Hybrid memory/disk | Small objects in memory, large objects on disk | |
| Memory-first queue | Hand payload buffers directly to workers | |

**User's choice:** Local spool first
**Notes:** The user explicitly accepts a small amount of archive loss to keep the main service stable.

---

## Backend Selection

| Option | Description | Selected |
|--------|-------------|----------|
| Single backend mode | One request writes to either local or Azure Blob based on config | ✓ |
| Local + Azure dual write | Always write local then upload Azure | |
| Primary + fallback | Try one backend and fall back to another | |

**User's choice:** Single backend mode
**Notes:** The first version should keep state and semantics simple.

---

## Local Directory Layout

| Option | Description | Selected |
|--------|-------------|----------|
| Separate `spool/` and `objects/` | Keep temporary files separate from finalized archive objects | ✓ |
| Single mixed tree | Store temporary and final objects under one tree | |
| Custom layout | Use a custom directory structure | |

**User's choice:** Separate `spool/` and `objects/`
**Notes:** This choice supports safe cleanup and easier operational inspection.

---

## Spool Cleanup

| Option | Description | Selected |
|--------|-------------|----------|
| Delete on success, retain failed spool briefly | Immediate cleanup on success; TTL cleanup for failures | ✓ |
| Delete immediately always | Remove spool files regardless of outcome | |
| Retain everything | Keep both successful and failed spool files | |

**User's choice:** Delete on success, retain failed spool briefly
**Notes:** Locked failed-spool retention TTL: `24 hours`.

---

## Azure Metadata

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal metadata set | Write request/trace/object basics to blob metadata | ✓ |
| No metadata | Rely on path and blob content only | |
| Expanded business metadata | Write more fields such as channel/model/status into blob metadata | |

**User's choice:** Minimal metadata set
**Notes:** The chosen set includes `request_id`, `trace_id`, trace presence, object type, stream flag, backend mode, encoding, content type, and created time.

---

## Naming and Content-Type Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Request-ID keyed path, unified data object names | Use `request_id` as the primary path key and keep payload type in metadata | ✓ |
| Path includes `request_id` and `trace_id` | Use both values in path naming | |
| Custom format | Use a bespoke naming pattern | |

**User's choice:** Request-ID keyed path, unified data object names
**Notes:** Standard names are `manifest.json.gz`, `request.data.gz`, and `response.data.gz`. `trace_id` remains in manifest and metadata.

---

## Configuration Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Global-only controls | Archive enablement and sizing are controlled globally | ✓ |
| Global + route controls | Add route-level switches | |
| Global + channel controls | Add channel-level switches | |

**User's choice:** Global-only controls
**Notes:** Per-route and per-channel controls are deferred to future phases.

---

## Runtime Logging

| Option | Description | Selected |
|--------|-------------|----------|
| Log skipped/failed only | Runtime logs emit only on archive skip/failure cases | ✓ |
| Log full lifecycle | Emit queued, success, skipped, and failed logs | |
| Log failures only | Emit only archive failures | |

**User's choice:** Log skipped/failed only
**Notes:** This limits log volume while preserving the important operational signals.

---

## Manifest Lifecycle and Capture Timing

| Option | Description | Selected |
|--------|-------------|----------|
| Placeholder manifest first | Write an initial manifest entry and fill it in after work completes | ✓ |
| Final manifest only | Write manifest once at the end only | |

**User's choice:** Placeholder manifest first
**Notes:** The user also delegated capture timing to best practice. The locked interpretation is: capture request bytes as soon as reusable body data is available, and capture response bytes at the final client-visible write path using tee-style interception that preserves stream order and flush behavior.

---

## the agent's Discretion

- Exact config key names and package boundaries may follow local conventions.
- The narrowest safe response tee points may be selected during planning, as long as archived bytes remain client-visible final output.

## Deferred Ideas

- Dashboard archive search by trace ID
- Offline archive compaction
- Tenant-specific retention policy
- Per-channel or per-route archive toggles
- Dual-write or fallback backend behavior

