# Phase 3 Plan: Request Response Archive Pipeline

**Phase:** 3  
**Status:** Drafted for execution  
**Updated:** 2026-06-09

## 1. Goal

Implement bounded request/response archival for the main relay path plus task/video routes, with local or Azure Blob backend selection, request/response object capture, manifest finalization, queue/worker isolation, and structured archive metadata that does not destabilize relay retries, stream behavior, or customer-visible success paths.

This phase must reuse the Phase 2 `trace_id` semantic and keep full payloads out of database storage.

## 2. Locked Decisions

These points are already confirmed and should not be reopened during implementation unless the user changes direction:

1. First version covers the main relay path plus task/video routes.
2. `mj`, realtime, and channel-test are not required in the first version.
3. Each customer request archives exactly three logical objects: `manifest`, `request`, `final response`.
4. Failed retry attempts do not persist full body objects.
5. Manifest records retry attempt summaries.
6. Streaming archival stores the final SSE/chunk bytes actually sent to the customer.
7. Archival is non-blocking by default; archive failures must not fail otherwise successful customer requests.
8. Archive skip/failure reasons must be recorded in both `logs.other.archive` and runtime logs.
9. Runtime logs emit archive `skipped` and `failed` events only.
10. Request and response object size limits default to `128MB` each.
11. Oversized objects are skipped, not truncated.
12. Archive objects are gzip-compressed by default.
13. Queue is bounded and non-blocking with `queue_size = 50000` and `worker_count = 32`.
14. Queue items must contain only lightweight metadata and spool file paths, never full payload bytes.
15. Request/response payloads must first land in local spool files before enqueue.
16. Successful spool files are deleted immediately; failed spool files retain for `24 hours` before TTL cleanup.
17. Backend selection is single-backend only: `local` or `azure_blob`.
18. Local storage uses separate `spool/` and `objects/` trees.
19. Archive object paths are keyed by `request_id`; `trace_id` lives in manifest and metadata, not the path primary key.
20. Standard object names are:
    - `manifest.json.gz`
    - `request.data.gz`
    - `response.data.gz`
21. `logs.other` stores one structured `archive` object.
22. First version exposes only global archive controls, not per-channel or per-route toggles.
23. Manifest should be written as a placeholder first and then finalized later.
24. Request capture should happen as soon as reusable body storage is available.
25. Response capture should follow best practice and archive the final client-visible bytes at the write path.

## 3. Implementation Model

## 3.1 Effective lifecycle

The target end-to-end lifecycle is:

1. eligible route receives request
2. archive middleware initializes request-scoped archive state and wraps the response writer
3. request body is captured from reusable `BodyStorage`
4. placeholder manifest state is created
5. downstream relay/task/video logic executes normally
6. final client-visible response bytes are tee-captured
7. request completion enqueues a lightweight archive job containing spool paths and metadata
8. worker compresses and writes finalized objects to the configured backend
9. worker finalizes manifest and updates archive status metadata
10. consume/error logs persist structured `logs.other.archive`

## 3.2 Capture semantics

Required capture behavior:

- request archive is based on reusable request body bytes, not a second raw body read path
- non-stream response archive is the final client-visible response body after provider conversion
- stream response archive is the final client-visible SSE/chunk output after provider conversion
- task/video JSON responses and proxied binary content both use the same final-writer capture principle

## 3.3 Failure semantics

Required failure behavior:

- archive queue full -> request continues, status becomes `skipped_queue_full`
- object exceeds size limit -> request continues, that object is skipped with `skipped_size_limit`
- spool write failure -> request continues, archive status records the failure
- backend write/upload failure -> request continues, archive status records the failure
- runtime logs print `request_id`, `trace_id`, backend, status, and reason for skipped/failed events

## 4. Architecture Strategy

## 4.1 Middleware + writer wrapper first

Primary control points:

- new archive middleware on eligible route groups
- new archive-aware response writer wrapper

Reason:

- minimizes provider-specific churn
- captures final client-visible output across `c.JSON(...)`, `c.Data(...)`, `c.Writer.Write(...)`, and helper-based stream output
- keeps route eligibility centralized

## 4.2 Reuse body-storage infrastructure

Primary reuse points:

- `common.GetBodyStorage(...)`
- `common.UnmarshalBodyReusable(...)`
- `middleware.BodyStorageCleanup()`

Reason:

- request body reuse and large-body disk spill already exist
- retry behavior already depends on this path
- duplicating body buffering would increase memory and correctness risk

## 4.3 Local-first backend interface

Primary implementation order:

1. define archive manager + store interface
2. implement spool + local backend
3. wire metadata/logging
4. add Azure backend behind the same interface

Reason:

- repository currently has no Azure Blob SDK dependency
- local backend is the shortest path to stabilize naming, manifest, queue, and log semantics

## 5. Code Design

## 5.1 Archive configuration and state

Main files:

- `common/` for env-backed archive config
- optionally a focused `setting/` helper if needed for typed access
- new archive service package

Required changes:

1. add global archive config:
   - `ARCHIVE_ENABLED`
   - `ARCHIVE_BACKEND`
   - `ARCHIVE_LOCAL_DIR`
   - `ARCHIVE_SPOOL_DIR`
   - `ARCHIVE_QUEUE_SIZE`
   - `ARCHIVE_WORKER_COUNT`
   - `ARCHIVE_MAX_REQUEST_BYTES`
   - `ARCHIVE_MAX_RESPONSE_BYTES`
   - `ARCHIVE_SPOOL_TTL_HOURS`
   - Azure backend credentials/settings
2. define request-scoped archive state struct containing:
   - request identity (`request_id`, `trace_id`)
   - route/mode metadata
   - request spool path + bytes/hash/content type
   - response spool path + bytes/hash/content type
   - status/reason
   - attempt summaries
3. define lightweight queue item struct that contains metadata + spool paths only

Implementation rule:

- queue items must not contain raw request/response `[]byte`

## 5.2 Spool management

Main files:

- new archive spool helper in `service/archive/`
- possibly reuse pieces of `common/disk_cache.go`

Required changes:

1. create archive spool directories separate from generic body/file cache paths
2. write request and response spool files with safe temp-file semantics
3. track spool file size/content type/path
4. delete successful spool files immediately after worker success
5. retain failed spool files for `24 hours`
6. add periodic cleanup for expired failed spool files

Important detail:

- archive spool must not be mixed with finalized local archive objects

## 5.3 Response writer tee

Main files:

- new archive writer wrapper in `service/archive/` or `middleware/`
- route middleware wiring in `router/relay-router.go` and `router/video-router.go`

Required changes:

1. wrap `gin.ResponseWriter` for eligible routes
2. tee written bytes into archive capture sinks
3. preserve:
   - status code
   - headers
   - `http.Flusher`
   - byte ordering
   - stream flush behavior
4. ensure binary proxy paths such as `VideoProxy` are captured without content corruption

Important detail:

- use the wrapper as the common non-stream capture path instead of hand-patching every provider response function first

## 5.4 Stream capture hardening

Main files:

- `relay/helper/common.go`
- optionally targeted stream handlers where helper coverage is insufficient

Required changes:

1. ensure helper-based stream writes still flow through the wrapped writer
2. verify final archived stream bytes match customer-visible bytes
3. avoid capturing from raw upstream scanner input as the primary archive source

Important detail:

- stream safety is a correctness gate, not an optimization target

## 5.5 Retry attempt summary capture

Main files:

- `controller/relay.go`
- new archive state helper package

Required changes:

1. accumulate per-attempt summary entries during retry progression:
   - retry index
   - channel id/name/type
   - status/error summary
   - timeout metadata
   - timestamps / duration
   - result (`failed`, `success`, `skipped`)
2. pass accumulated attempt summaries into manifest finalization

Important detail:

- keep this summary path separate from payload capture

## 5.6 Manifest and backend writes

Main files:

- new archive manifest helper in `service/archive/`
- local backend implementation
- Azure backend implementation

Required changes:

1. create placeholder manifest state early
2. finalize manifest after worker processing
3. write standardized object names:
   - `manifest.json.gz`
   - `request.data.gz`
   - `response.data.gz`
4. use `request_id`-keyed date-partitioned paths
5. include `trace_id` only in manifest/metadata, not path primary key
6. for Azure Blob:
   - add official SDK dependency
   - upload gzip objects
   - write minimal metadata:
     - `request_id`
     - `trace_id`
     - `trace_id_present`
     - `object_type`
     - `is_stream`
     - `storage_mode`
     - `content_encoding`
     - `content_type`
     - `created_at`

Important detail:

- backend mode is single-select; no dual-write or fallback semantics in this phase

## 5.7 Log metadata integration

Main files:

- `model/log.go`
- possibly `service/log_info_generate.go`
- runtime logging helpers

Required changes:

1. add one structured `archive` object into `logs.other`
2. record at least:
   - `enabled`
   - `backend`
   - `status`
   - `reason`
   - `manifest_object`
   - `request_object`
   - `response_object`
   - raw bytes
   - gzip bytes
   - `is_stream`
3. ensure consume logs and error logs both persist archive metadata where available
4. emit runtime logs only for `skipped` and `failed` events

Important detail:

- database rows must never contain full request or response payloads

## 5.8 Route coverage

Main files:

- `router/relay-router.go`
- `router/video-router.go`
- task/video controller and relay entrypoints only as needed

Required changes:

1. enable archive middleware on main relay routes
2. enable archive middleware on task/video routes included in scope
3. exclude out-of-scope routes (`mj`, realtime, channel-test) in the first version unless needed for shared helper correctness

## 6. Execution Waves

## Wave 1: Archive foundation

Files:

- `common/` archive config
- new `service/archive/` package for state, spool, queue item, manifest types

Deliverables:

- archive config surface
- request-scoped archive state model
- queue item model
- spool helpers
- placeholder/final manifest schema

## Wave 2: Route middleware and request capture

Files:

- archive middleware / writer wrapper
- `router/relay-router.go`
- `router/video-router.go`
- `controller/relay.go` integration points

Deliverables:

- eligible route coverage
- request body capture from reusable storage
- writer tee for final response capture
- retry attempt summary accumulation hooks

## Wave 3: Local backend and metadata persistence

Files:

- `service/archive/store_local.go`
- `model/log.go`
- runtime logging helpers

Deliverables:

- local object finalization
- gzip object writes
- `logs.other.archive`
- skipped/failed runtime logs
- spool cleanup rules

## Wave 4: Stream/video hardening and tests

Files:

- `relay/helper/common.go`
- `controller/video_proxy.go`
- tests across relay/task/video and log metadata

Deliverables:

- stream-safe final-byte capture
- binary proxy capture
- queue full / size limit / failure-mode coverage

## Wave 5: Azure backend

Files:

- `go.mod` / `go.sum`
- `service/archive/store_azure_blob.go`
- archive config wiring

Deliverables:

- Azure Blob backend behind shared interface
- metadata writes
- backend mode switching

## 7. Testing Plan

## 7.1 Request capture tests

Cover:

- JSON request body archived without breaking request parsing
- disk-backed request body archived without forcing large in-memory copy semantics where avoidable
- retry path still reuses body successfully after request capture

## 7.2 Non-stream response tests

Cover:

- final client-visible JSON/body bytes are archived exactly
- task JSON responses are archived exactly
- proxied binary video bytes are archived exactly

## 7.3 Stream response tests

Cover:

- final SSE/chunk output archived exactly
- chunk order preserved
- stream completion still emits expected client output
- archive capture does not break flush-capable paths

## 7.4 Failure-isolation tests

Cover:

- queue full -> request succeeds, archive marked `skipped_queue_full`
- oversized request object -> skipped with limit metadata and runtime log
- oversized response object -> skipped with limit metadata and runtime log
- local backend write failure -> request succeeds, archive marked failed
- Azure backend write failure -> request succeeds, archive marked failed

## 7.5 Metadata and manifest tests

Cover:

- `logs.other.archive` exists and is structured
- `trace_id` is preserved in manifest/metadata
- `request_id`-keyed object naming is correct
- manifest records retry attempt summaries
- placeholder manifest is finalized correctly

## 8. Risks and Mitigations

## Risk 1: Response-writer tee misses a provider write path

Mitigation:

- cover main relay, task/video, and representative stream helper paths in tests
- patch only the remaining uncovered helpers after the generic wrapper is in place

## Risk 2: Writer wrapper breaks Gin behavior

Mitigation:

- preserve `gin.ResponseWriter` methods and `http.Flusher`
- keep wrapper narrow and test against JSON, binary, and SSE outputs

## Risk 3: Async worker completion races with log persistence

Mitigation:

- decide one consistent first-version timing model for log metadata
- if final object refs are not available synchronously, persist a safe partial archive status and update where the codebase already supports it; otherwise finalize before consume/error log write for the supported path

## Risk 4: Azure backend expands scope before local behavior is proven

Mitigation:

- complete and verify local backend first
- keep Azure work isolated to one backend module and config layer

## Risk 5: Spool/temp growth overwhelms disk

Mitigation:

- immediate success cleanup
- `24h` failed-spool TTL cleanup
- runtime skip/fail logs
- clear object-size caps

## 9. Current Recommendation on First Execution Scope

Implement in this order:

1. archive config + state + spool helpers
2. route middleware + response-writer tee
3. request capture from `BodyStorage`
4. local backend + `logs.other.archive`
5. stream/video hardening tests
6. Azure backend

This keeps the riskiest correctness surfaces in front of the cross-cloud storage work.

## 10. Verification Gate

This plan should be considered ready for execution when these are all true:

- every locked decision from `03-CONTEXT.md` is represented in architecture, wave sequencing, or tests
- request capture reuses existing body-storage infrastructure
- response capture is defined in terms of final client-visible bytes
- queue items never carry full payload bytes
- archive failures remain non-blocking by default
- `logs.other` stores metadata and object refs only, not payloads
- Azure backend remains behind the same interface as local backend

---

*Phase: 3-Request Response Archive Pipeline*
*Research completed: 2026-06-09*
*Plan drafted: 2026-06-09*
