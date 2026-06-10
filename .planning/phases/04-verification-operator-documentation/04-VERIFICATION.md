# Phase 4 Verification Notes

**Phase:** 4  
**Status:** Verification expanded  
**Updated:** 2026-06-10

## Completed Verification

The touched packages were verified with the local GVM Go toolchain. The shell environment has a mismatched default `GOROOT`, so commands were run with explicit `GOROOT`.

```bash
GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./service/archive ./middleware ./router ./model
GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./controller -run '^$'
GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./middleware ./router ./model -run 'Trace|RecordConsumeLog|RecordErrorLog|Archive|Queue|Oversize'
GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./service/archive
```

Results:

- `service/archive` passed.
- `middleware` passed.
- `router` passed.
- `model` passed.
- `controller` compile-only passed.
- focused trace/archive/log tests passed across `middleware`, `router`, and `model`.

## Covered Behaviors

- Local backend writes gzip request, response, and manifest objects.
- Oversized response capture skips the object without truncating the client response.
- Streaming/SSE-style chunk writes are captured in the same order as client-visible output.
- Oversized request capture skips the object and removes the temporary spool file.
- Queue overflow marks archive status as skipped with reason `queue_full`.
- Archive status summarization prioritizes failures over partial skips.
- Archive middleware compiles across route wiring.
- Route wiring compiles with relay/task/video integration.
- Log metadata patch helper compiles against the model layer.
- Retry attempt summary integration compiles in relay and task retry loops.

## Known Verification Limits

- Full `controller` test run has an existing database-state-dependent failure unrelated to the archive implementation (`sql: database is closed` in `TestListModelsTokenLimitIncludesTieredBillingModel`).
- Azure Blob backend is compile-verified but still needs a credentialed smoke test.
- Local end-to-end UAT with `ARCHIVE_ENABLED=true` should still be run against a live relay request.
- High-RPM soak testing is not yet run in this phase.

## Recommended UAT

1. Start the service with local archive enabled:

```bash
ARCHIVE_ENABLED=true \
ARCHIVE_BACKEND=local \
ARCHIVE_LOCAL_DIR=/tmp/new-api-archive \
GOROOT=/Users/zf/.gvm/gos/go1.25.1 \
go run .
```

2. Send one normal relay request without `Trace-Id`.
3. Send one normal relay request with `Trace-Id: Abc123`.
4. Confirm both responses still return normally.
5. Confirm `logs.other.trace_id` exists.
6. Confirm `logs.other.archive` contains archive status and object byte/hash metadata.
7. Confirm `/tmp/new-api-archive/objects/<request_id>/manifest.json.gz` exists.
8. Decompress `manifest.json.gz` and confirm:

- `request_id`
- `trace_id`
- `trace_id_present`
- request object status
- response object status
- retry attempt summaries

9. Send an oversized response/request test when feasible and confirm the object is skipped, not truncated.
10. For Azure Blob, repeat with `ARCHIVE_BACKEND=azure_blob` and real credentials.
