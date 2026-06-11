---
phase: 05-group-strategy-settings
plan: 02
subsystem: relay-runtime
tags: [relay, retry, timeout, group-strategy, stream]
requires:
  - phase: 05-group-strategy-settings
    provides: validated group strategy option storage
provides:
  - Shared runtime group-strategy resolver for relay retries
  - Per-request stream first-byte retry budget accounting
  - Group-aware retry ceiling integration for auto-group switching
affects: [relay, channel-selection, timeout-handling]
tech-stack:
  added: []
  patterns: [runtime snapshot in gin context, request-scoped duration accumulator]
key-files:
  created: [relay/common/group_strategy.go, relay/common/group_strategy_test.go]
  modified: [controller/relay.go, controller/relay_retry_test.go, relay/common/relay_info.go, constant/context_key.go, service/channel_select.go]
key-decisions:
  - "Resolve effective retry times from the currently matched group on every attempt, not once at request start."
  - "Count only retry-attempt first-byte waiting toward the group stream budget."
  - "When the retry budget is exhausted, only replace the final error with group fallback status/message if the upstream error is itself a stream first-byte timeout."
patterns-established:
  - "Current-group strategy resolution is centralized in relay/common/group_strategy.go."
  - "Cross-group retry switching uses the same effective retry ceiling helper as the main relay loop."
requirements-completed: [GSET-04, GSET-05, GSET-06, GSET-07]
duration: 45min
completed: 2026-06-11
---

# Phase 5: Group Strategy Settings Summary

**Applied per-group retry and stream budget rules to the live relay path**

## Accomplishments
- Added a shared runtime resolver that snapshots the active group strategy into request context and exposes one effective retry ceiling for both relay execution and auto-group channel selection.
- Added request-scoped accounting for stream retry first-byte wait time and stopped retries when the accumulated retry wait reaches the configured group budget.
- Preserved upstream-error priority by only synthesizing the configured fallback HTTP status and message when the terminal error is itself a stream first-byte timeout.

## Verification
- `GOROOT=/Users/zf/.gvm/gos/go1.25.1 go test ./relay/common ./controller ./service -run 'Test.*Retry|Test.*GroupStrategy|Test.*FirstByte'`

## Notes
- The main relay loop no longer hard-wires `common.RetryTimes`; it re-resolves retry allowance from the current matched group after channel selection.
- Auto-group switching now compares per-group retry exhaustion against the same resolver, so `default` and `vip` can advance with different retry ceilings in one request.

