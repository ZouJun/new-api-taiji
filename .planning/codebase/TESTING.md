# Testing

**Analysis Date:** 2026-06-06

## Backend Test Framework

**Framework:**
- Standard Go `testing` package.
- `github.com/stretchr/testify/require` and related helpers are used in several tests.

**Run commands:**
- Whole backend: `go test ./...`
- Targeted package: `go test ./relay/channel/aws`
- Targeted test: `go test ./relay/channel/aws -run TestDoAwsClientRequest_AppliesRuntimeHeaderOverrideToAnthropicBeta`

## Existing Backend Coverage Areas

**Relay and providers:**
- `relay/channel/aws/relay_aws_test.go` covers AWS runtime header override conversion.
- `relay/channel/claude/relay_claude_test.go` and `message_delta_usage_patch_test.go` cover Claude behavior.
- `relay/channel/gemini/relay_gemini_usage_test.go` covers Gemini usage behavior.
- `relay/helper/stream_scanner_test.go`, `relay/helper/price_test.go`, `relay/common/stream_status_test.go` cover relay support helpers.

**DTO behavior:**
- `dto/openai_request_zero_value_test.go` protects explicit zero-value request fields.
- `dto/gemini_generation_config_test.go` and `dto/gemini_isstream_test.go` cover Gemini request semantics.

**Billing and quota:**
- `service/task_billing_test.go`, `service/tiered_settle_test.go`, `service/text_quota_test.go`, `pkg/billingexpr/billingexpr_test.go`.

**Models and controllers:**
- `model/model_owner_test.go`, `model/task_cas_test.go`, `controller/channel_upstream_update_test.go`, `controller/model_list_test.go`, `controller/token_test.go`.

**Middleware/common:**
- `middleware/header_nav_test.go`, `common/url_validator_test.go`, `common/json_test.go`.

## Frontend Test and Quality Scripts

**Default frontend scripts in `web/default/package.json`:**
- `bun run typecheck` - TypeScript project build.
- `bun run lint` - ESLint.
- `bun run format:check` - Prettier check.
- `bun run build` - production build.
- `bun run build:check` - typecheck and build.

**Existing frontend tests:**
- `web/default/src/components/ui/dropdown-menu.test.tsx`.
- Additional feature-level tests can be added under `web/default/src/features/**`.

## CI Coverage

**Release workflows:**
- `.github/workflows/release.yml` builds both default and classic frontends, then Go binaries.
- `.github/workflows/docker-build.yml` and related Docker workflows build images.

**Gaps:**
- The scanned workflows emphasize builds and packaging; they do not appear to run a full `go test ./...` gate in the release paths.
- Frontend typecheck/lint may be skipped in release builds through environment flags in some workflows.

## Testing Guidance for New Work

**Relay/AWS timeout or trace work:**
- Add unit tests in `relay/channel/aws` for timeout context creation and error classification where feasible.
- Add controller or relay helper tests for propagation of customer trace ID from headers into Gin context.
- Add log model/controller tests verifying `logs.other` includes trace metadata without breaking existing filters.

**Request/response archival work:**
- Add tests for non-stream capture that avoid double-reading request/response bodies.
- Add stream capture tests around SSE chunk forwarding to verify streamed bytes still reach the client in order.
- Add queue/backpressure tests for storage failure behavior, especially under high RPM.
- Add local storage implementation tests before Azure Blob integration tests.

**Cross-DB work:**
- Prefer GORM-based tests and avoid DB-specific SQL.
- If migration changes are needed, explicitly test SQLite behavior because SQLite has the strictest DDL limitations.

**Frontend changes:**
- Run `bun run typecheck` for TS/TSX edits.
- Run `bun run i18n:sync` for user-facing text changes.

---
*Testing analysis: 2026-06-06*
*Update as test coverage or CI gates change*
