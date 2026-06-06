# Codebase Structure

**Analysis Date:** 2026-06-06

## Top-Level Layout

- `main.go` - application entry, resource initialization, middleware, route registration, embedded frontend assets.
- `router/` - Gin route groups for API, relay, dashboard, web, video.
- `controller/` - HTTP handlers and orchestration.
- `service/` - business logic, billing, quota, token counting, HTTP clients, file processing, task polling.
- `model/` - GORM models, database initialization, DB helpers, cache-backed model access.
- `relay/` - AI request relay core, format helpers, provider adaptors.
- `middleware/` - auth, CORS, logging, request IDs, rate limits, distribution, stats, body handling.
- `setting/` - configuration domains and option loading.
- `common/` - shared utilities for JSON, logging, Redis, URL validation, body storage, env, config.
- `dto/` - request/response DTOs.
- `constant/` - constants for API/channel types and context keys.
- `types/` - shared types and error structures.
- `i18n/` - backend translations.
- `oauth/` - OAuth provider implementations.
- `pkg/` - internal packages such as `billingexpr`, `cachex`, `ionet`, `perf_metrics`.
- `web/default/` - default React 19 frontend.
- `web/classic/` - classic React frontend.
- `electron/` - Electron packaging.
- `docs/` - project and channel documentation.

## Backend Structure

**Routing:**
- `router/relay-router.go` defines relay endpoints and middleware order.
- `router/api-router.go` and `router/dashboard.go` handle management APIs.
- `router/web-router.go` serves frontend assets and web pages.

**Relay:**
- `relay/relay_adaptor.go` maps channel API types to adaptors.
- `relay/compatible_handler.go`, `relay/claude_handler.go`, `relay/gemini_handler.go`, `relay/responses_handler.go`, and related files implement format-specific helpers.
- `relay/common/` stores relay metadata, billing helpers, overrides, outbound body handling, and stream status.
- `relay/helper/` stores request validation, price helpers, stream scanner, model mapping, and stream result helpers.
- `relay/channel/<provider>/` contains provider-specific adaptors and conversions.

**AWS provider:**
- `relay/channel/aws/adaptor.go` - adaptor interface implementation.
- `relay/channel/aws/relay-aws.go` - Bedrock client construction, request build, timeout context, response handling.
- `relay/channel/aws/dto.go` - AWS Claude and Nova DTO conversion.
- `relay/channel/aws/constants.go` - model ID mappings and cross-region rules.
- `relay/channel/aws/relay_aws_test.go` - header override test coverage.

**Billing and logs:**
- `service/pre_consume_quota.go` - pre-consumption.
- `service/tiered_settle.go` and `pkg/billingexpr/` - expression/tiered settlement.
- `model/log.go` - consume/error/task billing log storage and queries.

**HTTP clients and file IO:**
- `service/http_client.go` - shared/proxy HTTP clients.
- `service/http.go` - response copy helpers.
- `service/file_service.go`, `service/file_decoder.go`, `service/download.go` - file retrieval/decoding paths.

## Frontend Structure

**Default UI:**
- `web/default/src/routes/` - TanStack Router routes.
- `web/default/src/features/` - feature modules.
- `web/default/src/components/` - shared components and UI primitives.
- `web/default/src/i18n/locales/{lang}.json` - flat i18n JSON files for en, zh, fr, ru, ja, vi.
- `web/default/src/i18n/static-keys.ts` - static i18n key registration.

**Classic UI:**
- `web/classic/` - legacy/classic frontend, also built in release workflows.

## Test Layout

**Backend tests:**
- Go tests use `*_test.go` files near the package under test.
- Existing tests cover billing, channel routing, AWS request conversion, relay helpers, DTO zero-value handling, model behavior, and middleware.

**Frontend tests:**
- Component tests exist in `web/default/src/**/*.test.tsx`.
- Typecheck/lint/build scripts are in `web/default/package.json`.

## CI/CD Layout

- `.github/workflows/release.yml` - multi-platform binary release.
- `.github/workflows/docker-build.yml` - tagged Docker multi-arch images.
- `.github/workflows/docker-image-alpha.yml` and `docker-image-nightly.yml` - branch-based image builds.
- `.github/workflows/electron-build.yml` - Windows Electron build.
- `.github/workflows/pr-check.yml` - PR quality gate.

## Naming Conventions

**Go:**
- Package directories are short lowercase names.
- Tests live beside implementation.
- Provider adaptors use `Adaptor` types and provider package names.

**Frontend:**
- Feature modules under `src/features/<feature>/`.
- Components generally use PascalCase.
- Utilities and hooks are colocated by feature or under shared `src/lib` / `src/hooks`.

---
*Structure analysis: 2026-06-06*
*Update when directory layout changes*
