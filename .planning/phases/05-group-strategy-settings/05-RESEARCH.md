# Phase 5: Group Strategy Settings - Research

**Date:** 2026-06-11  
**Phase:** 5 - Group Strategy Settings  
**Status:** Complete

## Objective

Research how to implement per-group relay strategy controls in Operations Settings without breaking existing retry, timeout, and cross-group selection behavior.

## Codebase Findings

### 1. Group source is already stable

- `setting/ratio_setting/group_ratio.go` stores group keys in `groupRatioMap`.
- `GroupRatio2JSONString()` and `UpdateGroupRatioByJSONString()` already define the authoritative serialized shape.
- This matches the locked decision to source strategy groups from `options.GroupRatio`.

Implication:
- The new strategy editor should derive its group list from `GroupRatio`, not from users, tokens, or channels.

### 2. Global retry is currently hard-wired

- `controller/relay.go` uses `for ; retryParam.GetRetry() <= common.RetryTimes; ...`
- `shouldRetry(...)` also receives remaining retries derived from `common.RetryTimes`.
- `service/channel_select.go` uses `common.RetryTimes` inside `auto` / cross-group retry switching logic.

Implication:
- Per-group retry override cannot be done in one place only.
- Both the outer controller retry loop and the auto-group channel selection logic must resolve the effective retry limit from the current group.

### 3. Current group can change during retries

- `service/channel_select.go` updates context keys such as:
  - `ContextKeyAutoGroup`
  - `ContextKeyAutoGroupIndex`
  - `ContextKeyAutoGroupRetryIndex`
- `relay/common/relay_info.go` already carries `UsingGroup`.

Implication:
- The product decision "re-resolve by current group on retry switch" is implementable with current plumbing.
- The runtime should store a request-level strategy snapshot per retry attempt, not once at request start.

### 4. Stream first-byte timeout groundwork already exists

- `relay/common/timeout.go` already defines:
  - `ErrStreamFirstByteTimeout`
  - `ResolveStreamFirstByteTimeoutSeconds(...)`
  - first-byte timeout metadata helpers
- Phase 1 already separated first-byte waiting from total stream duration.

Implication:
- Phase 5 should not invent a second stream timeout mechanism.
- It should layer a request-level cumulative first-byte retry budget on top of the existing per-attempt first-byte timeout path.

### 5. Settings storage path is straightforward

- `model/option.go` initializes and syncs flat option keys.
- `controller/option.go` validates special option keys before persistence.
- `web/default/src/features/system-settings/api.ts` and `useUpdateOption` already support generic option updates.

Implication:
- `group_strategy_settings` should be added as a new option key with backend validation and typed frontend parsing.
- No schema migration is needed if stored as an option row.

### 6. Frontend patterns are reusable

- `operations/section-registry.tsx` controls section order for Operations Settings.
- `general/system-behavior-section.tsx` shows the expected save pattern and field density.
- `models/group-ratio-visual-editor.tsx` is a strong reference for structured editing of group-keyed settings.

Implication:
- The new UI should be a structured repeated-row editor, not raw JSON.
- Existing settings page primitives are enough; no new framework or page shell is required.

## Recommended Runtime Model

### Strategy data shape

Recommended option key:
- `group_strategy_settings`

Recommended serialized shape:

```json
{
  "default": {
    "enabled": true,
    "stream_retry_first_byte_budget_seconds": 15,
    "retry_times": 2,
    "timeout_http_status": 503,
    "timeout_error_message": "资源繁忙，请稍后尝试"
  },
  "vip": {
    "enabled": true,
    "stream_retry_first_byte_budget_seconds": 30,
    "retry_times": 4,
    "timeout_http_status": 503,
    "timeout_error_message": "资源繁忙，请稍后尝试"
  }
}
```

Notes:
- Missing group key => no group strategy => fall back to existing global behavior.
- Disabled entry can be treated the same as absent for runtime resolution, but should still be preserved for UI editing.

### Budget accumulator recommendation

Recommended model:
- Keep one request-level accumulator for total stream first-byte wait already spent.
- On each retry:
  1. resolve current group from runtime context
  2. resolve current group's strategy
  3. compare accumulated spent seconds against that strategy's budget
  4. if remaining budget is exhausted, stop retrying and synthesize the group strategy response unless a clearer upstream error should win
  5. otherwise run the next retry attempt and add the actual first-byte wait spent by that attempt

Why this is the best fit:
- It honors the user decision that strategy is matched by current group on each retry.
- It keeps one understandable request-level budget instead of one independent timer per group.
- It allows switching into a group with a larger budget to continue, or into a tighter group to fail sooner, which matches the idea that each retry uses the active group's policy.

### Retry override recommendation

Recommended helper:
- resolve effective retry times from current group strategy first, else `common.RetryTimes`

This helper should be shared by:
- `controller/relay.go`
- any task-relay loop if Phase 5 chooses to extend the same behavior there later
- `service/channel_select.go` when deciding whether current auto-group retries are exhausted

## Risks

### 1. Split-brain retry accounting

Risk:
- The controller loop and `service/channel_select.go` could calculate different retry ceilings if they each partially reimplement the strategy rules.

Mitigation:
- Use one backend resolver function for effective retry policy.

### 2. Budget counted incorrectly

Risk:
- Developers may accidentally count total stream duration or downstream flush time rather than first-byte wait time.

Mitigation:
- Budget accounting must be anchored to the same point where `ErrStreamFirstByteTimeout` semantics are already defined.

### 3. Fallback confusion in UI

Risk:
- Operators may assume blank fields mean disabled feature rather than fallback to global behavior.

Mitigation:
- UI must render explicit fallback labels and precedence notes inline.

### 4. JSON validation drift

Risk:
- Backend accepts malformed strategy JSON while frontend emits a stricter shape, or vice versa.

Mitigation:
- Define one exact backend struct for persistence validation and mirror it in frontend typed parsing.

## Recommended Verification

### Backend

- Unit test strategy JSON validation and fallback behavior.
- Unit test effective retry resolution for:
  - matching group strategy
  - missing group strategy
  - disabled group strategy
- Unit test stream budget exhaustion behavior with accumulated first-byte waits.
- Test group switch behavior across retries.

### Frontend

- Typecheck the new Operations Settings section.
- Verify section registry ordering.
- Verify fallback labels and operator descriptions are present in rendered code.

### Commands

- `go test ./controller ./service ./setting/... ./model/... ./relay/common/...`
- `cd web/default && bun run typecheck`
- `cd web/default && bun run i18n:sync`

## Recommendation Summary

1. Implement a dedicated `group_strategy_settings` option with strict backend validation.
2. Add a single runtime resolver for current-group strategy and effective retry limit.
3. Track one request-level stream first-byte retry budget accumulator.
4. Build the UI as a structured group-row editor in Operations Settings, immediately after System Behavior.
5. Make precedence and fallback visible in the UI, not hidden in docs only.

## RESEARCH COMPLETE
