# Phase 2: Customer Trace-Id Propagation - Discussion Log

**Date:** 2026-06-09
**Status:** Captured

## Areas Discussed

### Trace header source
- User direction captured:
  - Only `Trace-Id` is accepted as the source header.
  - No fallback aliases are enabled in this phase.
- Locked discussion outcome:
  - `Trace-Id` is the only accepted customer trace source.

### Empty, missing, and multi-value semantics
- User direction captured:
  - Missing header is allowed.
  - Present-but-empty header is invalid.
  - Multiple values are invalid.
- Locked discussion outcome:
  - Missing `Trace-Id` -> persist empty trace string.
  - Empty or multi-value `Trace-Id` -> fail request with HTTP `400`.

### Validation rules
- User direction captured:
  - Only letters and digits are allowed.
  - Maximum length is `64`.
  - Preserve original case.
  - Any whitespace anywhere is invalid.
- Locked discussion outcome:
  - Validation is strict and reject-first; no trimming or character rewriting is allowed.

### Internal naming and propagation
- User direction captured:
  - Unified internal semantic name should be `trace_id`.
  - Implementation should unify semantics across contexts.
- Locked discussion outcome:
  - Use `trace_id` consistently in Gin context, request context, and `logs.other`.

### Relationship with request ID and logging
- User direction captured:
  - `trace_id` and `request_id` must coexist.
  - Every output point that currently includes `request_id` should also include `trace_id`.
  - Error logs and consume logs must both carry the trace value.
- Locked discussion outcome:
  - Runtime log format should include both values.
  - `logs.other.trace_id` is always persisted, even when empty.

### Error behavior and middleware placement
- User direction captured:
  - Invalid trace IDs should return HTTP `400`.
  - Middleware-based extraction/validation is acceptable.
- Locked discussion outcome:
  - Add a dedicated middleware on the relay main path to validate and inject `trace_id` before downstream business logic.

## Deferred Ideas

- Supporting alternate trace header names is deferred.
- Archive object naming work is deferred to Phase 3, but will reuse the same `trace_id` field.

## the agent's Discretion

- Exact constant names, helper names, and internal error code reuse may follow local conventions as long as the externally confirmed semantics do not change.

---

*Discussion captured for Phase 2 on 2026-06-09*
