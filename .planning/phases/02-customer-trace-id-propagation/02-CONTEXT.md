# Phase 2: Customer Trace-Id Propagation - Context

**Gathered:** 2026-06-09
**Status:** Ready for planning

<domain>
## Phase Boundary

This phase adds customer-provided trace correlation to the relay request chain without replacing the existing server-generated request ID. It covers extraction from the inbound `Trace-Id` header, validation, context propagation, runtime log enrichment, and persistence into `logs.other`. It does not yet cover payload archival or archive object naming implementation.

</domain>

<decisions>
## Implementation Decisions

### Trace Source Policy
- **D-01:** Only the inbound HTTP header `Trace-Id` is recognized as the customer trace source in this phase.
- **D-02:** `X-Trace-Id`, `X-Request-Id`, and other aliases are not accepted as fallback sources in this phase.

### Presence, Empty, and Multi-Value Handling
- **D-03:** If the `Trace-Id` header is absent, the request continues and the normalized trace value becomes the empty string.
- **D-04:** If the `Trace-Id` header is present but empty, the request must fail with HTTP `400`.
- **D-05:** If the `Trace-Id` header appears with multiple values, the request must fail with HTTP `400`.

### Validation Rules
- **D-06:** A valid trace ID may contain only ASCII letters and digits.
- **D-07:** A valid trace ID must preserve original case; no case normalization is allowed.
- **D-08:** Any whitespace anywhere in the value is invalid. Leading whitespace, trailing whitespace, and embedded whitespace all fail validation with HTTP `400`.
- **D-09:** Maximum length is `64` characters. Values longer than `64` fail validation with HTTP `400`.
- **D-10:** Validation failures should return request-validation style errors with specific messages describing the exact `Trace-Id` violation.

### Internal Naming and Context Propagation
- **D-11:** The unified internal semantic name is `trace_id`.
- **D-12:** The validated value must be written to Gin context under `trace_id`.
- **D-13:** The validated value must also be written into `c.Request.Context()` under the same `trace_id` semantic key.
- **D-14:** Downstream code in this phase should read the normalized trace value from context, not by re-reading the raw header.

### Relationship with Existing Request ID
- **D-15:** Customer `trace_id` and server-generated `request_id` must remain separate concepts.
- **D-16:** No code in this phase may replace, rename, or overload the existing internal `request_id`.
- **D-17:** Any main relay-chain runtime log point that already outputs `request_id` must also output `trace_id`.

### Log Persistence
- **D-18:** `logs.other` must always contain a `trace_id` field for consume logs and error logs.
- **D-19:** When the inbound `Trace-Id` header is absent, `logs.other.trace_id` must be persisted as the empty string rather than omitted or set to `null`.
- **D-20:** Error logs must include both `request_id` and `trace_id`.
- **D-21:** Consume logs must include both `request_id` and `trace_id`.

### Middleware Placement
- **D-22:** Trace extraction and validation should be implemented in a dedicated middleware, not duplicated across controllers or relay handlers.
- **D-23:** The middleware should run on the relay main path before business logic consumes the request.

### Forward Compatibility
- **D-24:** The normalized `trace_id` field is the canonical value that later archive work should reuse.

### Verification Scope
- **D-25:** Verification must cover:
  - header missing -> empty trace ID propagated and persisted
  - header present but empty -> `400`
  - header multi-value -> `400`
  - whitespace anywhere -> `400`
  - non-alphanumeric characters -> `400`
  - length > 64 -> `400`
  - valid mixed-case alphanumeric value -> case preserved
  - Gin context and request context both carry `trace_id`
  - consume logs persist `logs.other.trace_id`
  - error logs persist `logs.other.trace_id`
  - runtime log points that include `request_id` also include `trace_id`

### the agent's Discretion
- Middleware naming, helper naming, and constant placement may follow the nearest existing project conventions as long as the runtime semantic name exposed to downstream code remains `trace_id`.
- The exact request-validation error code symbol may reuse an existing request error code or introduce a dedicated one if that materially improves clarity without destabilizing current error handling.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase and requirements
- `.planning/ROADMAP.md` — Phase 2 goal, success criteria, and plan breakdown
- `.planning/REQUIREMENTS.md` — `TRAC-*` define this phase's deliverables
- `.planning/PROJECT.md` — milestone context and logging/archive constraints
- `.planning/STATE.md` — current project position after Phase 1 closeout

### Existing request ID and logging code
- `middleware/request-id.go` — current server-generated request ID creation and request-context propagation pattern
- `middleware/logger.go` — current request runtime log formatting that already includes request ID
- `controller/relay.go` — main relay path, runtime error logging, and database error-log trigger path
- `model/log.go` — consume log and error log persistence, including `logs.other` construction
- `common/constants.go` — existing request/upstream request ID constants and shared context keys
- `middleware/utils.go` — error response helpers that currently append request ID

### Relay-chain integration points
- `relay/common/relay_info.go` — existing use of request-scoped context values during relay setup
- `relay/helper/common.go` — helper log formatting locations using the current request ID
- `controller/channel-test.go` — one consume-log path outside the main customer relay handler that still uses the same log persistence functions
- `relay/mjproxy_handler.go` — alternate consume-log path that should inherit `logs.other.trace_id` behavior if it flows through the same log model helpers

### Planning context
- `AGENTS.md` — project constraints: cross-DB support, JSON wrapper rule, preserve protected identifiers

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `middleware/request-id.go` already shows the local project pattern for writing a value into both Gin context and `c.Request.Context()`.
- `model.RecordErrorLog(...)` and `model.RecordConsumeLog(...)` already centralize `logs.other` persistence, so trace persistence should be added there rather than reimplemented at each caller.
- `middleware/logger.go` already resolves request-scoped values from the Gin log formatter path, making it the primary place to enrich access logs with `trace_id`.

### Established Patterns
- Request-scoped values are frequently stored in Gin context with shared constant keys.
- Runtime request IDs are appended to customer-facing error text through shared helpers, so trace handling should avoid introducing a separate ad hoc output convention.
- `logs.other` is already the established place for structured request metadata that should not become a first-class database column.

### Integration Points
- Add a new trace middleware alongside `middleware/request-id.go`.
- Define a shared semantic key for `trace_id` in common constants or the closest existing constant module.
- Update runtime request logging in `middleware/logger.go`.
- Update database log persistence in `model/log.go`.
- Update relay main-path logging in `controller/relay.go` and any shared helpers that currently emit `request_id` without `trace_id`.

</code_context>

<specifics>
## Specific Ideas

- The user explicitly wants `Trace-Id` to be strict, not permissive: only one header name, only alphanumeric characters, and any whitespace is invalid.
- The user explicitly wants invalid `Trace-Id` values rejected instead of silently normalized.
- The user explicitly wants `trace_id` to always be present in `logs.other`, even when the inbound header is absent.
- The user explicitly wants every main-chain runtime log point that already includes `request_id` to also include `trace_id`.

</specifics>

<deferred>
## Deferred Ideas

- Archive object naming and archive metadata reuse are deferred to Phase 3 implementation, even though this phase locks `trace_id` as the reusable canonical field.
- Header alias support such as `X-Trace-Id` or `X-Request-Id` is deferred unless requirements change later.

</deferred>

---

*Phase: 2-Customer Trace-Id Propagation*
*Context gathered: 2026-06-09*
