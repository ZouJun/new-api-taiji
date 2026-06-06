# Coding Conventions

**Analysis Date:** 2026-06-06

## Naming Patterns

**Go files and packages:**
- Package names are lowercase and concise, for example `service`, `model`, `middleware`, `relay`, `aws`.
- Tests use standard `*_test.go` names beside the code under test.
- Provider code is grouped under `relay/channel/<provider>/`.

**Go functions and types:**
- Exported functions/types use PascalCase: `RecordConsumeLog`, `RelayInfo`, `Adaptor`.
- Unexported helpers use camelCase: `newAwsInvokeContext`, `formatRequest`, `getAwsErrorStatusCode`.
- Request/response DTOs use descriptive struct names in `dto/` or provider packages.

**Frontend:**
- Components use PascalCase.
- Hooks use `use*`.
- i18n locale files are flat JSON under `web/default/src/i18n/locales/`.

## Code Style

**Go formatting:**
- Use `gofmt` / `go test` conventions.
- Keep provider-specific logic inside the provider package when possible.
- Prefer small helpers for cross-cutting logic rather than duplicating request/response handling.

**Frontend formatting:**
- Use `bun run format` / `bun run format:check` in `web/default/`.
- ESLint disallows `console` in frontend code and enforces type imports.
- TypeScript changes require `bun run typecheck`.

## Import Organization

**Go:**
- Standard library first, blank line, project imports, blank line, external imports.
- Project imports use `github.com/QuantumNous/new-api/...`.
- Per project policy, direct JSON marshal/unmarshal calls should not be used in business code; use `common` wrappers.

**Frontend:**
- Path alias `@/*` maps to `web/default/src/*`.
- Type-only imports are enforced by ESLint.

## Error Handling

**Backend patterns:**
- Relay errors are normalized into `types.NewAPIError`.
- Provider errors are wrapped with context and mapped to provider-compatible responses.
- `controller.Relay` centralizes client-facing error response formatting.
- Billing refund and violation fee handling happen in `defer` blocks after relay failures.

**Logging patterns:**
- Use context-aware logger functions such as `logger.LogError(c, ...)`, `logger.LogInfo(c, ...)`, and common system log helpers.
- Request ID is stored in Gin context by `middleware.RequestId`.
- Business logs are persisted through `model.RecordConsumeLog`, `model.RecordErrorLog`, and task billing helpers.

## JSON Handling

**Project rule:**
- Use `common.Marshal`, `common.Unmarshal`, `common.UnmarshalJsonStr`, and `common.DecodeJson`.
- `encoding/json` types such as `json.RawMessage` are allowed as types.

**Observed care points:**
- Some AWS code imports `encoding/json` for direct marshal/unmarshal in `relay/channel/aws/dto.go` and `relay/channel/aws/relay-aws.go`; future changes should align with the wrapper rule.
- Optional upstream request scalar fields should use pointer types with `omitempty` to preserve explicit zero/false values when re-marshaled.

## Database Conventions

**Compatibility:**
- Code must support SQLite, MySQL, and PostgreSQL together.
- Prefer GORM abstractions over raw SQL.
- Raw SQL must account for reserved column quoting and boolean differences.
- Use shared variables from `model/main.go` for reserved columns like `group` and `key`.

**Logs:**
- Request trace data should fit existing `logs` columns where possible: `request_id`, `upstream_request_id`, and structured `other`.
- `Other` is stored as a string containing JSON-like data generated through common map helpers.

## Frontend i18n

**Default frontend:**
- User-facing text must use `useTranslation()` and `t(...)`.
- Flat locale JSON uses English source strings as keys.
- Supported locales: en, zh, fr, ja, ru, vi.
- Use `bun run i18n:sync` from `web/default/` after adding UI text.

## Function and Module Design

**Relay hot path:**
- Avoid blocking, large memory copies, or synchronous external storage in the relay response path unless isolated behind bounded queues/timeouts.
- Preserve stream/non-stream behavior when adding wrappers around response bodies or writers.
- Keep provider-specific quirks in provider adaptors or relay helper utilities.

**Configuration:**
- New runtime toggles should follow existing `common` / `setting` patterns and be safe under concurrent requests.

---
*Convention analysis: 2026-06-06*
*Update when patterns change*
