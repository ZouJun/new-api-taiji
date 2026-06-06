# Codebase Concerns

**Analysis Date:** 2026-06-06

## Tech Debt

**JSON wrapper consistency:**
- Issue: Some business/provider code still uses direct `encoding/json` marshal/unmarshal calls, for example AWS Nova handling.
- Files: `relay/channel/aws/dto.go`, `relay/channel/aws/relay-aws.go`.
- Impact: Violates project convention and can create inconsistent behavior if JSON implementation changes.
- Fix approach: Replace actual marshal/unmarshal calls with `common.Marshal`, `common.Unmarshal`, and `common.DecodeJson` while keeping `json.RawMessage` type usage where needed.

**Optional zero-value preservation:**
- Issue: Some provider DTO/conversion paths may still drop explicit zero values when using non-pointer scalar fields and `omitempty`.
- Files: `relay/channel/aws/dto.go` has `TopP float64`, `TopK int`, and Nova conversion checks `*value != 0`.
- Impact: Clients explicitly sending `0`, `0.0`, or `false` can lose intent before upstream call.
- Fix approach: Use pointer optional fields and preserve explicit zero semantics in conversions, following `dto/openai_request_zero_value_test.go`.

## Known Bugs

**No confirmed bugs from this mapping pass.**
- The mapping was read-only and did not execute the full test suite.
- Treat this section as a caution list, not a defect report.

## Security Considerations

**Sensitive request/response archival:**
- Risk: The requested future feature to store client prompts and upstream responses can capture secrets, PII, credentials, or regulated data.
- Current mitigation: Existing logs store metadata, not full payloads.
- Recommendations: Add explicit feature flags, retention controls, encryption, access controls, redaction hooks, size limits, and failure isolation before enabling archival in production.

**SSRF and external file fetches:**
- Risk: File or URL ingestion can be abused to hit internal services.
- Current mitigation: `service.checkRedirect` uses `common.ValidateURLWithFetchSetting`.
- Recommendations: Preserve this validation if adding archival or replay features that read external URLs.

**Trace ID trust boundary:**
- Risk: Customer-supplied `Trace-Id` can be spoofed or contain unsafe characters.
- Current mitigation: Local request IDs are generated internally.
- Recommendations: Sanitize and length-limit customer trace IDs before logging, file naming, or blob path construction.

## Performance Bottlenecks

**Full payload archival in relay path:**
- Problem: Uploading request and response bodies synchronously to local disk or Azure Blob from the hot path would add latency and failure coupling.
- Expected load: User specified 8000-15000 RPM.
- Cause: Blob/network IO, compression, buffering, and stream teeing can block request completion if not isolated.
- Improvement path: Use bounded async queue, worker pool, chunk-safe stream tee, size limits, retry/dead-letter policy, and metrics.

**Log writes and large `other` fields:**
- Problem: `logs.other` is a string column used for JSON metadata; storing large payloads there would hurt DB performance.
- Files: `model/log.go`.
- Improvement path: Store only trace IDs, blob object names, hashes, byte counts, and status in `logs.other`; keep full payloads in blob/local object storage.

## Fragile Areas

**Relay stream handling:**
- Why fragile: Streaming response handlers write incremental chunks to clients while also deriving usage and final status.
- Files: `relay/channel/aws/relay-aws.go`, `relay/channel/claude`, `relay/helper/stream_scanner.go`, `relay/common/stream_status.go`.
- Common failures: Out-of-order chunks, missing final event, broken flush behavior, stalled client connection, double close.
- Safe modification: Use tee-style capture with bounded buffers and tests that assert client receives exact streamed bytes.

**Billing cleanup around relay errors:**
- Why fragile: `controller.Relay` uses deferred cleanup for refund and violation fees.
- Files: `controller/relay.go`, `service/pre_consume_quota.go`, `service/tiered_settle.go`.
- Common failures: Charging on storage-only failures, refunding after response already succeeded, losing original upstream error.
- Safe modification: Keep archival/storage failures non-fatal by default and record status in `logs.other`.

**AWS Bedrock invocation:**
- Why fragile: Uses two modes: API key HTTP path and AK/SK SDK path. Streaming and non-streaming use different AWS SDK calls.
- Files: `relay/channel/aws/adaptor.go`, `relay/channel/aws/relay-aws.go`.
- Common failures: Timeout behavior differs between HTTP client timeout and SDK context timeout; stream close errors may not surface.
- Safe modification: Keep timeout tests and document semantics clearly.

## Scaling Limits

**Archive throughput:**
- Current capacity: Not implemented in scanned code.
- Target pressure: 8000-15000 RPM means roughly 133-250 requests/sec before retries or fan-out.
- Symptoms at limit: queue growth, blob throttling, disk fill, increased p95 latency, dropped captures.
- Scaling path: bounded async ingestion, batching manifests rather than request batching, compression per request or per object class, worker pool tuning, per-tenant quotas, and backpressure policy.

**Database logs:**
- Current indexes include `request_id`, `upstream_request_id`, `created_at`, `user_id`, token/model/channel fields.
- Limit: high-cardinality trace metadata should not be queried through unindexed JSON inside `other`.
- Scaling path: add first-class indexed column only if querying customer trace ID is required; otherwise store it in `other` and blob object names.

## Test Coverage Gaps

**Customer trace ID propagation:**
- Not yet covered because customer `Trace-Id` is not a first-class field in current code.

**Payload archival:**
- Not implemented; future work needs stream and non-stream capture tests, local and Azure storage tests, and failure-mode tests.

**AWS timeout behavior:**
- `newAwsInvokeContext` exists but direct timeout behavior is not covered by current AWS test.

---
*Concerns audit: 2026-06-06*
*Update as issues are fixed or new ones discovered*
