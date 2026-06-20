# new-api Reliability and Traceability

## What This Is

This is an existing AI API gateway/proxy that aggregates many upstream AI providers behind a unified API with user management, billing, rate limiting, and an admin dashboard. The current milestone focuses on reliability and traceability for high-volume relay traffic, especially channel-level timeout control, AWS SDK timeout/retry governance, full request/response archival, and customer Trace-Id propagation.

The target audience is operators and developers maintaining the gateway under 8000-15000 RPM, where stability, fault isolation, performance, and auditability matter more than adding user-visible features.

## Core Value

Every relay request must remain stable, bounded, and traceable from customer request through upstream call, response delivery, storage/audit artifacts, and database logs.

## Requirements

### Validated

- ✓ The backend runs as a Gin-based Go API gateway with layered Router -> Controller -> Service -> Model architecture — existing
- ✓ The relay layer supports many upstream AI providers through provider adaptors under `relay/channel/` — existing
- ✓ AWS Bedrock Claude/Nova relay support exists through `relay/channel/aws/` — existing
- ✓ Relay billing, quota pre-consumption, refund handling, and usage settlement exist in controller/service/model paths — existing
- ✓ Consume and error logs persist `request_id`, `upstream_request_id`, `other`, model, token, channel, quota, and timing metadata in `model.Log` — existing
- ✓ Default frontend uses React 19, TypeScript, Rsbuild, Base UI, Tailwind CSS, and i18next — existing
- ✓ The project supports SQLite, MySQL, and PostgreSQL through GORM — existing

### Active

- [ ] Design and implement channel-level timeout control for all relay channels, not only AWS, with clear fallback to global defaults.
- [ ] Separate streaming and non-streaming timeout semantics so that streaming timeout is based on first response chunk timing rather than total stream duration.
- [ ] Ensure timeout failures emit detailed structured metadata and remain compatible with the existing retry mechanism.
- [ ] Expose retained AWS invoke-timeout and retry-attempt knobs to the New API layer for unified control and tuning.
- [ ] Design a complete full-chain request/response archival system for both streaming and non-streaming relay calls, with a switchable local/Azure Blob backend.
- [ ] Ensure archival design handles 8000-15000 RPM without coupling upstream latency or availability to blob/local storage failures.
- [ ] Define object naming, metadata, compression, batching/non-batching strategy, retention, retry, dead-letter, and observability for archived payloads.
- [ ] Preserve original downstream request body bytes and original upstream response body bytes wherever available; fallback capture is allowed only when raw bytes are unavailable and must be marked in metadata.
- [ ] Capture the confirmed request-header whitelist for consume/error logs and error output, storing empty values and truncating oversized values without rejecting requests.
- [ ] Store upstream-native usage metadata in consume logs for reconciliation, with estimated/incomplete fallback metadata for interrupted streams.
- [ ] Add configurable CPU, memory, and local disk safety thresholds that skip archive storage under pressure even when archive storage is enabled.
- [ ] Capture customer `Trace-Id` from request headers, sanitize it, and propagate it through Gin context, relay logs, error logs, and storage object names.
- [ ] Store customer trace metadata in `logs.other` without putting full request/response payloads in the database.
- [ ] Improve error logging so customer trace ID is printed wherever available, while preserving the existing local request ID.
- [ ] Produce solution documents that are detailed enough for beginner maintainers to understand the chain, failure modes, and implementation plan.

### Out of Scope

- Replacing the existing relay adaptor architecture — the work should integrate with current provider patterns.
- Storing full payloads directly in `logs.other` or another database column — this would harm DB performance and complicate retention.
- Changing existing table logic solely to support archive backend selection — local versus Azure Blob must be selected by configuration, not by new archive tables or archive-specific schema changes.
- Making storage upload failures fail otherwise successful customer relay requests by default — archival must be isolated unless explicitly configured otherwise.
- Removing or renaming protected project or organization identifiers — project policy forbids this.
- Dropping support for SQLite, MySQL, or PostgreSQL — all persistence changes must remain cross-DB compatible.

## Context

The repository has already been mapped in `.planning/codebase/`. Key reference files:

- `.planning/codebase/ARCHITECTURE.md` documents the relay flow and AWS Bedrock invocation path.
- `.planning/codebase/CONCERNS.md` calls out archival throughput, trace ID trust boundary, and streaming fragility.
- `.planning/codebase/TESTING.md` lists current tests and gaps for AWS timeout, trace propagation, and archival.
- `relay/channel/aws/relay-aws.go` contains `newAwsInvokeContext`, `awsHandler`, and `awsStreamHandler`.
- `service/http_client.go` contains shared HTTP client timeout and transport pool configuration.
- `model/log.go` contains `Log`, `RecordConsumeLog`, and `RecordErrorLog`.
- `middleware/request-id.go` creates the internal request ID.
- `middleware/logger.go` prints request ID in Gin access logs.
- `controller/relay.go` owns request validation, retry loop, billing cleanup, error response formatting, and log-producing relay behavior.

Important current findings:

- AWS AK/SK mode creates a Bedrock SDK client with the shared or proxy HTTP client.
- AWS SDK calls use a context created by `newAwsInvokeContext`; when `common.RelayTimeout > 0`, it applies `context.WithTimeout(..., RelayTimeout seconds)`.
- The shared `http.Client` also uses `common.RelayTimeout` as `http.Client.Timeout` when non-zero.
- Current timeout behavior is global-first and AWS-specific in implementation detail; it does not yet satisfy the new requirement for per-channel timeout control and separate streaming first-byte timeout semantics.
- Local request IDs are already generated and persisted; customer `Trace-Id` is not yet a first-class field in the scanned code.
- `logs.other` is suitable for structured metadata such as `customer_trace_id`, object names, hashes, byte counts, and storage status, but not full payloads.

## Constraints

- **Performance**: 8000-15000 RPM means roughly 133-250 requests/second before retries, so archival must use bounded async work, backpressure rules, and non-blocking stream capture.
- **Payload distribution**: Expected traffic includes both large-payload scenarios around 64MB per request and small-payload scenarios around 3-5KB per request. Storage strategy must not rely on one batching model for both distributions.
- **Stability**: Customer response delivery and upstream timeout behavior must remain bounded even if Azure Blob, disk, or archive workers are slow or unavailable.
- **Traceability**: Object names must include both local `requestId` and sanitized customer `Trace-Id` when provided.
- **Security**: Full request/response payloads can contain secrets or PII; the design must include feature flags, retention, access controls, encryption assumptions, redaction hooks, and size limits.
- **Database compatibility**: Any schema or query changes must support SQLite, MySQL >= 5.7.8, and PostgreSQL >= 9.6.
- **JSON convention**: Marshal/unmarshal operations in business code must use `common/json.go` wrappers.
- **Relay semantics**: Streaming and non-streaming behavior must preserve client-visible bytes, flush timing, usage settlement, and error handling.
- **Payload fidelity**: Request archives must prefer original downstream request bytes and response archives must prefer original upstream response bytes. Later-stage fallback captures are acceptable only when raw bytes are not available and must be explicitly marked.
- **Beginner readability**: Architecture and implementation documents should explain the current flow, where code changes happen, why each decision exists, and what failure behavior to expect.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Treat this as brownfield work on the existing gateway | The repository already implements relay, billing, logging, and AWS provider paths | — Pending |
| Use `.planning/codebase` as the source of current architecture context | Codebase mapping was generated before initialization | — Pending |
| Keep full payload archival outside the database | DB logs are metadata-oriented and high-RPM payload storage would degrade query/write performance | — Pending |
| Choose local versus Azure Blob archival purely through configuration | The user explicitly does not want archive backend selection to depend on existing table logic changes | — Pending |
| Prefer async isolated archival over synchronous blob upload in the hot path | The user requires stability and performance at 8000-15000 RPM | — Pending |
| Keep local request ID and customer Trace-Id as separate concepts | Local IDs are trusted server-generated IDs; customer IDs are external correlation IDs | — Pending |
| Capture only the confirmed request-header whitelist in Phase 3 | The user explicitly limited header storage to the listed fields for now | Accepted |
| Truncate oversized captured header values instead of rejecting requests | The user confirmed storage/logging should be bounded without affecting request acceptance | Accepted |
| Allow estimated usage metadata when streams interrupt before native usage arrives | The user confirmed estimated usage is acceptable if marked as such | Accepted |
| Make archive runtime safety thresholds configurable with default values | The user confirmed CPU 80%, memory 80%, disk 15%/10GB, and 5s check interval as defaults | Accepted |
| Prefer raw downstream request bytes and raw upstream response bytes for archives | The user confirmed payload archives must be upstream/downstream original data whenever available | Accepted |
| Allow fallback payload capture only when raw bytes are unavailable and mark the capture stage | Some relay paths may not expose original bytes, but operators must be able to distinguish fallback captures | Accepted |
| Put expanded timeout and AWS SDK configurability work into Phase 1 | The user wants timeout hardening done first and it is a prerequisite for safe high-volume relay behavior | Accepted |
| Split timeout settings into non-stream and stream-first-byte fields in `channel.setting` | The user wants streaming and non-streaming timeout semantics separated and configured per channel | Accepted |
| Apply timeout configuration to all channels through `channel.setting`, with fallback to defaults | The user wants timeout control generalized beyond AWS to channels such as Sora and future providers | Accepted |
| Keep HTTP connection pooling while honoring per-channel timeout settings | The system must remain performant at 8000-15000 RPM and cannot regress into one-client-per-request without pooling | Accepted |
| Treat timeout failures as compatible with the existing retry mechanism | The user explicitly requires timeout failures to participate in retries rather than bypass them | Accepted |
| Keep AWS SDK `max_attempts` as a documented retained control | The current project still exposes `AWS_SDK_MAX_ATTEMPTS` and `aws_sdk_max_attempts` in code | Accepted |
| Remove AWS HTTP client timeout configuration from the Phase 1 plan surface | The user clarified that AWS HTTP client timeout-related configuration has been fully removed | Accepted |
| Allow selected AWS Claude timeout-related knobs to be overridden per channel in `channel.setting` | The user wants AWS Claude timeout behavior to remain channel-tunable without expanding retry-mode controls in this phase | Accepted |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `$gsd-transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `$gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check -> still the right priority?
3. Audit Out of Scope -> reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-06-06 after initialization*
