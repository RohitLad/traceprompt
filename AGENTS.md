# AGENTS.md — AI agent guide for Traceprompt

Read this file before writing any code in this repo. It distills everything an agent needs: stack, commands, conventions, architecture, and hard-won gotchas. Human deep-dives live in `docs/DEVELOPMENT.md` (architecture/workflows) and `docs/langfuse-reference.md` (upstream API map).

## What this is

Traceprompt is an open-source, Langfuse-compatible LLM observability / prompt-management / evaluation platform. Go + Fiber + GORM + Postgres + Redis backend, Svelte 5 + Vite + DaisyUI SPA frontend. SDKs authenticate with `pk-lf-…:sk-lf-…` BasicAuth; the browser UI uses JWT.

**Golden rule:** `reference/langfuse/` is a read-only upstream clone for confirming API shapes. Never copy code from it — all code here is original MIT.

## Commands (`make help` for all)

```bash
make test            # backend vet+tests AND frontend check+tests (run before every change ships)
make test-backend-race
make lint            # gofmt + golangci-lint (toolchain-pinned; must report 0 issues)
make test-e2e        # Playwright
make up / make down  # full docker stack (postgres16, redis7, api, worker, web)
make smoke           # live stack check against http://localhost:3000
make migrate-new NAME=add_foo   # scaffold goose migration
```

Toolchains: Go ≥1.26 (see `backend/go.mod`), Node 24+. CI (`.github/workflows/ci.yml`) runs fmt/vet/test/lint + check/test/build/e2e on every PR.

## Backend (`backend/`, module `github.com/traceprompt/traceprompt/backend`)

```
cmd/api, cmd/worker          # entrypoints; BOTH run goose migrations on boot
internal/auth                # bcrypt, JWT HS256/24h, pk/sk gen + SHA-256 verify
internal/config              # env → Config (no globals; Load() once, inject)
internal/db                  # pg connect/pool; Migrate() over embedded goose SQL
internal/db/migrations       # versioned schema — source of truth for Postgres
internal/httpapi             # Fiber router + middleware + *handlers.go + tests
internal/ingest              # legacy batch parse/validate + idempotent upserts
internal/models              # GORM entities, camelCase JSON (SDK compat)
internal/otel                # OTLP/HTTP JSON → ingestion events
internal/playground          # {{var}} renderer + OpenAI-compatible client
internal/queue               # Queue iface; Memory (tests/dev) + Redis Streams (prod)
internal/worker              # DrainMemory + RunRedis (consumer group, DLQ)
```

Conventions (enforced by `backend/.golangci.yml`: errcheck, govet, staticcheck, unused, gosec, misspell, gofumpt):

- **No globals.** Handlers hang off `Handler{db, jwtSecret, queue}` built from `Deps`. Config via `config.Load()`.
- **Routes** go in `app.go`: JWT `requireJWT` (+ `requireProjectMember` for project scope) for `/api/v1`, BasicAuth `requireAPIKey` for `/api/public`. Errors via `fiber.NewError` (JSON envelope is automatic).
- **Doc comments on handlers are route-first** (`// GET /api/…`) — ST1020 is intentionally disabled; keep that style.
- **Ingestion is async**: handlers validate → enqueue → `207`/`200`. The worker applies via `store.Apply`, which **must stay idempotent** (upserts on `trace_id`/`observation_id`/`external_id`; at-least-once delivery). Schema violations return `*ingest.PermanentError` → immediate DLQ, no retry.
- **Provider keys are never stored** (playground takes them per-request). API secrets are stored as SHA-256, shown once at creation. Revoked keys fail closed.
- **Migrations are append-only**, `IF NOT EXISTS` where practical. Serializer-backed columns **must stay NULLABLE** — GORM writes explicit NULL for nil slices/maps despite column DEFAULTs. Tests use SQLite `AutoMigrate(AllModels())`; always verify schema changes on real Postgres (`make smoke` or a scratch container).
- **GORM single-column `Update()` bypasses the JSON serializer** (writes driver-stringified text). Mutate the struct and `Save()` instead.
- **Sort API list outputs deterministically** — Go map iteration randomizes order and flakes tests.
- Fiber v2 has **no `c.BasicAuth()`** — use `parseBasicAuth` in `middleware.go`.
- Type names use full initialisms (`APIKey`, not `ApiKey`). No underscores in Go identifiers. Package comment on one file per package.

## Frontend (`frontend/`, Svelte 5 + Vite + DaisyUI)

- **All HTTP lives in `src/lib/api.ts`** — pure URL builders (unit-tested) + `fetch` wrappers taking `(base, token, …)`. Never fetch from components directly.
- Pages in `src/pages/`, routes + navbar in `App.svelte` (`svelte-spa-router`). Auth state: `token` / `currentProjectId` stores mirrored to `localStorage`.
- **No data-grid library** (no TanStack) — plain tables with server-side cursor pagination. No chart lib — CSS bars.
- Provider API keys stay in component state; never persist secrets.
- Svelte parses `{{…}}` in attributes — keep mustache examples out of placeholders/attributes.

## Tests (required for every behavior change)

- Backend: `testify` + isolated SQLite apps via `newTestApp(t)` (`drop + AutoMigrate` per test); `register/createProject/createKey` helpers; `drain()` simulates the worker. Name new files `phaseN_test.go`-style by domain, or `<pkg>_test.go` for unit tests.
- Assert status codes **and** shapes; cover authz boundaries (401/403/404), validation (400), conflicts (409).
- Frontend: pure helpers + stubbed-`fetch` wrapper tests in `src/lib/api.test.ts` (`vi.stubGlobal`); render/nav smoke in `e2e/app.spec.ts`.
- Flaky test? Run the package 6× (`for i in $(seq 1 6); do go test …`). Suspect shared SQLite state or map ordering first.

## Git

- Conventional commits (`feat:`, `fix:`, `chore:`, `test:`, `docs:`). Small, focused commits; never commit secrets (`.env` is git-ignored), `reference/`, `dist/`, or `node_modules/`.
- Never `git push --force`, never amend others' commits.

## Common tasks

| Task | How |
| --- | --- |
| New endpoint | Handler in matching `*handlers.go` → route in `app.go` with correct middleware → tests |
| New table | Struct in `models.go` + `AllModels()` → `make migrate-new` DDL (nullable serializer cols) → tests → live Postgres verify |
| New UI page | Helpers in `lib/api.ts` + tests → `src/pages/X.svelte` (project picker + error alert pattern) → route + navbar → e2e nav case |
| New migration | `make migrate-new NAME=…`, goose format, `IF NOT EXISTS`, verify 0→latest on fresh DB |
| Debug ingestion | Check worker logs → Redis stream length → DLQ stream (`traceprompt:ingest:dlq`) |

## Known gotchas (all bitten before — don't re-learn)

1. Single-column GORM `Update()` skips JSON serializer → use struct `Save()`.
2. GORM writes NULL for nil slices/maps despite DEFAULTs → nullable serializer columns.
3. Map iteration randomizes API output → sort before responding.
4. No `c.BasicAuth()` in Fiber v2 → manual header parse.
5. `golangci-lint` must be built with ≥ repo toolchain → Makefile pins `GOTOOLCHAIN` from `go.mod`.
6. `{{…}}` in Svelte attributes parses as expressions → avoid in placeholders.
7. SQLite test DBs are shared-cache → every setup drops + re-migrates; never assume emptiness.
