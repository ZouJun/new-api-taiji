# Roadmap: new-api Reliability and Traceability

## Overview

This milestone turns the existing relay gateway into a more traceable and auditable system without destabilizing the hot path. It starts by hardening timeout behavior across channels and exposing key AWS SDK timeout controls, then adds customer Trace-Id propagation, then designs and implements isolated request/response archival for local or Azure Blob storage, and finally hardens the work with beginner-readable docs and verification.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Channel Timeout Control and AWS SDK Governance** - Add channel-level timeout control, streaming/non-streaming timeout semantics, timeout observability, and AWS SDK timeout configuration management.
- [ ] **Phase 2: Customer Trace-Id Propagation** - Extract, sanitize, propagate, log, and persist customer trace IDs.
- [ ] **Phase 3: Request Response Archive Pipeline** - Design and implement local/Azure archival for streaming and non-streaming relay payloads.
- [ ] **Phase 4: Verification and Operator Documentation** - Verify the chain and produce beginner-readable operational documentation.

## Phase Details

### Phase 1: Channel Timeout Control and AWS SDK Governance
**Goal**: Timeout behavior becomes configurable and traceable across channels, while retained AWS relay knobs are exposed for unified management at the New API layer.
**Depends on**: Nothing (first phase)
**Requirements**: [AWS-01, AWS-02, AWS-03, AWS-04, TIME-01, TIME-02, TIME-03, TIME-04, TIME-05, TIME-06, TIME-07, TIME-08, SDK-01, SDK-02, SDK-03, SDK-04]
**Success Criteria** (what must be TRUE):
  1. Every channel can resolve effective timeout behavior from `channel.setting`, using two separate fields for non-stream total timeout and stream first-byte timeout, with documented fallback to defaults.
  2. Streaming requests use first-byte/first-event timeout semantics that are distinct from non-streaming request timeout semantics, and once the first stream byte arrives the first version does not enforce a separate total stream timeout kill switch.
  3. All channels honor channel-level timeout settings when configured, while preserving HTTP connection pooling and falling back to shared defaults when channel settings are absent.
  4. Timeout failures generate structured logs and database metadata that identify effective timeout, timeout source, timeout stage, stream mode, and retry index.
  5. Timeout failures do not bypass or break the existing retry flow.
  6. Maintainer can point to the exact AWS Bedrock client and invocation code path and explain how global `AWS_INVOKE_TIMEOUT_SECONDS` / `AWS_SDK_MAX_ATTEMPTS` and per-channel `aws_invoke_timeout_seconds` / `aws_sdk_max_attempts` are applied.
  7. Timeout and SDK behavior are backed by focused tests or documented verification steps.
**Plans**: 4 plans

Plans:
- [x] 01-01: Trace the current timeout path across shared HTTP client, proxy client, AWS invoke context, and retry logic.
- [x] 01-02: Design and implement channel-level timeout resolution for all channels, using separate `channel.setting` fields for non-stream and stream-first-byte timeout semantics.
- [x] 01-03: Add timeout observability, including detailed error metadata, and verify that timeout failures remain compatible with the retry flow.
- [x] 01-04: Expose and govern retained AWS invoke-timeout and retry-attempt controls from the New API layer, including selected per-channel AWS Claude overrides, with focused verification.

### Phase 2: Customer Trace-Id Propagation
**Goal**: Customer `Trace-Id` becomes a safe, separate correlation value that follows the request through context, logs, errors, and future archive metadata.
**Depends on**: Phase 1
**Requirements**: [TRAC-01, TRAC-02, TRAC-03, TRAC-04, TRAC-05]
**Success Criteria** (what must be TRUE):
  1. System extracts a customer trace ID from the configured request header.
  2. Unsafe or oversized trace IDs are sanitized and bounded before use.
  3. Trace ID is available from Gin context and request context wherever relay code can access it.
  4. Consume and error logs include trace metadata in `logs.other`.
  5. Error log messages include customer trace ID when present, without replacing server request ID.
**Plans**: 3 plans

Plans:
- [ ] 02-01: Add trace ID extraction, sanitization, constants, and context propagation.
- [ ] 02-02: Persist trace metadata into consume/error logs and `logs.other`.
- [ ] 02-03: Add tests for trace extraction, sanitization, logging, and error output.

### Phase 3: Request Response Archive Pipeline
**Goal**: Relay requests and responses can be archived to local storage or Azure Blob with bounded, observable, non-blocking behavior suitable for 8000-15000 RPM.
**Depends on**: Phase 2
**Requirements**: [ARCH-01, ARCH-02, ARCH-03, ARCH-04, ARCH-05, ARCH-06, ARCH-07, OPS-01, OPS-02, OPS-03, OPS-04, OPS-05]
**Success Criteria** (what must be TRUE):
  1. System captures request bodies and non-streaming responses without corrupting replay, retry, billing, or client output.
  2. System captures streaming response chunks while preserving chunk order and client flush behavior.
  3. Archive storage can switch between local and Azure Blob through configuration.
  4. Archive object names include server request ID and sanitized customer trace ID.
  5. Archive failures are isolated from successful customer responses by default and recorded as metadata.
  6. Queue limits, worker limits, retries, overflow behavior, compression, retention, and security controls are documented in code-facing design.
**Plans**: 4 plans

Plans:
- [ ] 03-01: Design archive interfaces, object naming, manifests, metadata, and configuration.
- [ ] 03-02: Implement local archive backend and non-streaming capture path.
- [ ] 03-03: Implement streaming capture, bounded queue, worker pool, and failure metadata.
- [ ] 03-04: Implement Azure Blob backend and storage-mode switching.

### Phase 4: Verification and Operator Documentation
**Goal**: Maintainers have clear proof and documentation for timeout behavior, Trace-Id propagation, and archival operation under high load assumptions.
**Depends on**: Phase 3
**Requirements**: [DOC-01, DOC-02, DOC-03, TEST-01, TEST-02, TEST-03]
**Success Criteria** (what must be TRUE):
  1. Beginner-readable docs explain the AWS timeout path, including file names and control flow.
  2. Beginner-readable docs explain request/response archival, naming, local/Azure storage, compression, non-batching decision, failure modes, and tuning.
  3. Beginner-readable docs explain where customer Trace-Id appears in request context, errors, logs, archive metadata, and object names.
  4. Tests cover trace propagation, archival capture, storage failure behavior, and queue overflow policy.
  5. Verification notes explain what was tested, what remains risky, and how operators should monitor the feature.
**Plans**: 3 plans

Plans:
- [ ] 04-01: Add end-to-end and failure-mode tests for trace and archival.
- [ ] 04-02: Write operator and maintainer documentation.
- [ ] 04-03: Run final verification and update planning artifacts.

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Channel Timeout Control and AWS SDK Governance | 4/4 | Completed | 2026-06-09 |
| 2. Customer Trace-Id Propagation | 0/3 | Not started | - |
| 3. Request Response Archive Pipeline | 0/4 | Not started | - |
| 4. Verification and Operator Documentation | 0/3 | Not started | - |
