# Phase 1: AWS Claude Timeout Audit - Discussion Log

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
- User direction captured:
  - Timeout must be dynamically configured against the channel record.
- Recommended option selected for downstream planning:
  - Store the timeout in `channel.setting` JSON through `dto.ChannelSettings`, not as a new physical DB column.
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
  - Invocation context should use `c.Request.Context()` as parent
  - Streaming and non-streaming behavior must be analyzed separately during planning

## Deferred Ideas

- Customer `Trace-Id` propagation belongs to Phase 2.
- Request/response archival belongs to Phase 3.

## the agent's Discretion

- Final field name for the timeout setting may be `relay_timeout_seconds` or `request_timeout_seconds`.
- Client-level timeout override may use a keyed cache or safe clone path if planning concludes that context-only timeout is insufficient for some request modes.

---

*Discussion captured for Phase 1 on 2026-06-07*
