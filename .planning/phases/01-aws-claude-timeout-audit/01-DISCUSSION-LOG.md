# Phase 1: Channel Timeout Control and AWS SDK Governance - Discussion Log

**Date:** 2026-06-07
**Status:** Captured

## Areas Discussed

### Channel-level timeout configuration
- Existing code reviewed:
  - `model/channel.go`
  - `dto/channel_settings.go`
  - `middleware/distributor.go`
  - `relay/common/relay_info.go`
  - `service/http_client.go`
  - `relay/channel/aws/relay-aws.go`
  - `relay/channel/api_request.go`
  - `relay/helper/stream_scanner.go`
  - `controller/relay.go`
- User direction captured:
  - Timeout must be dynamically configured against the channel record.
  - If `channel.setting` contains a timeout value, the request path must actually enforce it.
  - Streaming and non-streaming timeout must be separated.
  - Streaming timeout is based on first-byte/first-event timing, not total stream duration.
  - Timeout failures must not break retry behavior.
- Recommended option selected for downstream planning:
  - Store the timeout in `channel.setting` JSON through `dto.ChannelSettings`, not as a new physical DB column.
  - Use two channel fields:
    - `non_stream_timeout_seconds`
    - `stream_first_byte_timeout_seconds`
- Reasoning captured:
  - Lower migration risk
  - Reuses existing per-channel settings path
  - Cross-DB friendly
  - Keeps timeout close to other channel-specific behaviors such as `proxy`

### AWS timeout application path
- Current behavior identified:
  - AWS invoke context uses global `common.RelayTimeout`
  - Shared/proxy HTTP clients also use global `common.RelayTimeout`
- Locked discussion outcome:
  - Effective timeout must be resolved per request from `RelayInfo.ChannelSetting`
  - A configured channel timeout cannot be treated as informational metadata; it must alter live timeout behavior
  - Invocation context should use `c.Request.Context()` as parent
  - Streaming and non-streaming behavior are explicitly separated
  - Streaming first version uses first-byte timeout only; once the first stream event arrives, it is not killed by a total-stream timeout budget

### HTTP pooling and timeout control
- User direction captured:
  - All channels should honor `channel.setting` timeout configuration
  - If a channel has no timeout configuration, use default timeout configuration
  - HTTP connection pooling must be preserved
- Recommended direction selected:
  - Keep shared transport / connection pooling
  - Use request context timeout as primary non-stream timeout control
  - Use stream first-byte control as primary stream timeout control
  - Treat client-level timeout as a default guardrail, not as the main per-channel streaming control

### AWS SDK controls
- User direction captured:
  - Keep AWS SDK retry count and retry mode on prior behavior for now
  - Only make AWS SDK timeout-related configuration dynamically configurable in this phase
  - Some AWS Claude timeout-related configuration must also be channel-configurable
- Recommended direction selected:
  - Global controls:
    - `aws_http_client_non_stream_timeout_seconds`
    - `aws_http_client_stream_first_byte_timeout_seconds`
    - `aws_invoke_timeout_seconds`
  - First-version channel overrides:
    - `aws_http_client_non_stream_timeout_seconds`
    - `aws_http_client_stream_first_byte_timeout_seconds`
    - `aws_invoke_timeout_seconds`
  - `AWS_SDK_MAX_ATTEMPTS` and `AWS_SDK_RETRY_MODE` stay unchanged in this phase

## Deferred Ideas

- Customer `Trace-Id` propagation belongs to Phase 2.
- Request/response archival belongs to Phase 3.

## the agent's Discretion

- Final field name for the timeout setting may be `relay_timeout_seconds` or `request_timeout_seconds`.
- Client-level timeout override may use a keyed cache or safe clone path if planning concludes that context-only timeout is insufficient for some request modes.

---

*Discussion captured for Phase 1 on 2026-06-08*
