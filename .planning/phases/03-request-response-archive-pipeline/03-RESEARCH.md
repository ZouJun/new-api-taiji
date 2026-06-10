# Phase 3: Request Response Archive Pipeline - Research

**Date:** 2026-06-09
**Status:** Complete

## Objective

Research how to implement bounded request/response archival for the main relay path plus task/video routes without destabilizing retries, billing, stream flush behavior, or cross-backend storage selection.

## Files Read

- `.planning/phases/03-request-response-archive-pipeline/03-CONTEXT.md`
- `.planning/ROADMAP.md`
- `.planning/REQUIREMENTS.md`
- `.planning/codebase/ARCHITECTURE.md`
- `.planning/codebase/INTEGRATIONS.md`
- `.planning/codebase/CONCERNS.md`
- `common/gin.go`
- `common/body_storage.go`
- `common/disk_cache.go`
- `middleware/body_cleanup.go`
- `controller/relay.go`
- `controller/video_proxy.go`
- `relay/helper/common.go`
- `relay/helper/stream_scanner.go`
- `relay/helper/valid_request.go`
- `relay/relay_task.go`
- representative response handlers under `relay/channel/openai/`, `relay/channel/claude/`, `relay/channel/aws/`, `relay/channel/task/sora/`
- `model/log.go`
- `service/http_client.go`
- `reliability-trace-archive-plan.md`

## Key Findings

### 1. Request body capture already has a reusable, disk-backed foundation

The project already routes relay request-body parsing through `common.UnmarshalBodyReusable(...)`, which in turn relies on `common.GetBodyStorage(...)`.

Important properties:

- `GetBodyStorage(...)` can create memory-backed or disk-backed body storage depending on size and disk-cache settings.
- The storage is seekable and reusable across retries.
- `controller.Relay` already resets `c.Request.Body = io.NopCloser(bodyStorage)` before each retry attempt.
- `middleware.BodyStorageCleanup()` already cleans request-body storage after the request completes.

Implication:

- Phase 3 should not introduce a second request-body buffering mechanism.
- Request archival should read from `BodyStorage` after request validation/body reuse is established.
- For large requests, archive capture can piggyback on the existing disk-backed storage path instead of pulling everything back into heap memory.

### 2. Final client-visible response capture is more practical at the response-writer layer than inside every provider

The relay surface is heterogeneous:

- many non-stream handlers `io.ReadAll(resp.Body)` and then call `c.JSON(...)`, `c.Data(...)`, or `c.Writer.Write(...)`
- stream handlers often call `helper.StringData(...)`, `helper.ObjectData(...)`, `helper.Done(...)`, `helper.ClaudeData(...)`, `helper.ResponseChunkData(...)`
- `VideoProxy` streams upstream content directly with `io.Copy(c.Writer, resp.Body)`

Because the user locked "archive what the client actually receives", a response-writer tee is the strongest common interception point.

Practical implication:

- introduce an archive-capable `gin.ResponseWriter` wrapper for eligible routes
- tee bytes written to the client into archive capture sinks
- preserve `http.Flusher`, status, headers, and write ordering
- use the wrapper for non-stream, stream, and video-proxy cases instead of patching every provider path first

This should dramatically reduce provider-specific archival code.

### 3. Stream safety requires teeing final writes, not scanner input

`relay/helper/stream_scanner.go` is already a fragile concurrency-sensitive path:

- goroutines coordinate scanner, ping, data handler, stop signaling, and timeout
- stream timeout and end-state behavior are already locked from Phase 1
- chunk ordering and flush timing are critical

Implication:

- do not archive by hooking the raw upstream scanner input as the primary source
- archive the final bytes at `helper.StringData`, `helper.ObjectData`, `helper.Done`, `helper.ClaudeData`, `helper.ResponseChunkData`, or the wrapped writer below them
- this aligns with the user decision to store final client-visible SSE/chunk bytes

### 4. Task/video routes are response-shape-diverse but still writer-centric

`RelayTask` / `RelayTaskFetch` routes eventually go through task adaptors that commonly:

- read upstream body fully
- parse task payload
- respond with `c.JSON(...)`

`VideoProxy` behaves differently:

- forwards selected upstream headers
- writes status and then streams raw bytes with `io.Copy(c.Writer, resp.Body)`

Implication:

- the same route-level response-writer tee can cover both task JSON responses and proxied binary video content
- task/video scope does not require a separate archive subsystem design

### 5. Existing disk-cache utilities are close to what Phase 3 needs, but not identical

The project already has:

- `common.CreateDiskCacheFile(...)`
- `common.WriteDiskCacheFile(...)`
- `common.RemoveDiskCacheFile(...)`
- `common.CleanupOldDiskCacheFiles(...)`

These utilities currently back request body and file cache temp storage under a unified disk-cache directory.

Implication:

- archive spool can reuse these filesystem patterns and safety practices
- but archive spool should still live under its own logical directory / type rather than mixing finalized archive artifacts with generic body/file cache temp files
- the implementation may either extend the disk-cache utility set with archive-specific types or introduce a dedicated archive spool helper that follows the same conventions (mkdir, temp file, 0600 perms, TTL cleanup)

### 6. Azure Blob support is not present yet

The repository currently has no Azure Blob SDK dependency in `go.mod`.

Implication:

- Phase 3 must explicitly add the official Azure Blob SDK dependency
- the planner should include a contained backend interface so local storage lands first, and Azure support plugs into the same interface
- because Azure is absent today, local backend and interface shape should be treated as the implementation anchor

### 7. `logs.other` integration should stay centralized in `model/log.go`

Phase 2 already centralized `trace_id` persistence in `RecordConsumeLog(...)` and `RecordErrorLog(...)`.

Implication:

- archive metadata should be attached centrally in the same log model helpers or in a closely-related helper invoked there
- avoid provider-specific or controller-specific ad hoc `other["archive_*"]` mutations
- the final shape should be one structured `archive` object as locked by context

### 8. Retry summaries belong in manifest assembly, not response capture

`controller.Relay` already tracks:

- `retry`
- `use_channel`
- timeout metadata
- channel error processing
- final error logging

Implication:

- attempt summaries should be accumulated as request-scoped metadata during retry progression
- manifest finalization can serialize this accumulated attempt list
- response capture should stay focused on payload bytes only

### 9. The most stable first-version architecture is "capture in-request, store out-of-band"

Given the user's locked decisions, the most coherent architecture is:

1. request enters eligible route
2. archive middleware wraps writer and initializes request-scoped archive state
3. request body is copied from reusable `BodyStorage` into spool once
4. response bytes are tee-captured as final client-visible output
5. placeholder manifest is created
6. on request completion, a lightweight archive job containing paths/metadata is enqueued
7. worker compresses/writes local object or uploads Azure
8. worker finalizes manifest and archive status
9. consume/error logs persist `logs.other.archive`

This matches the user preference for:

- non-blocking behavior
- queue entries containing lightweight metadata only
- request/response payloads first landing in local spool files

## Recommended Implementation Shape

### Archive state and middleware

Recommended additions:

- archive route middleware for eligible relay and video/task groups
- request-scoped archive state struct stored in Gin context
- response-writer wrapper that tees bytes and tracks status/content type/stream flag

Why middleware:

- broadest coverage with least provider churn
- allows one place to decide whether archival is enabled
- aligns with existing request-id / trace-id / body-cleanup route patterns

### Backend interface

Recommended interface shape:

- one archive service entrypoint invoked after request handling
- one storage backend interface with local and Azure implementations
- manifest builder/finalizer separated from backend implementation

Example responsibility split:

- `service/archive/manager.go` — lifecycle, enqueue, worker orchestration
- `service/archive/store_local.go` — finalized object writes under local root
- `service/archive/store_azure_blob.go` — Azure uploads and metadata
- `service/archive/spool.go` — spool temp file management
- `service/archive/manifest.go` — manifest assembly/finalization
- `service/archive/writer.go` — response writer tee

### Configuration placement

The codebase commonly uses env vars in `common/init.go` and typed settings packages under `setting/`.

For Phase 3 first version, global env-backed config is the least risky path:

- `ARCHIVE_ENABLED`
- `ARCHIVE_BACKEND`
- `ARCHIVE_LOCAL_DIR`
- `ARCHIVE_QUEUE_SIZE`
- `ARCHIVE_WORKER_COUNT`
- `ARCHIVE_MAX_REQUEST_BYTES`
- `ARCHIVE_MAX_RESPONSE_BYTES`
- `ARCHIVE_SPOOL_DIR`
- `ARCHIVE_SPOOL_TTL_HOURS`
- Azure credentials/settings

This matches the locked "global-only" decision and avoids per-channel settings expansion in this phase.

## Risks and Mitigations

### Risk 1: Response-writer wrapper breaks stream flush behavior

Mitigation:

- preserve the full `gin.ResponseWriter` contract
- preserve `http.Flusher`
- add focused stream tests that assert byte-for-byte output and flush-compatible completion

### Risk 2: Queue design accidentally carries payload bytes

Mitigation:

- make queue items contain only file paths + metadata
- explicitly forbid `[]byte` payload storage in queue item struct
- verify with code review and targeted tests

### Risk 3: Archive metadata races with consume/error log timing

Mitigation:

- define exactly when archive status is considered final for success and failure requests
- allow placeholder/partial metadata before worker completion only if the log path can safely finalize later, otherwise log request-local status plus final object refs when available
- plan targeted tests for consume log, error log, queue full, and oversized object cases

### Risk 4: Azure backend adds too much scope before local path is stable

Mitigation:

- implement and verify local backend first behind the shared interface
- add Azure after object naming, manifest, queue, and spool behavior are stable

## Open Planning Constraints

These are not unresolved user decisions; they are planner obligations:

- choose the narrowest safe insertion point for request capture in `controller.Relay` and task/video flows
- choose whether manifest placeholder persistence happens in spool or final object root first
- define how `logs.other.archive` gets updated when worker completion is asynchronous relative to response completion
- define how video-proxy binary content is labeled in `content_type` and object metadata
- choose the exact Azure SDK package and initialization path

## Research Conclusion

Phase 3 should be planned as a layered implementation:

1. shared archive state, config, and local spool/object model
2. route middleware + writer tee + request body capture
3. local backend + manifest + queue/worker + log metadata
4. stream/video hardening tests
5. Azure backend plugged into the same store interface

This order matches the user's stability-first constraints and the repository's existing reusable request-body and writer patterns.

---

*Research completed for Phase 3 on 2026-06-09*
