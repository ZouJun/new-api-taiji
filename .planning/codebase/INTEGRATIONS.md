# External Integrations

**Analysis Date:** 2026-06-06

## AI Provider Integrations

**Relay providers:**
- Provider adaptors are registered in `relay/relay_adaptor.go`.
- Channel implementations live under `relay/channel/`, including `openai`, `claude`, `gemini`, `aws`, `vertex`, `deepseek`, `minimax`, `xai`, `volcengine`, and others.
- Task-style providers are integrated through `relay.GetTaskAdaptor` and `service.GetTaskAdaptorFunc`.

**AWS Bedrock:**
- Code: `relay/channel/aws/adaptor.go`, `relay/channel/aws/relay-aws.go`, `relay/channel/aws/dto.go`, `relay/channel/aws/constants.go`.
- Supports API-key mode through HTTP Bedrock converse URL and AK/SK mode through AWS SDK Bedrock runtime.
- AK/SK mode builds `bedrockruntime.InvokeModelInput` or `InvokeModelWithResponseStreamInput`.
- Timeout is applied through `newAwsInvokeContext`, using `common.RelayTimeout` seconds when positive.

**OpenAI-compatible providers:**
- Generic OpenAI-compatible relay paths are handled through `relay.TextHelper` and channel adaptors.
- The relay supports OpenAI chat/completions/responses, Claude messages, Gemini compatible routes, image, audio, embedding, rerank, realtime, and task routes.

## Databases

**Supported engines:**
- SQLite via `github.com/glebarez/sqlite`.
- MySQL via `gorm.io/driver/mysql`.
- PostgreSQL via `gorm.io/driver/postgres`.

**Database layer:**
- Models and DB access live under `model/`.
- Cross-DB raw SQL helpers and reserved column names are centralized in `model/main.go`.
- Important log storage model: `model.Log` in `model/log.go`, including `request_id`, `upstream_request_id`, and `other`.

## Cache and Background Work

**Redis / memory cache:**
- Redis is controlled through `common.RedisEnabled`; enabling Redis also enables memory cache compatibility in `main.go`.
- Channel cache is initialized by `model.InitChannelCache()` and refreshed by `model.SyncChannelCache`.

**Background tasks:**
- Options sync: `model.SyncOptions`.
- Quota dashboard: `model.UpdateQuotaData`.
- Channel testing and upstream model update tasks: `controller.AutomaticallyTestChannels`, `controller.StartChannelUpstreamModelUpdateTask`.
- Task polling: `controller.UpdateMidjourneyTaskBulk`, `controller.UpdateTaskBulk`.
- Subscription quota reset: `service.StartSubscriptionQuotaResetTask`.

## Auth, Payments, and External Services

**Authentication:**
- JWT auth middleware and token auth live in `middleware/auth.go`.
- WebAuthn/passkey support lives under `service/passkey/` and `model/passkey.go`.
- OAuth integrations live under `oauth/` and provider controllers such as `controller/github.go`, `controller/discord.go`, `controller/oidc.go`.

**Payments:**
- Stripe SDK is used by subscription/topup controllers.
- Waffo, Waffo Pancake, Epay, and Creem payment paths exist in `controller/`, `service/`, `setting/`, and tests.

## HTTP and Network

**Shared HTTP client:**
- Initialized in `service.InitHttpClient`.
- Uses `http.Transport` pool settings from `common`.
- `RelayTimeout` becomes `http.Client.Timeout` when non-zero.
- Redirects are validated through `common.ValidateURLWithFetchSetting` to enforce SSRF policy.

**Proxy support:**
- `service.NewProxyHttpClient` supports `http`, `https`, `socks5`, and `socks5h`.
- Proxy clients are cached by proxy URL and reuse transport pools.

## Observability

**Request IDs:**
- `middleware.RequestId` creates a local request ID and stores it in Gin context and request context.
- Upstream request ID is captured from `X-Oneapi-Request-Id` by `service.ShouldCopyUpstreamHeader`.

**Logs:**
- Gin access log setup is in `middleware/logger.go`.
- Business consume/error logs are written through `model.RecordConsumeLog` and `model.RecordErrorLog`.
- `logs.other` stores JSON-like auxiliary data.

## Deployment Integrations

**CI/CD:**
- Docker image publishing: `.github/workflows/docker-build.yml`, `.github/workflows/docker-image-alpha.yml`, `.github/workflows/docker-image-nightly.yml`.
- Binary release: `.github/workflows/release.yml`.
- Electron build: `.github/workflows/electron-build.yml`.

---
*Integration analysis: 2026-06-06*
*Update when external services or provider contracts change*
