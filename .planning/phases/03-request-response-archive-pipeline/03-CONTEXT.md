# Phase 3: Request Response Archive Pipeline - Context

**Gathered:** 2026-06-09
**Status:** Ready for planning

<domain>
## Phase Boundary

This phase designs and implements bounded request/response archival for the main relay path plus task/video routes. It covers request capture, final client-visible response capture, local or Azure Blob storage selection, object naming, manifest structure, async queueing, spool file lifecycle, and failure isolation. It does not add archive search UI, offline compaction, tenant-specific retention policy, or per-channel archive controls.

</domain>

<decisions>
## Implementation Decisions

### Scope Boundary
- **D-01:** Phase 3 first version must cover the main relay path plus task/video routes.
- **D-02:** `mj`, realtime, and channel-test paths are not required in the first version.
- **D-03:** Future archive work must continue to reuse the already-validated `trace_id` from Phase 2 rather than inventing a second customer trace field.

### Archive Object Model
- **D-04:** Each customer request produces three archive objects: `manifest`, `request`, and `final response`.
- **D-05:** Failed retry attempts do not persist full request or response payload objects.
- **D-06:** The manifest must record per-attempt summary entries for retry/channel/error analysis.
- **D-07:** The manifest should be written in placeholder form first and then updated/finalized after archival work completes.

### Streaming Capture Semantics
- **D-08:** Streaming archival stores the final SSE/chunk bytes actually sent to the customer, not the raw upstream stream.
- **D-09:** Non-stream archival stores the final client-visible response body.
- **D-10:** Request capture should occur as soon as the reusable request body is available.
- **D-11:** Response capture should follow best practice for the existing codebase: capture the final client-visible bytes at the write path, using tee-style interception that preserves order and flush behavior.

### Failure Isolation
- **D-12:** Archival is non-blocking by default. Archive failures, queue overflow, or size-limit skips must never turn an otherwise successful customer request into a failed request in this phase.
- **D-13:** Archive skip/failure reasons must be recorded in both `logs.other.archive` and runtime logs.
- **D-14:** Runtime logs should be emitted only for archive `skipped` and `failed` events, not for every queued/success event, to avoid excessive log volume at high RPM.

### Size Limits and Compression
- **D-15:** Default maximum request object size is `128MB`.
- **D-16:** Default maximum response object size is `128MB`.
- **D-17:** If a request or response object exceeds its configured limit, that object is skipped rather than truncated.
- **D-18:** Size-limit skips must record the actual size, configured limit, and skip reason, and must emit runtime logs.
- **D-19:** Archive objects are gzip-compressed by default.

### Queue and Worker Model
- **D-20:** Archive ingestion uses a bounded, non-blocking in-memory queue.
- **D-21:** Default queue size is `50000`.
- **D-22:** Default worker count is `32`.
- **D-23:** If the queue is full, archival is skipped with reason `skipped_queue_full`.
- **D-24:** Queue entries must contain only lightweight metadata and local spool file paths, never full request/response payload bytes.

### Spool and Temporary File Strategy
- **D-25:** Request/response payloads must first be written to local spool files before archive work is queued.
- **D-26:** The queue should carry spool file paths plus request metadata, not payload buffers.
- **D-27:** A small amount of archive data loss is acceptable in exchange for protecting main relay stability.
- **D-28:** Successful spool files are deleted immediately after archive completion.
- **D-29:** Failed spool files are retained temporarily for diagnostics and cleaned up later.
- **D-30:** Default failed-spool retention TTL is `24 hours`.

### Backend Selection
- **D-31:** Archive backend selection is single-backend only: one request writes to either `local` or `azure_blob`, never both in this phase.
- **D-32:** Backend selection is controlled by global configuration, not by table logic changes and not by per-channel/per-route overrides in the first version.

### Local Layout and Naming
- **D-33:** Local archive storage uses separate `spool/` and `objects/` directory trees.
- **D-34:** Formal object paths use `request_id` as the path key; `trace_id` lives in manifest and metadata, not the path primary key.
- **D-35:** Recommended path pattern is date-partitioned by day and then grouped by `request_id`.
- **D-36:** Standard object names are:
  - `manifest.json.gz`
  - `request.data.gz`
  - `response.data.gz`
- **D-37:** File extension should not attempt to encode payload media type; content type is carried in manifest/metadata.

### Manifest and Metadata Shape
- **D-38:** `logs.other` stores a single structured `archive` object containing archive status and references.
- **D-39:** Azure Blob objects must include minimal metadata:
  - `request_id`
  - `trace_id`
  - `trace_id_present`
  - `object_type`
  - `is_stream`
  - `storage_mode`
  - `content_encoding`
  - `content_type`
  - `created_at`
- **D-40:** The manifest must capture at least:
  - request identity (`request_id`, `trace_id`, trace presence)
  - object references (`manifest`, `request`, `response`)
  - raw and gzip byte sizes
  - stream flag
  - content types
  - backend
  - overall archive status and reason
  - retry attempt summaries
  - channel/model/time metadata relevant for request audit

### Configuration Surface
- **D-41:** The first version exposes only global archive controls and does not add per-channel or per-route archive toggles.
- **D-42:** The first version should include global configuration for:
  - archive enablement
  - backend selection
  - queue size
  - worker count
  - request size limit
  - response size limit

### Capacity Guidance
- **D-43:** For single-node deployment with archival enabled, the minimum startup baseline is `4 vCPU / 8 GB RAM / 100 GB SSD`.
- **D-44:** The recommended single-node production starting point is `8 vCPU / 16 GB RAM / 200 GB SSD`.
- **D-45:** If the app, MySQL, and Redis run on the same node, the recommended starting point is `12 vCPU / 24 GB RAM`.

### the agent's Discretion
- Exact config key names, package/module boundaries, and spool cleanup scheduling may follow existing project conventions as long as the locked semantics above are preserved.
- Response capture may use the narrowest safe tee/writer wrapping points available in relay helpers and response writers, as long as the archived bytes remain the final client-visible output.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase and requirements
- `.planning/ROADMAP.md` — Phase 3 goal, success criteria, and plan breakdown
- `.planning/REQUIREMENTS.md` — `ARCH-*` and `OPS-*` define this phase's deliverables
- `.planning/PROJECT.md` — milestone context, throughput target, and archive constraints
- `.planning/STATE.md` — current project position after Phase 2 completion

### Prior phase decisions that must carry forward
- `.planning/phases/01-aws-claude-timeout-audit/01-CONTEXT.md` — timeout behavior and retry observability already locked for relay requests
- `.planning/phases/02-customer-trace-id-propagation/02-CONTEXT.md` — `trace_id` semantics, validation, and persistence rules that archival must reuse

### Existing relay and request-body integration points
- `controller/relay.go` — main request lifecycle, retry loop, error logging, and final relay orchestration
- `relay/helper/valid_request.go` — reusable request-body parsing through `common.UnmarshalBodyReusable`
- `common` body storage helpers referenced by `controller/relay.go` — request body reuse path that archive capture should align with
- `router/relay-router.go` — main relay route coverage
- `router/video-router.go` — task/video route coverage included in this phase

### Streaming and response-write integration points
- `relay/helper/common.go` — final SSE/chunk write helpers and flush points for client-visible stream output
- `relay/helper/stream_scanner.go` — stream scanner loop, first-byte timeout path, and stream fragility considerations
- `relay/common/stream_status.go` — stream end-state model already used by logging/settlement
- provider `DoResponse` / stream handlers under `relay/channel/` — non-stream and stream final response formatting paths that planning must map

### Logging and metadata persistence
- `model/log.go` — consume/error log persistence and `logs.other` merge behavior
- `service/log_info_generate.go` — existing structured `other` generation patterns that archive metadata should align with
- `logger/logger.go` — runtime log formatting with `request_id` and `trace_id`

### Project constraints
- `AGENTS.md` — cross-DB support, JSON wrapper rule, protected identifiers, and current milestone focus
- `.planning/codebase/ARCHITECTURE.md` — request flow and relay boundaries
- `.planning/codebase/INTEGRATIONS.md` — local/Azure integration context and shared client behavior
- `.planning/codebase/CONCERNS.md` — archival throughput, stream fragility, and sensitive-data constraints

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `common.UnmarshalBodyReusable(...)` and the request body storage path already support reading request payloads without breaking retries.
- `controller.Relay` already resets `c.Request.Body` from stored body data for retries, giving the archive design a stable place to capture request bytes once and reuse them.
- `model.RecordConsumeLog(...)` and `model.RecordErrorLog(...)` already centralize metadata persistence into `logs.other`.
- `relay/helper/common.go` centralizes key stream write helpers (`StringData`, `ClaudeData`, `ResponseChunkData`, `Done`, `FlushWriter`), making it the best candidate for teeing final streamed output.

### Established Patterns
- The relay hot path already protects billing cleanup and retry semantics through `controller.Relay` defers; archive failures must not interfere with those flows.
- Stream handling is already treated as fragile and time-sensitive; capture must preserve chunk order and flush timing.
- `logs.other` is used for structured metadata only, not for payload storage.
- Request-scoped correlation already flows through `request_id`, `upstream_request_id`, and `trace_id`; archival should attach to these rather than inventing new correlation fields.

### Integration Points
- Request capture should hook immediately after request validation/body reuse is established, before retry fan-out starts mutating per-attempt state.
- Non-stream response capture should hook at the final client response write path after provider conversion is complete.
- Stream response capture should hook into the final client-facing stream write helpers rather than raw upstream scanner input.
- Async archive enqueueing and spool cleanup likely belong in a dedicated package/service to keep `controller/relay.go` from becoming more overloaded.

</code_context>

<specifics>
## Specific Ideas

- The user explicitly accepts a small amount of archive loss in exchange for keeping the main relay path stable.
- The queue and worker defaults are intentionally sized for 8000-15000 RPM pressure, but only under the locked assumption that queue items do not hold full payload bytes.
- The user wants explicit runtime logs for archive skip/failure cases, not silent metadata-only behavior.
- The user prefers stable `request_id`-keyed object paths, with `trace_id` treated as metadata instead of a path primary key.

</specifics>

<deferred>
## Deferred Ideas

- Dashboard archive search by trace ID remains a future phase item.
- Offline archive compaction remains a future phase item.
- Tenant-specific retention policy remains a future phase item.
- Per-channel or per-route archive enablement remains deferred; the first version is global-only.
- Dual-write or fallback archive backend behavior remains deferred; the first version is single-backend only.

</deferred>

---

*Phase: 3-Request Response Archive Pipeline*
*Context gathered: 2026-06-09*
