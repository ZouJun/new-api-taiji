# Requirements: new-api Reliability and Traceability

**Defined:** 2026-06-06
**Core Value:** Every relay request must remain stable, bounded, and traceable from customer request through upstream call, response delivery, storage/audit artifacts, and database logs.

## v1 Requirements

Requirements for this milestone. Each maps to exactly one roadmap phase.

### Timeout and SDK Control

- [ ] **AWS-01**: Maintainer can identify whether AWS Claude upstream calls have timeout protection.
- [ ] **AWS-02**: Maintainer can see exactly how `common.RelayTimeout` affects AWS Bedrock SDK calls in streaming and non-streaming modes.
- [ ] **AWS-03**: Maintainer can verify timeout behavior with focused tests or documented verification steps.
- [ ] **AWS-04**: Maintainer can understand the difference between shared HTTP client timeout, proxy client timeout, and AWS invocation context timeout.
- [ ] **TIME-01**: All channels can define independent relay timeout settings instead of relying only on a single global timeout.
- [ ] **TIME-02**: Streaming timeout semantics are separated from non-streaming timeout semantics; streaming timeout is based on time-to-first-byte/first event rather than total stream duration.
- [ ] **TIME-03**: Timeout failures record detailed structured metadata for later troubleshooting, including effective timeout, timeout stage, stream flag, and timeout source.
- [ ] **TIME-04**: Timeout failures do not break or short-circuit the existing retry mechanism.
- [ ] **TIME-05**: Timeout configuration supports provider differences such as AWS, Sora, and other slow or long-running upstream channels.
- [ ] **TIME-06**: Per-channel timeout settings are stored in `channel.setting` as two separate fields: non-stream total timeout and stream first-byte timeout.
- [ ] **TIME-07**: After the first stream byte/event is received, the first version does not enforce a separate total stream timeout kill switch.
- [ ] **TIME-08**: All channels build or resolve HTTP behavior using channel-level timeout settings when configured, while preserving connection pooling and falling back to shared defaults when absent.
- [ ] **SDK-01**: AWS relay invoke-timeout and retry-attempt knobs used by execution are exposed to New API configuration instead of being fully hidden inside SDK defaults.
- [ ] **SDK-02**: Maintainers can explain which AWS knobs are controlled globally, which are controlled per channel, and how they interact.
- [ ] **SDK-03**: New API globally controls `AWS_INVOKE_TIMEOUT_SECONDS` and `AWS_SDK_MAX_ATTEMPTS` with documented interaction rules against per-channel timeout settings.
- [ ] **SDK-04**: `aws_invoke_timeout_seconds` and `aws_sdk_max_attempts` can be overridden per channel through `channel.setting`, with documented precedence over AWS global defaults.

### Customer Trace

- [x] **TRAC-01**: System extracts customer `Trace-Id` from request headers using the documented fixed-header policy.
- [x] **TRAC-02**: System strictly validates and length-limits customer trace IDs before logging, context propagation, or object naming reuse.
- [x] **TRAC-03**: System carries customer trace ID through Gin context and request context alongside the existing server request ID.
- [x] **TRAC-04**: Error logs and consume logs persist customer trace ID in `logs.other`.
- [x] **TRAC-05**: Error log messages include customer trace ID wherever it is available.

### Payload Archival

- [ ] **ARCH-01**: System can capture client request payloads for relay calls without rereading or corrupting request bodies.
- [ ] **ARCH-02**: System can capture non-streaming upstream/client response payloads without changing client-visible responses.
- [ ] **ARCH-03**: System can capture streaming response chunks while preserving flush timing and chunk order.
- [ ] **ARCH-04**: System can store archived payloads in either local storage or Azure Blob based on configuration.
- [ ] **ARCH-04A**: Archive backend selection between local storage and Azure Blob does not require changes to existing table logic.
- [ ] **ARCH-05**: Archive object names include server request ID and sanitized customer trace ID.
- [ ] **ARCH-06**: System writes per-request archive metadata including object names, hashes, byte counts, stream flag, provider/channel/model, status, and errors.
- [ ] **ARCH-07**: `logs.other` stores archive references and metadata, not full request or response payloads.

### Reliability and Operations

- [ ] **OPS-01**: Archival does not block successful customer responses on local disk or Azure Blob availability by default.
- [ ] **OPS-02**: Archival uses bounded queues, worker limits, retries, and explicit overflow behavior suitable for 8000-15000 RPM.
- [ ] **OPS-03**: Storage failures are observable through logs/metadata/metrics without hiding the original relay outcome.
- [ ] **OPS-04**: Design includes retention, compression, size limits, redaction hooks, and security/access-control guidance.
- [ ] **OPS-05**: All database-related changes remain compatible with SQLite, MySQL, and PostgreSQL.

### Documentation and Verification

- [ ] **DOC-01**: Design documentation explains the current AWS timeout implementation in beginner-readable detail.
- [ ] **DOC-02**: Design documentation explains the full archival architecture, object naming, local/Azure behavior, batching decision, compression, failure modes, and operational tuning.
- [ ] **DOC-03**: Design documentation explains customer Trace-Id propagation and where it appears in logs, archive objects, and errors.
- [ ] **TEST-01**: Tests cover Trace-Id extraction, sanitization, context propagation, and `logs.other` persistence.
- [ ] **TEST-02**: Tests cover archival capture for non-streaming and streaming relay paths.
- [ ] **TEST-03**: Tests cover storage failure behavior and queue overflow policy.

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Archive Operations

- **AOPS-01**: Admin can search archive manifests by customer trace ID from the dashboard.
- **AOPS-02**: System can run offline compaction jobs that package old per-request objects into larger archive bundles.
- **AOPS-03**: System can apply tenant-specific archive retention policies.
- **AOPS-04**: System can redact payloads with configurable rules before archival.

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Full payload storage in `logs.other` | Would bloat DB rows and degrade high-RPM log performance |
| Cross-request zip batching in the relay hot path | Requires waiting for batches and complicates failure isolation |
| Synchronous Azure Blob upload as the default response path | Violates stability and latency goals at 8000-15000 RPM |
| Removing existing server-generated request IDs | They are trusted internal correlation IDs and should remain separate from customer trace IDs |
| Dropping SQLite/MySQL/PostgreSQL compatibility | Project policy requires all three databases |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| AWS-01 | Phase 1 | Completed |
| AWS-02 | Phase 1 | Completed |
| AWS-03 | Phase 1 | Completed |
| AWS-04 | Phase 1 | Completed |
| TIME-01 | Phase 1 | Completed |
| TIME-02 | Phase 1 | Completed |
| TIME-03 | Phase 1 | Completed |
| TIME-04 | Phase 1 | Completed |
| TIME-05 | Phase 1 | Completed |
| TIME-06 | Phase 1 | Completed |
| TIME-07 | Phase 1 | Completed |
| TIME-08 | Phase 1 | Completed |
| SDK-01 | Phase 1 | Completed |
| SDK-02 | Phase 1 | Completed |
| SDK-03 | Phase 1 | Completed |
| SDK-04 | Phase 1 | Completed |
| TRAC-01 | Phase 2 | Completed |
| TRAC-02 | Phase 2 | Completed |
| TRAC-03 | Phase 2 | Completed |
| TRAC-04 | Phase 2 | Completed |
| TRAC-05 | Phase 2 | Completed |
| ARCH-01 | Phase 3 | Pending |
| ARCH-02 | Phase 3 | Pending |
| ARCH-03 | Phase 3 | Pending |
| ARCH-04 | Phase 3 | Pending |
| ARCH-04A | Phase 3 | Pending |
| ARCH-05 | Phase 3 | Pending |
| ARCH-06 | Phase 3 | Pending |
| ARCH-07 | Phase 3 | Pending |
| OPS-01 | Phase 3 | Pending |
| OPS-02 | Phase 3 | Pending |
| OPS-03 | Phase 3 | Pending |
| OPS-04 | Phase 3 | Pending |
| OPS-05 | Phase 3 | Pending |
| DOC-01 | Phase 4 | Pending |
| DOC-02 | Phase 4 | Pending |
| DOC-03 | Phase 4 | Pending |
| TEST-01 | Phase 4 | Pending |
| TEST-02 | Phase 4 | Pending |
| TEST-03 | Phase 4 | Pending |

**Coverage:**
- v1 requirements: 40 total
- Mapped to phases: 40
- Unmapped: 0

---
*Requirements defined: 2026-06-06*
*Last updated: 2026-06-09 after Phase 2 closeout*
