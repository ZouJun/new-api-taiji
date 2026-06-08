# Phase 1: Channel Timeout Control and AWS SDK Governance - Context

**Gathered:** 2026-06-07
**Status:** Ready for planning

<domain>
## Phase Boundary

This phase determines the current timeout behavior across relay paths and locks the implementation direction for channel-level dynamic timeout control, streaming first-byte timeout, retry compatibility, and AWS SDK governance. The scope still excludes full payload archival and customer Trace-Id rollout.

</domain>

<decisions>
## Implementation Decisions

### Timeout Configuration Shape
- **D-01:** Channel-level timeout must be configured per channel record, not only through the global `common.RelayTimeout`.
- **D-02:** The timeout setting should be stored in the existing `channel.setting` JSON via `dto.ChannelSettings`, not as a new physical DB column in `channels`.
- **D-03:** Channel timeout is split into two pointer fields in `dto.ChannelSettings`:
  - `NonStreamTimeoutSeconds *int \`json:"non_stream_timeout_seconds,omitempty"\``
  - `StreamFirstByteTimeoutSeconds *int \`json:"stream_first_byte_timeout_seconds,omitempty"\``
- **D-04:** `0` or negative values should not mean "infinite timeout" at the channel level. Treat them as invalid or as fallback-to-global during validation to avoid ambiguous production behavior.
- **D-04a:** Once a channel-level timeout is present in `channel.setting`, the request path must actually enforce it. This cannot be a stored-but-ignored configuration.

### Effective Timeout Resolution
- **D-05:** Introduce a single helper to resolve effective timeout for a request in this precedence order:
  1. channel setting timeout
  2. provider-specific global timeout default
  3. legacy `common.RelayTimeout`
  4. zero means "no explicit timeout configured"
- **D-06:** The effective timeout must be derived from `relaycommon.RelayInfo.ChannelSetting`, because distribution already injects `ChannelSettings` into Gin context and then into `RelayInfo`.
- **D-06a:** If `channel.setting.non_stream_timeout_seconds` or `channel.setting.stream_first_byte_timeout_seconds` is set, live request execution must use that effective timeout instead of silently falling back to the global timeout path.

### AWS Invocation Behavior
- **D-07:** Replace `newAwsInvokeContext()` with a request-aware variant that uses `c.Request.Context()` as the parent, not `context.Background()`, so client cancellation and upstream timeout share the same tree.
- **D-08:** Non-streaming AWS Claude calls should apply the effective timeout to the invocation context.
- **D-09:** Streaming calls use first-byte timeout semantics, not full-stream total timeout semantics. Once the first upstream stream event arrives, the first version does not enforce a separate total stream timeout kill switch.
- **D-10:** The current global `http.Client.Timeout` is too coarse for channel-level dynamic timeout reuse. Planning should prefer timeout application through per-request context first, and only use client-level timeout where it is safe and intentional.

### HTTP Client Strategy
- **D-11:** Do not mutate the global shared `httpClient` per request.
- **D-12:** Preserve HTTP connection pooling by reusing shared transports. Do not degrade into one-client-per-request construction just to honor channel-level timeout settings.
- **D-13:** Proxy-enabled paths must preserve per-channel proxy behavior while still honoring channel-level timeout.
- **D-13a:** Recommended control split:
  - non-stream timeout -> request context timeout
  - stream first-byte timeout -> stream establishment / first-event timeout
  - `http.Client.Timeout` -> global/provider default guardrail, not the primary per-channel streaming control

### Retry and Logging
- **D-14:** Timeout failures must remain compatible with the existing retry mechanism and should be treated like other retry-eligible upstream failures unless current retry rules explicitly skip them.
- **D-15:** Timeout failures must emit structured metadata for troubleshooting, including timeout type, timeout source, effective timeout seconds, stream flag, and retry index.

### AWS SDK Governance
- **D-16:** Expose AWS timeout-related controls globally from New API layer:
  - `aws_http_client_non_stream_timeout_seconds`
  - `aws_http_client_stream_first_byte_timeout_seconds`
  - `aws_invoke_timeout_seconds`
- **D-17:** Allow selected AWS Claude-related knobs to be overridden per channel through `channel.setting`, with higher precedence than global AWS defaults.
- **D-18:** Recommended first-version per-channel AWS overrides:
  - `aws_http_client_non_stream_timeout_seconds`
  - `aws_http_client_stream_first_byte_timeout_seconds`
  - `aws_invoke_timeout_seconds`

### Verification Scope
- **D-19:** Phase 1 must prove current behavior with focused tests or documented verification, not just static code reading.
- **D-20:** Verification should cover at least:
  - no channel timeout configured -> falls back to global timeout behavior
  - channel timeout configured -> non-stream path uses channel timeout
  - stream-first-byte timeout is enforced before first event
  - timeout failures remain retry-compatible
  - AWS global and channel override precedence is documented and testable
- **D-20a:** Verification must explicitly reject the failure mode where `channel.setting` contains a timeout value but the live request still behaves as if only global timeout exists.

### the agent's Discretion
- Validation location can be in channel setting validation, request-time resolver, or both, as long as invalid values are rejected consistently.
- `AWS_SDK_MAX_ATTEMPTS` and `AWS_SDK_RETRY_MODE` remain on prior behavior in this phase; this phase only adds dynamic timeout-related control.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase and requirements
- `.planning/ROADMAP.md` — Phase 1 goal, success criteria, and plan breakdown
- `.planning/REQUIREMENTS.md` — `AWS-*`, `TIME-*`, and `SDK-*` define this phase's deliverables
- `.planning/PROJECT.md` — milestone context and the 8000-15000 RPM constraint
- `.planning/STATE.md` — current project position

### Existing timeout and channel configuration code
- `model/channel.go` — `Channel`, `GetSetting`, and `SetSetting`; this is the preferred storage path for per-channel timeout
- `dto/channel_settings.go` — existing `ChannelSettings` definition that should gain the timeout field
- `middleware/distributor.go` — injects `ChannelSettings` into request context
- `relay/common/relay_info.go` — `RelayInfo.ChannelSetting` carries per-channel settings into relay handlers
- `service/http_client.go` — current shared/proxy HTTP client timeout behavior and connection pool reuse points
- `relay/channel/aws/relay-aws.go` — current AWS client creation and `newAwsInvokeContext` behavior
- `relay/channel/api_request.go` — shared HTTP request execution path used by many channels
- `relay/helper/stream_scanner.go` — existing stream timeout/idle handling, relevant when separating stream semantics from request timeout
- `controller/relay.go` — retry loop and channel error handling

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
- The project uses global timeout env vars today (`common.RelayTimeout` and `constant.StreamingTimeout`), so the new timeout system must integrate without silently changing unrelated providers.
- The common HTTP relay path currently selects either `service.GetHttpClient()` or `service.NewProxyHttpClient(proxy)` and then calls `client.Do(req)`, so channel-level timeout design must plug into this shared path without breaking pooling.

### Integration Points
- Add the timeout field in `dto/channel_settings.go`.
- Resolve effective timeout from `RelayInfo.ChannelSetting` in a helper reachable from relay handlers.
- Update AWS timeout context creation in `relay/channel/aws/relay-aws.go`.
- If necessary, extend `service/http_client.go` with a timeout-aware client accessor that does not mutate shared state.

</code_context>

<specifics>
## Specific Ideas

- The user explicitly wants timeout to be dynamically controlled per channel record.
- The user explicitly requires that any timeout configured in `channel.setting` must have a real enforcement path.
- The timeout setting is now locked as two separate fields in `channel.setting`: non-stream total timeout and stream first-byte timeout.
- The user explicitly requires timeout failures to remain compatible with retries.
- The user explicitly requires AWS SDK global controls and selected per-channel AWS Claude overrides.
- This phase should produce a concrete explanation of current timeout behavior and a locked implementation direction for cross-channel timeout resolution and AWS SDK governance.

</specifics>

<deferred>
## Deferred Ideas

Full request/response archival and customer `Trace-Id` propagation are deferred to later phases by roadmap design.

</deferred>

---

*Phase: 1-Channel Timeout Control and AWS SDK Governance*
*Context gathered: 2026-06-07*
