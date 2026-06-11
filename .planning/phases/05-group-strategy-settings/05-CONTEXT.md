# Phase 5: Group Strategy Settings - Context

**Gathered:** 2026-06-11
**Status:** Ready for planning

<domain>
## Phase Boundary

This phase adds group-level relay strategy controls in dashboard Operations Settings. It covers how operators configure per-group streaming first-byte retry budget, per-group retry counts, and the timeout-budget HTTP response. It also locks the storage shape, runtime resolution rules, and the level of operator guidance required in the UI. The scope does not include changing pricing semantics, inventing a new group source, or replacing the existing relay retry architecture.

</domain>

<decisions>
## Implementation Decisions

### Strategy Group Source
- **D-01:** Strategy groups must be derived from the keys in the `options` table entry `GroupRatio`, for example `default` and `vip`.
- **D-02:** Do not introduce a separate "strategy group" concept. The runtime must reuse the existing group namespace already exposed through `GroupRatio`.
- **D-03:** If a request changes group during retries, strategy lookup must use the current effective group for that retry attempt, not a request-start snapshot.

### Storage and Fallback
- **D-04:** Store the new configuration in a dedicated option key, recommended as `group_strategy_settings`.
- **D-05:** Do not embed strategy configuration into `GroupRatio`; pricing configuration and runtime strategy configuration must stay separate.
- **D-06:** If the current group has no strategy entry, do not force an error and do not synthesize a partial strategy. Fall back to the existing global behavior: current global retry settings and current timeout logic.

### Retry Count Semantics
- **D-07:** The per-group retry value means extra retries only. The first request does not count toward this value.
- **D-08:** When a group strategy is matched, its retry count overrides global `RetryTimes`.
- **D-09:** The per-group retry count applies to both streaming and non-streaming requests.
- **D-10:** The retry control here is cross-channel retry at the New API relay layer, not AWS SDK internal retry attempts.

### Streaming Budget Semantics
- **D-11:** The streaming budget is measured in seconds.
- **D-12:** The budget counts the sum of time spent waiting for the first byte/event across retry attempts.
- **D-13:** This budget applies only to streaming first-byte waiting. It does not become a total full-stream duration cap after the first byte arrives.
- **D-14:** Because group resolution is retry-level dynamic, the planner must define how the accumulated first-byte budget is tracked when retries cross from one `GroupRatio` key to another. The locked product rule is that each retry attempt resolves strategy from the current group; the implementation detail of accumulator ownership is left to planning as long as operator-facing semantics remain explainable.

### Error Response Rules
- **D-15:** Each group strategy can configure an HTTP status to use when the streaming retry first-byte budget is exhausted.
- **D-16:** Each group strategy can configure a custom error message for the same condition.
- **D-17:** If no custom message is configured, the default error message is `资源繁忙，请稍后尝试`.
- **D-18:** If the upstream original error is more specific, it has higher priority than the group-strategy timeout-budget response.

### Operations UI Requirements
- **D-19:** The Operations Settings page must add a new section/module named `策略设置`.
- **D-20:** The section should appear after the current general/system behavior settings area. The user expectation is "between 通用设置 and 顶栏管理"; planning must reconcile that wording with the current section registry and preserve an intuitive order in the actual Operations Settings navigation.
- **D-21:** Every new field must include operator-facing explanation text covering: what the field does, when to use it, how it falls back, and what takes precedence over it.
- **D-22:** The explanations must be understandable to operators with limited technical depth, not just engineers.
- **D-23:** The page should proactively explain the difference between:
  - group strategy retry count vs global `RetryTimes`
  - relay-layer retry count vs AWS SDK retry attempts
  - streaming first-byte retry budget vs single-channel timeout settings from Phase 1

### Recommended Field Set
- **D-24:** The planner should start from this field set in the new section:
  - target group
  - enabled flag
  - stream retry first-byte budget in seconds
  - retry times
  - timeout HTTP status
  - timeout error message
- **D-25:** The UI should expose fallback behavior clearly when a group is not configured, rather than making operators infer it from missing data.

### Verification Scope
- **D-26:** Verification must cover both backend behavior and operator usability:
  - strategy option persistence and reload
  - runtime strategy resolution by current group
  - fallback to global behavior when group strategy is absent
  - retry-count override behavior for streaming and non-streaming requests
  - streaming first-byte retry budget exhaustion behavior
  - upstream-original-error priority over synthetic timeout-budget response
  - Operations Settings copy clarity and field ordering

### the agent's Discretion
- The exact JSON schema for `group_strategy_settings` is open to planning, as long as it is stable, explainable, and easy to validate.
- The exact UI control style can follow existing system-settings patterns, but should favor structured editors over raw JSON if the current codebase supports it cleanly.
- The planner may choose whether the section is a single form, table editor, or accordion-based editor, provided the operator guidance remains strong.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase and requirements
- `.planning/ROADMAP.md` — Phase 5 goal, success criteria, and plan breakdown
- `.planning/REQUIREMENTS.md` — `GSET-01` through `GSET-07` define this phase's deliverables
- `.planning/PROJECT.md` — milestone context and non-negotiable operational constraints
- `.planning/STATE.md` — current roadmap position and prior phase context
- `.planning/phases/01-aws-claude-timeout-audit/01-CONTEXT.md` — locked timeout semantics from Phase 1 that this phase must build on, not redefine

### Option storage and option update flow
- `model/option.go` — option map initialization, DB sync, `UpdateOption`, and `UpdateOptionsBulk`
- `controller/option.go` — management API validation and update flow for system options
- `web/default/src/features/system-settings/api.ts` — frontend system-settings fetch/update API
- `web/default/src/features/system-settings/types.ts` — typed shape for Operations Settings data

### Group and retry runtime behavior
- `service/channel_select.go` — current retry/group switching logic, especially `auto` group and cross-group retry behavior
- `controller/relay.go` — relay retry loop using global `RetryTimes`
- `model/token.go` — token-level `cross_group_retry` semantics
- `relay/common/relay_info.go` — current relay metadata shape, including `UsingGroup`

### Timeout behavior inherited from Phase 1
- `relay/common/timeout.go` — current timeout resolution helpers and stream first-byte timeout behavior
- `dto/channel_settings.go` — existing per-channel timeout-related settings; this phase must explain its relationship to them
- `model/channel.go` — channel setting validation for timeout-related fields

### Operations Settings frontend patterns
- `web/default/src/features/system-settings/operations/index.tsx` — Operations Settings route and default data
- `web/default/src/features/system-settings/operations/section-registry.tsx` — current Operations Settings section order and composition
- `web/default/src/features/system-settings/general/system-behavior-section.tsx` — existing section style for retry-related fields and operator descriptions
- `web/default/src/features/system-settings/components/settings-section.tsx` — shared section container
- `web/default/src/features/system-settings/hooks/use-update-option.ts` — option mutation hook used across system settings

### Codebase constraints and testing
- `AGENTS.md` — project constraints: cross-DB support, JSON wrapper rule, preserve explicit zero values where request DTOs need them
- `.planning/codebase/CONVENTIONS.md` — system-settings frontend and backend conventions
- `.planning/codebase/STRUCTURE.md` — location of relay, settings, controller, and frontend modules
- `.planning/codebase/TESTING.md` — expected backend/frontend verification commands and relevant coverage style

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `model.UpdateOption` / `model.UpdateOptionsBulk`: existing persistence path for a new independent `option` key.
- `controller.UpdateOption`: existing generic admin option update API, suitable if the new option can be validated there.
- `web/default/src/features/system-settings/operations/section-registry.tsx`: existing place to insert a new Operations Settings section in a predictable order.
- `web/default/src/features/system-settings/general/system-behavior-section.tsx`: established pattern for concise settings forms with descriptions and save actions.
- `relay/common/timeout.go`: existing stream first-byte timeout language and helpers that should anchor operator explanation text.

### Established Patterns
- System settings are stored as flat option keys and edited via `/api/option/`.
- Operations Settings sections are built through a registry rather than ad hoc route wiring.
- Retry behavior today is controlled globally through `common.RetryTimes`, and group switching is handled inside `service/channel_select.go`.
- Existing timeout work already distinguishes non-stream total timeout from stream first-byte timeout; this phase must not blur that distinction.

### Integration Points
- Add a new option key for group strategy configuration, and load/validate it through the option pipeline.
- Extend the runtime retry path so effective retry limits can be resolved per current group during retries.
- Add a runtime mechanism to accumulate stream first-byte waiting time across retry attempts.
- Add a new Operations Settings section and corresponding typed defaults so the UI can edit and explain the per-group strategy map.
- Ensure planner explicitly decides where to store parsed in-memory strategy data so runtime reads do not repeatedly parse raw JSON on the hot path.

</code_context>

<specifics>
## Specific Ideas

- Operators should be able to understand the priority chain at a glance:
  1. upstream original error
  2. current group strategy
  3. existing global retry/timeout behavior when no group strategy exists
- The UI should likely show examples such as `default` and `vip`, because those names come directly from `GroupRatio`.
- The section copy should call out that this feature is for different operational classes of traffic, not for billing-price management.
- The page should explain that the streaming budget is about "waiting for the first response chunk across retries", not "entire stream duration".

</specifics>

<deferred>
## Deferred Ideas

- A richer global default strategy object that replaces legacy global retry/timeout knobs was not chosen in this discussion. This phase keeps fallback behavior on the existing global settings.
- No new strategy-group namespace was introduced; the system continues to depend on `GroupRatio` keys.

</deferred>

---

*Phase: 5-Group Strategy Settings*
*Context gathered: 2026-06-11*
