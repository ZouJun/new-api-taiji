# Phase 1: AWS Claude Timeout Audit - Context

**Gathered:** 2026-06-07
**Status:** Ready for planning

<domain>
## Phase Boundary

This phase determines the current AWS Claude timeout behavior and locks the implementation direction for channel-level dynamic timeout control. The scope is limited to understanding and shaping timeout behavior for AWS Claude relay calls, not full payload archival or customer Trace-Id rollout.

</domain>

<decisions>
## Implementation Decisions

### Timeout Configuration Shape
- **D-01:** Channel-level timeout must be configured per channel record, not only through the global `common.RelayTimeout`.
- **D-02:** The timeout setting should be stored in the existing `channel.setting` JSON via `dto.ChannelSettings`, not as a new physical DB column in `channels`.
- **D-03:** Recommended field shape is a pointer scalar, for example `RelayTimeoutSeconds *int \`json:"relay_timeout_seconds,omitempty"\``, so absence means "inherit global default" and explicit values are preserved.
- **D-04:** `0` or negative values should not mean "infinite timeout" at the channel level. Treat them as invalid or as fallback-to-global during validation to avoid ambiguous production behavior.

### Effective Timeout Resolution
- **D-05:** Introduce a single helper to resolve effective timeout for a request in this precedence order:
  1. channel setting `relay_timeout_seconds`
  2. global `common.RelayTimeout`
  3. zero means "no explicit timeout configured"
- **D-06:** The effective timeout must be derived from `relaycommon.RelayInfo.ChannelSetting`, because distribution already injects `ChannelSettings` into Gin context and then into `RelayInfo`.

### AWS Invocation Behavior
- **D-07:** Replace `newAwsInvokeContext()` with a request-aware variant that uses `c.Request.Context()` as the parent, not `context.Background()`, so client cancellation and upstream timeout share the same tree.
- **D-08:** Non-streaming AWS Claude calls should apply the effective timeout to the invocation context.
- **D-09:** Streaming AWS Claude calls need separate analysis from non-streaming calls because a full-request timeout can incorrectly kill long-running streams. Planning must explicitly evaluate whether the timeout governs:
  - request establishment only,
  - full stream lifetime,
  - or idle stream gaps only.
- **D-10:** The current global `http.Client.Timeout` is too coarse for channel-level dynamic timeout reuse. Planning should prefer timeout application through per-request context first, and only use client-level timeout where it is safe and intentional.

### HTTP Client Strategy
- **D-11:** Do not mutate the global shared `httpClient` per request.
- **D-12:** If client-level timeout override is still needed for some paths, implement it through a helper that returns a client keyed by `(proxy, timeout)` or a safe cloned client, rather than overwriting a global singleton.
- **D-13:** Proxy-enabled paths must preserve per-channel proxy behavior while still honoring channel-level timeout.

### Verification Scope
- **D-14:** Phase 1 must prove current behavior with focused tests or documented verification, not just static code reading.
- **D-15:** Verification should cover at least:
  - no channel timeout configured -> falls back to global timeout behavior
  - channel timeout configured -> AWS path uses channel timeout
  - streaming path behavior is explicitly documented, including any known mismatch between desired and current semantics

### the agent's Discretion
- Field name can be `relay_timeout_seconds` or `request_timeout_seconds`, but it must live in `dto.ChannelSettings`, be pointer-typed, and be clearly documented as seconds.
- Validation location can be in channel setting validation, request-time resolver, or both, as long as invalid values are rejected consistently.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase and requirements
- `.planning/ROADMAP.md` — Phase 1 goal, success criteria, and plan breakdown
- `.planning/REQUIREMENTS.md` — `AWS-01` through `AWS-04` define this phase's deliverables
- `.planning/PROJECT.md` — milestone context and the 8000-15000 RPM constraint
- `.planning/STATE.md` — current project position

### Existing timeout and channel configuration code
- `model/channel.go` — `Channel`, `GetSetting`, and `SetSetting`; this is the preferred storage path for per-channel timeout
- `dto/channel_settings.go` — existing `ChannelSettings` definition that should gain the timeout field
- `middleware/distributor.go` — injects `ChannelSettings` into request context
- `relay/common/relay_info.go` — `RelayInfo.ChannelSetting` carries per-channel settings into relay handlers
- `service/http_client.go` — current shared/proxy HTTP client timeout behavior
- `relay/channel/aws/relay-aws.go` — current AWS client creation and `newAwsInvokeContext` behavior
- `relay/helper/stream_scanner.go` — existing stream timeout/idle handling, relevant when separating stream semantics from request timeout

### Planning context
- `AGENTS.md` — project constraints: cross-DB support, JSON wrapper rule, preserve explicit zero values

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `model.Channel.GetSetting()` / `SetSetting()` already provide a per-channel JSON-backed setting path without schema migration.
- `middleware/distributor.go` already puts `dto.ChannelSettings` into request context, so the timeout can flow through existing context plumbing.
- `relaycommon.RelayInfo.InitChannelMeta()` already copies `ChannelSetting` into `RelayInfo`.

### Established Patterns
- Channel-specific behaviors such as `Proxy`, `PassThroughBodyEnabled`, and system prompt controls already live in `dto.ChannelSettings`.
- AWS relay code already distinguishes streaming and non-streaming execution paths in `relay/channel/aws/relay-aws.go`.
- The project uses global timeout env vars today (`common.RelayTimeout` and `constant.StreamingTimeout`), so the new channel timeout must integrate without silently changing unrelated providers.

### Integration Points
- Add the timeout field in `dto/channel_settings.go`.
- Resolve effective timeout from `RelayInfo.ChannelSetting` in a helper reachable from relay handlers.
- Update AWS timeout context creation in `relay/channel/aws/relay-aws.go`.
- If necessary, extend `service/http_client.go` with a timeout-aware client accessor that does not mutate shared state.

</code_context>

<specifics>
## Specific Ideas

- The user explicitly wants timeout to be dynamically controlled per channel record.
- The recommended implementation is to store that setting in the existing `channel.setting` JSON instead of adding a new `channels.timeout` column.
- This phase should produce a concrete explanation of current timeout behavior and a locked implementation direction for per-channel timeout resolution.

</specifics>

<deferred>
## Deferred Ideas

Full request/response archival and customer `Trace-Id` propagation are deferred to later phases by roadmap design.

</deferred>

---

*Phase: 1-AWS Claude Timeout Audit*
*Context gathered: 2026-06-07*
