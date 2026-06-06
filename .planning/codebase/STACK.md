# Technology Stack

**Analysis Date:** 2026-06-06

## Languages

**Primary:**
- Go 1.25.1 - backend API gateway, relay layer, billing, auth, persistence, background tasks. The module is `github.com/QuantumNous/new-api` in `go.mod`.
- TypeScript - default and classic frontend implementations under `web/default/` and `web/classic/`.

**Secondary:**
- JavaScript / ESM - frontend tooling and scripts such as `web/default/scripts/sync-i18n.mjs`.
- Shell - helper scripts under `bin/` and CI commands in `.github/workflows/`.

## Runtime

**Environment:**
- Go runtime >= 1.25.1 for release workflows; `go.mod` declares `go 1.25.1`.
- Bun is the preferred frontend package manager. CI uses `oven-sh/setup-bun` and `bun install`.
- Node.js is used for Electron packaging and some frontend tooling.

**Package Manager:**
- Backend: Go modules via `go.mod` / `go.sum`.
- Frontend: Bun workspace under `web/`, with `web/bun.lock`.
- Electron: npm under `electron/package.json`.

## Frameworks

**Core Backend:**
- Gin v1.9.1 - HTTP routing and middleware, initialized in `main.go`.
- GORM v1.25.2 - database access in `model/`, with MySQL, PostgreSQL, and SQLite drivers.
- go-redis v8.11.5 - Redis cache and distributed behavior.
- AWS SDK Go v2 - Bedrock runtime integration in `relay/channel/aws/`.
- go-i18n v2 - backend i18n under `i18n/`.

**Core Frontend:**
- React 19 and TypeScript - default frontend in `web/default/`.
- Rsbuild - frontend build/dev server via `web/default/rsbuild.config.ts`.
- Base UI, Tailwind CSS, Hugeicons, TanStack Router, TanStack Query, Zustand.
- i18next and react-i18next - frontend i18n under `web/default/src/i18n/`.

**Testing:**
- Go `testing` plus `stretchr/testify` for backend unit tests.
- Frontend scripts include `typecheck`, `lint`, `format:check`; component tests exist under `web/default/src/**/*.test.tsx`.

**Build/Dev:**
- Backend build: `go build`, embedding frontend dist via `//go:embed` in `main.go`.
- Default frontend: `cd web/default && bun run build`.
- Classic frontend: `cd web/classic && bun run build`.
- Docker multi-arch build and release workflows under `.github/workflows/`.

## Key Dependencies

**Critical Backend:**
- `github.com/gin-gonic/gin` - request routing and middleware chain.
- `gorm.io/gorm` plus `gorm.io/driver/mysql`, `gorm.io/driver/postgres`, `github.com/glebarez/sqlite` - cross-DB persistence.
- `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` - AWS Bedrock Claude/Nova relay.
- `github.com/golang-jwt/jwt/v5` and `github.com/go-webauthn/webauthn` - auth and passkeys.
- `github.com/tiktoken-go/tokenizer` - token estimation.
- `github.com/expr-lang/expr` - expression-based billing support.

**Critical Frontend:**
- `@tanstack/react-router` - typed route tree.
- `@tanstack/react-query` - server state.
- `axios` - shared API client.
- `react-hook-form` and `zod` - forms and validation.
- `tailwindcss` and `@base-ui/react` - UI implementation.

## Configuration

**Environment:**
- Backend configuration is heavily environment-driven through `common/`, `setting/`, and system option sync in `model.SyncOptions`.
- HTTP relay timeout and pooling are controlled by `common.RelayTimeout`, `common.RelayMaxIdleConns`, `common.RelayMaxIdleConnsPerHost`, and `common.RelayIdleConnTimeout`.
- Proxy support is handled by `service.NewProxyHttpClient`.

**Build:**
- Backend entry: `main.go`.
- Frontend config: `web/default/rsbuild.config.ts`, `web/default/tsconfig.json`, `web/default/eslint.config.js`.
- Docker: `Dockerfile`, `Dockerfile.dev`, `docker-compose.yml`, `docker-compose.dev.yml`.

## Platform Requirements

**Development:**
- Go toolchain matching `go.mod`.
- Bun for frontend work.
- Optional Docker for containerized deployment and local stack checks.
- Supported databases: SQLite, MySQL >= 5.7.8, PostgreSQL >= 9.6.

**Production:**
- Single Go binary can embed frontend assets.
- Docker images are built for linux/amd64 and linux/arm64.
- Redis is optional but enables distributed/cache behavior.
- High-RPM relay deployments should tune HTTP client pool sizes, timeout, Redis, DB indexes, and log write paths together.

---
*Stack analysis: 2026-06-06*
*Update after major dependency or runtime changes*
