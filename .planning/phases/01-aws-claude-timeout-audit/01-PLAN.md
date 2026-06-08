# Phase 1 Plan: Channel Timeout Control and AWS SDK Governance

**Phase:** 1  
**Status:** Drafted for execution  
**Updated:** 2026-06-08

## 1. Goal

Build a timeout control system that works across channels, separates non-stream and stream-first-byte behavior, preserves retry compatibility, keeps HTTP connection pooling, and exposes AWS SDK timeout controls with selected per-channel AWS Claude overrides.

This phase is no longer only an AWS audit. It is the foundation for bounded upstream behavior under 8000-15000 RPM.

## 2. Locked Decisions

These points are already confirmed and should not be reopened during implementation unless the user changes direction:

1. Per-channel timeout stays in `channel.setting`, not a new physical DB column.
2. Timeout is split into two `channel.setting` fields:
   - `non_stream_timeout_seconds`
   - `stream_first_byte_timeout_seconds`
3. Streaming timeout means waiting for the first upstream byte/event, not total stream duration.
4. After the first upstream stream event arrives, v1 does not enforce a total stream kill timeout.
5. Timeout failures must remain compatible with the existing retry mechanism.
6. All channels should honor channel timeout configuration when present; otherwise they fall back to defaults.
7. HTTP connection pooling must be preserved.
8. AWS global timeout controls must expose:
   - `http client timeout`
   - `invoke timeout`
9. Selected AWS Claude timeout-related knobs must also support channel-level override.

## 3. Proposed Configuration Model

## 3.1 Channel-level settings

Add to `dto.ChannelSettings`:

- `NonStreamTimeoutSeconds *int    json:"non_stream_timeout_seconds,omitempty"`
- `StreamFirstByteTimeoutSeconds *int    json:"stream_first_byte_timeout_seconds,omitempty"`
- `AwsHTTPClientNonStreamTimeoutSeconds *int    json:"aws_http_client_non_stream_timeout_seconds,omitempty"`
- `AwsHTTPClientStreamFirstByteTimeoutSeconds *int    json:"aws_http_client_stream_first_byte_timeout_seconds,omitempty"`
- `AwsInvokeTimeoutSeconds *int    json:"aws_invoke_timeout_seconds,omitempty"`

Why pointer fields:

- `nil` means inherit from global/provider default
- explicit value means override

## 3.2 New global timeout config

Add new global config instead of relying only on legacy `common.RelayTimeout`.

Recommended global settings:

- `RELAY_DEFAULT_NON_STREAM_TIMEOUT`
- `RELAY_DEFAULT_STREAM_FIRST_BYTE_TIMEOUT`
- `AWS_HTTP_CLIENT_NON_STREAM_TIMEOUT_SECONDS`
- `AWS_HTTP_CLIENT_STREAM_FIRST_BYTE_TIMEOUT_SECONDS`
- `AWS_INVOKE_TIMEOUT_SECONDS`

Notes:

- `common.RelayTimeout` remains a legacy fallback, not the preferred new control point.
- `constant.StreamingTimeout` remains existing infrastructure and should not be silently repurposed as the new per-channel stream-first-byte control.
- `AWS_SDK_MAX_ATTEMPTS` and `AWS_SDK_RETRY_MODE` stay on prior behavior in this phase.

## 3.3 Precedence

Timeout precedence:

1. channel-level setting
2. provider/global default
3. legacy fallback (`common.RelayTimeout`)
4. zero => no explicit timeout

AWS timeout precedence:

1. channel-level AWS override
2. global AWS config
3. legacy/common fallback where applicable

## 4. Architecture Strategy

## 4.1 Non-stream requests

Primary control:

- request-scoped `context.WithTimeout(...)`

This applies to:

- common HTTP relay path in `relay/channel/api_request.go`
- AWS SDK invoke path in `relay/channel/aws/relay-aws.go`

Reason:

- per-request context timeout is the safest way to support channel-level dynamic timeout without breaking connection pooling

## 4.2 Stream requests

Primary control:

- stream-first-byte timeout

Meaning:

- from request start until the first upstream stream event that can be forwarded to downstream

After first byte:

- do not kill the stream by total stream elapsed time in v1

## 4.3 HTTP connection pooling

Do not create one brand-new transport per request.

Recommended model:

- shared default transport for non-proxy traffic
- cached transport/client per proxy endpoint where needed
- request-scoped context timeout controls most per-channel behavior
- global/client-level timeout is only a guardrail

This keeps:

- keep-alive reuse
- lower handshake cost
- stable socket behavior
- lower allocation churn

## 5. Code Design

## 5.1 New common timeout resolver

Add a helper package or file in relay/common or service layer to resolve effective timeouts.

Recommended helpers:

- `ResolveNonStreamTimeoutSeconds(info *relaycommon.RelayInfo, provider string) int`
- `ResolveStreamFirstByteTimeoutSeconds(info *relaycommon.RelayInfo, provider string) int`
- `ResolveAwsHTTPClientNonStreamTimeoutSeconds(info *relaycommon.RelayInfo) int`
- `ResolveAwsHTTPClientStreamFirstByteTimeoutSeconds(info *relaycommon.RelayInfo) int`
- `ResolveAwsInvokeTimeoutSeconds(info *relaycommon.RelayInfo) int`

These helpers must:

- validate negative/invalid values
- record timeout source when needed
- avoid duplicating precedence logic across providers

## 5.2 Common HTTP relay path

Main file:

- `relay/channel/api_request.go`

Required changes:

1. Before `client.Do(req)`, derive request context for non-stream requests
2. For stream requests, derive a context for first-byte waiting only
3. Select or build HTTP client without mutating global client state
4. Keep proxy behavior intact

Recommended implementation shape:

- `prepareRelayRequestContext(c, info, req) (context.Context, cancel, timeoutMeta)`
- `GetHttpClientForRelay(info, isStream) (*http.Client, error)`

Important detail:

- `http.NewRequest(...)` currently does not use `c.Request.Context()` explicitly in this path
- the implementation should attach the resolved request context back to `req = req.WithContext(ctx)`

## 5.3 AWS-specific path

Main file:

- `relay/channel/aws/relay-aws.go`

Required changes:

1. Replace `newAwsInvokeContext()` with request-aware resolver
2. Parent context must be `c.Request.Context()`
3. Non-stream path uses effective invoke timeout
4. Stream path uses stream-first-byte timeout for stream establishment
5. AWS client creation must honor global AWS timeout settings and selected channel overrides

Recommended function split:

- `newAwsInvokeContext(c *gin.Context, info *relaycommon.RelayInfo, mode awsTimeoutMode) (...)`
- `newAwsClient(c *gin.Context, info *relaycommon.RelayInfo) (...)`
- `buildAwsClientOptions(c, info) (...)`

Where `awsTimeoutMode` is conceptually:

- `non_stream_invoke`
- `stream_first_byte`

## 5.4 HTTP client service layer

Main file:

- `service/http_client.go`

Required changes:

1. Keep shared pooled clients
2. Add a safe accessor for provider/global HTTP timeout variants
3. Avoid mutating a singleton timeout field per request

Recommended strategy:

- default shared client remains
- proxy clients remain cached by proxy URL
- if provider/global HTTP timeout needs a separate client variant, cache by stable key:
  - `(proxy, timeout, tlsMode)`

Important constraint:

- do not key clients by `channelId`, or cache size will grow with channel count

## 5.5 Stream-first-byte enforcement

Main files:

- `relay/channel/api_request.go`
- `relay/helper/stream_scanner.go`
- provider-specific stream entry points

Recommended v1 design:

1. first-byte timeout applies while waiting for response body/event start
2. once first event is read, mark first-byte success
3. post-first-byte path is not killed by stream total timeout

Implementation note:

- `relay/helper/stream_scanner.go` currently uses `constant.StreamingTimeout` for idle scanning behavior
- v1 should not destroy that behavior blindly
- instead, integrate the new per-channel stream-first-byte control before or at stream establishment

Recommended first-byte signal:

- first non-empty upstream stream event that is eligible to forward downstream

## 5.6 Retry compatibility

Main file:

- `controller/relay.go`

Current retry loop already exists.

Required behavior:

- timeout failures must still go through `processChannelError(...)`
- timeout failures must still be evaluated by `shouldRetry(...)`
- no timeout path should accidentally set `skip_retry` unless already intended

Implementation rule:

- treat timeout as ordinary upstream failure unless channel/provider-specific logic says otherwise

## 5.7 Timeout observability

Main files:

- `controller/relay.go`
- `model/log.go`
- `logger/logger.go` later in Phase 2 for trace enrichment, but Phase 1 should still add structured timeout metadata

Recommended error metadata fields:

- `timeout_type`
  - `non_stream_total`
  - `stream_first_byte`
- `timeout_source`
  - `channel_setting`
  - `provider_global`
  - `legacy_global`
- `timeout_seconds`
- `retry_index`
- `is_stream`
- `provider`
- `channel_id`
- `request_path`
- `upstream_stage`
  - `http_request`
  - `aws_invoke`
  - `stream_first_byte_wait`

## 6. Execution Waves

## Wave 1: Config model and resolvers

Files:

- `dto/channel_settings.go`
- `common/init.go`
- `common/constants.go`
- new timeout resolver helper file

Deliverables:

- new channel fields
- new global config fields
- precedence helper functions

## Wave 2: Common HTTP path

Files:

- `service/http_client.go`
- `relay/channel/api_request.go`

Deliverables:

- non-stream request context timeout
- pooled client selection strategy
- stream-first-byte control hook in shared HTTP path

## Wave 3: AWS path

Files:

- `relay/channel/aws/relay-aws.go`

Deliverables:

- request-aware invoke context
- global AWS timeout controls
- selected channel-level AWS overrides

## Wave 4: Logging and retry verification

Files:

- `controller/relay.go`
- `model/log.go`
- tests

Deliverables:

- structured timeout metadata
- retry compatibility proof

## 7. Testing Plan

## 7.1 Resolver tests

Cover:

- channel timeout present
- only provider/global timeout present
- only legacy global timeout present
- invalid values
- AWS global and channel override precedence

## 7.2 Common HTTP path tests

Cover:

- non-stream channel timeout attaches context timeout
- stream request uses first-byte timeout path
- proxy and non-proxy clients still use pooled accessors

## 7.3 AWS tests

Cover:

- AWS non-stream uses channel or global invoke timeout
- AWS stream uses first-byte timeout path
- AWS client receives selected channel overrides

## 7.4 Retry behavior tests

Cover:

- timeout failure still reaches retry logic
- timeout failure does not silently become `skip_retry`
- retry index is captured in error metadata

## 8. Risks and Mitigations

## Risk 1: Stream-first-byte timeout is confused with existing idle timeout

Mitigation:

- keep them conceptually separate
- first-byte timeout is about stream start
- existing streaming timeout remains existing scanner/idle logic unless explicitly replaced later

## Risk 2: Client cache explosion

Mitigation:

- cache by stable transport-affecting keys only
- never cache by request ID or channel ID

## Risk 3: Timeout logic diverges between AWS and common HTTP channels

Mitigation:

- centralize timeout resolution
- minimize AWS-specific logic to SDK/client application only

## Risk 4: Retry regression

Mitigation:

- test timeout as retry-eligible failure explicitly
- audit `types.ErrOptionWithSkipRetry()` usage on timeout paths

## 9. Current Recommendation on First Execution Scope

Implement in this order:

1. config fields and timeout resolvers
2. common non-stream timeout path
3. AWS global/channel timeout controls
4. stream-first-byte control for AWS, OpenAI, Claude
5. logging metadata and retry tests

This gives the fastest path to bounded behavior without taking unnecessary risk across all channels at once.
