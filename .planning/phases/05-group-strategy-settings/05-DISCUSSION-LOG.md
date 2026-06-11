# Phase 5: Group Strategy Settings - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-11
**Phase:** 5-Group Strategy Settings
**Areas discussed:** Strategy group source, fallback behavior, streaming budget semantics, retry semantics, error response rules, storage shape

---

## Strategy Group Source

| Option | Description | Selected |
|--------|-------------|----------|
| GroupRatio keys | Read group names from the keys of the `GroupRatio` option, such as `default` and `vip` | ✓ |
| User groups | Use user-group concepts as the strategy source | |
| Separate strategy group namespace | Introduce a new group namespace just for strategy settings | |

**User's choice:** Read groups from the keys in `options.GroupRatio`, for example `default` and `vip`.
**Notes:** The user explicitly rejected inventing another group layer.

---

## Strategy Fallback

| Option | Description | Selected |
|--------|-------------|----------|
| Global default strategy object | Missing group strategy falls back to a new dedicated global strategy | |
| Existing global behavior | Missing group strategy falls back to current global `RetryTimes` and current timeout logic | ✓ |
| Hard error | Missing group strategy causes an error | |

**User's choice:** Fall back to existing global behavior.
**Notes:** The user wants this feature to remain additive rather than mandatory for every group.

---

## Strategy Match Timing

| Option | Description | Selected |
|--------|-------------|----------|
| Request-start snapshot | Resolve strategy once at request start and keep it fixed | |
| Current retry group | Re-resolve strategy when retries switch to another group | ✓ |
| Token initial group | Keep using the token's original group even after retry switching | |

**User's choice:** Re-resolve by the current group when retries switch groups.
**Notes:** This directly affects `auto` and cross-group retry behavior.

---

## Streaming Budget Semantics

| Option | Description | Selected |
|--------|-------------|----------|
| First-byte wait sum | Budget counts the sum of each retry's wait for first byte/event | ✓ |
| Total stream duration | Budget counts full end-to-end stream lifetime | |
| Per-channel timeout mirror | Budget behaves the same as existing per-channel stream timeout | |

**User's choice:** Count the sum of first-byte waiting time across retries.
**Notes:** The user later chose seconds as the operator-facing unit.

---

## Retry Count Semantics

| Option | Description | Selected |
|--------|-------------|----------|
| Extra retries only | First request does not count; configured value overrides global `RetryTimes` | ✓ |
| Total attempts | First request is included in the configured count | |
| Layered with global | Group retry count adds on top of global `RetryTimes` | |

**User's choice:** Extra retries only; first request does not count; matched group strategy overrides global `RetryTimes`.
**Notes:** The user also locked that this applies to both streaming and non-streaming requests, and that it refers to relay-layer retries rather than AWS SDK retries.

---

## Error Response Rules

| Option | Description | Selected |
|--------|-------------|----------|
| HTTP status + per-group message | Each group configures HTTP status and optional message | ✓ |
| Business code only | Configure only an application code field | |
| Fixed global response | Use one response for all groups | |

**User's choice:** Configure HTTP status and per-group message.
**Notes:** Default message is `资源繁忙，请稍后尝试`. If the upstream original error is more specific, it wins.

---

## Storage Shape

| Option | Description | Selected |
|--------|-------------|----------|
| Independent option key | Store strategy configuration in a new dedicated option such as `group_strategy_settings` | ✓ |
| Inline in GroupRatio | Extend the existing `GroupRatio` payload with strategy fields | |
| New DB table | Store strategies in a dedicated table | |

**User's choice:** Use an independent option key.
**Notes:** This keeps pricing configuration separate from runtime relay strategy.

---

## Operations UI Guidance

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal labels only | Add fields with short labels and no deep guidance | |
| Operator-first guidance | Add detailed explanations, scenarios, and precedence notes for each field | ✓ |
| JSON-only editor | Expose raw JSON and expect operators to know the structure | |

**User's choice:** Operator-first guidance.
**Notes:** The user explicitly wants each new field to explain usage scenario, priority/fallback, and practical meaning for less technical operators.

## the agent's Discretion

- Exact JSON schema for `group_strategy_settings`
- Exact UI structure and control composition
- Exact default HTTP status value, with `503` suggested and accepted without objection

## Deferred Ideas

- None — discussion stayed within phase scope.
