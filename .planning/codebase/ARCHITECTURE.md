# Architecture

**Analysis Date:** 2026-06-06

## Pattern Overview

**Overall:** Full-stack Go API gateway/proxy with embedded React admin dashboard.

**Key Characteristics:**
- Layered backend: Router -> Controller -> Service -> Model.
- Provider adaptor pattern for upstream AI relay channels.
- Single Gin server serving API, relay routes, dashboard routes, and embedded frontend assets.
- Cross-database persistence via GORM.
- Background workers for cache sync, quota updates, task polling, and channel health.

## Layers

**Router Layer:**
- Purpose: Define HTTP route groups, middleware ordering, and relay formats.
- Contains: `router/main.go`, `router/relay-router.go`, `router/api-router.go`, `router/dashboard.go`, `router/web-router.go`.
- Depends on: controllers, middleware, relay route helpers.
- Used by: `main.go` through `router.SetRouter`.

**Controller Layer:**
- Purpose: HTTP handlers, request orchestration, response formatting, admin APIs.
- Contains: `controller/relay.go`, `controller/channel.go`, `controller/token.go`, `controller/log.go`, payment controllers, OAuth controllers.
- Depends on: service, model, relay, settings.
- Used by: router layer.

**Service Layer:**
- Purpose: Business logic, billing, quota, token estimation, HTTP clients, file fetching, task billing.
- Contains: `service/pre_consume_quota.go`, `service/tiered_settle.go`, `service/http_client.go`, `service/channel_select.go`, `service/token_estimator.go`.
- Depends on: model, common utilities, settings, selected relay abstractions.
- Used by: controllers and relay helpers.

**Model Layer:**
- Purpose: GORM models and DB operations.
- Contains: `model/main.go`, `model/log.go`, `model/channel.go`, `model/user.go`, `model/token.go`, pricing and subscription models.
- Depends on: GORM and common DB flags.
- Used by: service and controller layers.

**Relay Layer:**
- Purpose: Normalize client requests, pick providers, convert request/response formats, stream responses, and compute usage.
- Contains: `relay/*.go`, `relay/common/`, `relay/helper/`, and `relay/channel/*`.
- Depends on: DTOs, provider adaptors, service utilities, billing helpers.
- Used by: `controller.Relay` and task handlers.

**Frontend Layer:**
- Purpose: Admin dashboard and user UI.
- Contains: `web/default/src/` and `web/classic/`.
- Depends on: backend API, React, TanStack, Base UI, Tailwind, i18next.
- Served by: embedded assets in `main.go`.

## Data Flow

**Normal Relay Request:**

1. Client calls `/v1/chat/completions`, `/v1/messages`, `/v1beta/models/*path`, or another relay route in `router/relay-router.go`.
2. Middleware applies CORS, decompression, request body storage cleanup, stats, system performance check, token auth, model rate limit, and channel distribution.
3. `controller.Relay` reads and validates the request through `relay/helper`.
4. `relaycommon.GenRelayInfo` derives format, channel, model, group, token, and retry metadata.
5. Sensitive-word checks and token estimation run in `controller.Relay`.
6. `helper.ModelPriceHelper` and `service.PreConsumeBilling` pre-consume quota.
7. Retry loop selects a channel and calls the appropriate relay helper.
8. Provider adaptor converts request, sends upstream call, handles streaming or non-streaming response.
9. Usage is settled, logs are recorded, and quota is refunded or violation fees are charged on failure.

**AWS Bedrock AK/SK Flow:**

1. `relay.GetAdaptor` returns `aws.Adaptor` for AWS channel type.
2. `aws.Adaptor.DoRequest` calls `doAwsClientRequest`.
3. `newAwsClient` builds a Bedrock runtime client with shared or proxy HTTP client.
4. `doAwsClientRequest` converts Claude/OpenAI/Nova payload into Bedrock `InvokeModel` or `InvokeModelWithResponseStream` input.
5. `awsHandler` or `awsStreamHandler` creates a context with `newAwsInvokeContext`.
6. AWS SDK call runs under `common.RelayTimeout` when configured.
7. Claude-format handlers emit client response and usage.

## Key Abstractions

**Adaptor:**
- Purpose: Provider-specific conversion, request execution, response handling.
- Examples: `relay/channel/aws/adaptor.go`, `relay/channel/claude`, `relay/channel/openai`, `relay/channel/gemini`.
- Pattern: Interface-style provider adapter selected by channel API type.

**RelayInfo:**
- Purpose: Per-request relay state and billing/channel metadata.
- Location: `relay/common/relay_info.go`.
- Pattern: Mutable request context object passed through relay helpers.

**BodyStorage:**
- Purpose: Preserve request body for validation, retries, and pass-through payloads.
- Used by: `controller.Relay` and AWS pass-through in `buildAwsRequestBody`.

**Log.Other:**
- Purpose: JSON auxiliary data for consume/error logs.
- Location: `model/log.go`.
- Important for traceability additions because it already carries structured metadata.

## Entry Points

**Server:**
- Location: `main.go`.
- Responsibilities: initialize resources, cache, background tasks, Gin server, session store, middleware, routes, and embedded assets.

**Relay routes:**
- Location: `router/relay-router.go`.
- Responsibilities: route OpenAI, Claude, Gemini, image, audio, embedding, rerank, realtime, and task formats into `controller.Relay`.

**Frontend:**
- Location: `web/default/src/` and `web/classic/`.
- Built assets are embedded from `web/default/dist` and `web/classic/dist`.

## Error Handling

**Strategy:** Convert errors into `types.NewAPIError`, log them with request context, return provider-compatible JSON/SSE/WebSocket errors, and handle billing refund/violation fee in deferred cleanup.

**Patterns:**
- `controller.Relay` defers error response formatting and includes local request ID in client-facing error messages.
- Provider errors preserve status code where available, such as AWS SDK HTTP status via `getAwsErrorStatusCode`.
- Logging helpers use Gin context so request ID can be included.

## Cross-Cutting Concerns

**JSON:**
- Project convention requires `common.Marshal`, `common.Unmarshal`, `common.DecodeJson`, and related wrappers instead of direct marshal/unmarshal calls.

**Database compatibility:**
- All model and migration changes must work on SQLite, MySQL, and PostgreSQL.
- Raw SQL should use existing cross-DB helpers for reserved columns and boolean values.

**Performance:**
- Relay path is hot; avoid synchronous heavy work in request path at 8000-15000 RPM.
- Existing code already uses connection pooling, cache sync, async goroutines for some background tasks, and Redis/memory cache options.

**Traceability:**
- Local `request_id` exists and is persisted in logs.
- `upstream_request_id` exists and is persisted when captured.
- Customer `Trace-Id` is not yet a first-class cross-chain field in the scanned code.

---
*Architecture analysis: 2026-06-06*
*Update when major patterns change*
