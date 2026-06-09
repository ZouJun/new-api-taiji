# Phase 2 Plan: Customer Trace-Id Propagation

**Phase:** 2  
**Status:** Completed  
**Updated:** 2026-06-09

## 1. Goal

Add a strict customer `Trace-Id` propagation path that validates the inbound header, writes the normalized value into request context, enriches runtime logs, and persists `trace_id` into consume/error log metadata without changing the existing internal `request_id` semantics.

This phase is intentionally narrow: it establishes the correlation primitive that later archive work can reuse, but it does not yet implement archive object naming or payload capture.

## 2. Locked Decisions

These points are already confirmed and should not be reopened during implementation unless the user changes direction:

1. Only the inbound `Trace-Id` header is accepted.
2. Missing `Trace-Id` is allowed and must propagate as the empty string.
3. Present-but-empty `Trace-Id` is invalid and returns HTTP `400`.
4. Multiple `Trace-Id` values are invalid and return HTTP `400`.
5. Only ASCII letters and digits are allowed.
6. Maximum length is `64`; longer values return HTTP `400`.
7. Original case must be preserved.
8. Any whitespace anywhere in the value is invalid and returns HTTP `400`.
9. The unified internal semantic name is `trace_id`.
10. `trace_id` and server-generated `request_id` must coexist and remain separate.
11. Runtime log points that already print `request_id` on the relay main path must also print `trace_id`.
12. `logs.other.trace_id` must always exist for consume and error logs, including the empty-string case.
13. Trace extraction and validation belong in a dedicated middleware on the relay main path.

## 3. Implementation Model

## 3.1 Trace lifecycle

The effective lifecycle should be:

1. inbound request arrives
2. trace middleware reads `Trace-Id`
3. middleware validates or rejects
4. middleware writes normalized value into:
   - Gin context
   - `c.Request.Context()`
5. downstream code reads `trace_id` from context only
6. runtime logs print both `request_id` and `trace_id`
7. consume/error logs persist `logs.other.trace_id`

## 3.2 Validation semantics

Recommended helper behavior:

- header absent -> `trace_id = ""`, continue
- header exists with zero values -> reject `400`
- header exists with more than one value -> reject `400`
- value contains non-alphanumeric chars -> reject `400`
- value contains any whitespace -> reject `400`
- value length > 64 -> reject `400`

No trimming, rewriting, or normalization should occur. Validation is strict and reject-first.

## 3.3 Error behavior

Validation failures should use the existing request-validation response path and return:

- HTTP status: `400`
- explicit message describing the violation

Suggested message set:

- `invalid Trace-Id: empty value`
- `invalid Trace-Id: multiple values are not allowed`
- `invalid Trace-Id: spaces are not allowed`
- `invalid Trace-Id: only alphanumeric characters are allowed`
- `invalid Trace-Id: length exceeds 64`

## 4. Architecture Strategy

## 4.1 Middleware first

Primary control point:

- new middleware adjacent to `middleware/request-id.go`

Reason:

- centralizes header reads
- rejects invalid input before business logic, pricing, relay, and logging flows
- keeps downstream code on one stable semantic: `trace_id`

## 4.2 Context storage

Use both:

- Gin context for handler/service/model code already expecting `*gin.Context`
- `c.Request.Context()` for lower-level code and helpers that only receive `context.Context` or `*http.Request`

## 4.3 Logging propagation

Main log propagation strategy:

- runtime logs:
  - request access log formatter
  - relay-path error logging
  - helper logs that already print `request_id`
- persistence logs:
  - `RecordErrorLog`
  - `RecordConsumeLog`

## 5. Code Design

## 5.1 New middleware

Main file:

- `middleware/trace-id.go`

Responsibilities:

1. read `Trace-Id` from inbound headers
2. enforce missing / empty / multi-value semantics
3. validate charset, whitespace, and max length
4. store the effective value as `trace_id`
5. update `c.Request = c.Request.WithContext(...)`
6. reject invalid requests with HTTP `400`

Recommended helpers:

- `validateTraceID(values []string) (string, *types.NewAPIError)` or equivalent
- `setTraceID(c *gin.Context, traceID string)`

## 5.2 Shared constants and accessors

Main files:

- `common/constants.go`
- possibly a small helper in `common/` or `middleware/`

Required changes:

1. add a shared semantic key constant for `trace_id`
2. add a getter helper if it reduces duplication in logging/model code

Implementation rule:

- do not scatter raw `"trace_id"` string literals through unrelated modules if a shared constant can keep semantics aligned

## 5.3 Runtime access log

Main file:

- `middleware/logger.go`

Required changes:

1. read `trace_id` from `param.Keys`
2. extend the formatter output so `request_id` and `trace_id` appear together

Recommended format fragment:

- `request_id=<...> trace_id=<...>`

## 5.4 Relay-path runtime logs

Main files:

- `controller/relay.go`
- `relay/helper/common.go`
- any other relay-chain location already formatting request-scoped log IDs

Required changes:

1. audit main-chain runtime logs that already include `request_id`
2. add `trace_id` next to them
3. keep `request_id` unchanged as the trusted internal request identifier

## 5.5 Log persistence

Main file:

- `model/log.go`

Required changes:

1. ensure `RecordErrorLog(...)` always writes `other["trace_id"]`
2. ensure `RecordConsumeLog(...)` always writes `other["trace_id"]`
3. use `""` when the inbound header was absent
4. preserve existing timeout metadata and admin info merge behavior

Important detail:

- The trace field belongs in `logs.other`; this phase must not introduce a new database column.

## 5.6 Middleware routing

Main area:

- relay API route registration

Required changes:

1. attach the new middleware to the relay main path
2. ensure it runs before handler logic that needs the value
3. avoid accidental attachment to unrelated routes unless they truly participate in the same relay logging chain

## 6. Execution Waves

## Wave 1: Constants and middleware

Files:

- `common/constants.go`
- `middleware/trace-id.go`
- relay route registration file(s)

Deliverables:

- shared `trace_id` semantic key
- validation rules
- Gin + request context propagation
- `400` rejection path

## Wave 2: Runtime logs

Files:

- `middleware/logger.go`
- `controller/relay.go`
- `relay/helper/common.go`

Deliverables:

- main-chain logs print `request_id` and `trace_id`
- empty trace case still emits `trace_id=`

## Wave 3: Log persistence

Files:

- `model/log.go`
- any call sites that need structured `other` merge adjustments

Deliverables:

- `logs.other.trace_id` always persisted for consume/error logs
- coexistence with timeout metadata, admin info, and existing request IDs

## Wave 4: Tests and verification

Files:

- middleware tests
- controller/log tests
- model/log tests

Deliverables:

- validation coverage
- context propagation coverage
- persistence coverage
- runtime log formatting coverage

## 7. Testing Plan

## 7.1 Validation tests

Cover:

- header missing -> success with empty trace
- header empty -> `400`
- header multi-value -> `400`
- embedded whitespace -> `400`
- leading whitespace -> `400`
- trailing whitespace -> `400`
- illegal characters -> `400`
- length 65 -> `400`
- valid mixed-case alphanumeric -> success preserving case

## 7.2 Context propagation tests

Cover:

- Gin context carries `trace_id`
- request context carries `trace_id`
- missing header still sets both to `""`

## 7.3 Persistence tests

Cover:

- consume log writes `other.trace_id`
- error log writes `other.trace_id`
- empty trace persists as `""`
- trace persistence coexists with timeout metadata already added by Phase 1

## 7.4 Runtime log tests

Cover:

- access log formatter includes both `request_id` and `trace_id`
- relay-path log formatting helpers include both values when available

## 8. Risks and Mitigations

## Risk 1: Hidden header reads bypass middleware normalization

Mitigation:

- audit for direct `Trace-Id` reads and keep all downstream reads on context only

## Risk 2: Log formatting drifts across modules

Mitigation:

- patch only the main relay-chain `request_id` log points in this phase
- use one stable output convention

## Risk 3: `logs.other` merges accidentally overwrite other metadata

Mitigation:

- keep trace injection close to centralized log model helpers
- add focused tests for coexistence with timeout metadata and admin info

## Risk 4: Over-expanding scope into archive naming

Mitigation:

- lock this phase to correlation propagation only
- defer archive object naming implementation to Phase 3

## 9. Current Recommendation on First Execution Scope

Implement in this order:

1. trace constants and middleware
2. route wiring
3. runtime log enrichment
4. consume/error log persistence
5. focused tests

This gives the fastest path to a stable trace primitive without mixing in archive work or broader request metadata redesign.

## 10. Implementation Status

Completed on 2026-06-09.

Implemented files:

- `middleware/trace-id.go`
- `common/constants.go`
- `common/utils.go`
- `middleware/logger.go`
- `middleware/utils.go`
- `logger/logger.go`
- `model/log.go`
- `controller/relay.go`
- `router/relay-router.go`
- `router/video-router.go`

Tests added:

- `middleware/trace_id_test.go`
- `model/log_trace_test.go`
- `router/trace_id_router_test.go`

Verification run:

- `GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./middleware -run 'TestTraceIDMiddleware_|TestSetUpLogger_'`
- `GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./router -run 'TestRelayTaggedRoutes'`
- `GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./model -run 'Test(AppendTraceID_|RecordConsumeLog_|RecordErrorLog_)'`
- `GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./controller -run 'TestShouldRetry_'`

Main-path log audit:

- access logs output both `request_id` and `trace_id`
- shared `logger` package outputs both `request_id` and `trace_id` from request context
- relay error responses use `MessageWithRequestIdAndTraceId`
- consume/error persistence always writes `logs.other.trace_id`
- task/video relay routes are wired with the same trace middleware where they participate in the relay route group

## 11. Verification Gate

This plan should be considered ready for execution when these are all true:

- every locked decision from `02-CONTEXT.md` is represented in tasks or test coverage
- no task proposes fallback aliases for trace headers
- no task proposes trimming or rewriting invalid trace values
- persistence remains in `logs.other`, not a new DB field
- `request_id` and `trace_id` remain clearly separate in both runtime logs and stored metadata

---

*Phase: 2-Customer Trace-Id Propagation*
*Plan drafted: 2026-06-09*
*Plan completed: 2026-06-09*
