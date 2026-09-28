# Development guide

This is the full contributor reference: architecture, workflows, testing, and extension points. For using a running instance, see the [README](../README.md). For the upstream API contract, see [langfuse-reference.md](langfuse-reference.md).

## 1. Architecture

```
                  ┌─────────────┐
  SDKs / OTEL ──▶ │  Fiber API  │──▶ Redis Stream ──▶  Go worker ──▶ Postgres
  Browser ──────▶ │  (cmd/api)  │     (traceprompt:ingest)  (cmd/worker)
                  └─────────────┘
```

- **Synchronous paths** (auth, CRUD, reads, playground proxy, single-score): request → Postgres → response.
- **Asynchronous path** (bulk trace ingestion): `POST /api/public/ingestion` and `POST /api/public/otel/v1/traces` validate, enqueue to Redis Streams, and return immediately (`207` / `200`). The worker drains via consumer group, applies idempotent upserts, ACKs; failures retry, poison events go to the `traceprompt:ingest:dlq` stream after 5 attempts.
- **No Redis?** The API pings Redis at boot; if unreachable it uses an in-memory queue drained by an inline goroutine (single-binary local dev). Compose always runs real Redis.
- **Postgres is the only store.** JSON-ish columns use GORM `serializer:json` (`jsonb` for metadata/config, `text` for tags/labels/messages) so the same models run on SQLite in tests. If tag filtering outgrows sequential scans, migrate `tags` to `text[]` + GIN in a new goose migration.

## 2. Repository layout

```
Makefile                    # control plane (make help)
docker-compose.yml          # postgres16, redis7, api, worker, web
backend/
  cmd/api, cmd/worker       # entrypoints; both run goose migrations on boot
  internal/
    auth        # bcrypt, JWT (HS256/24h), pk-lf-/sk-lf- gen + SHA-256 check
    config      # env loading with defaults (Config.Load)
    db          # pg connect + pooling; Migrate() with embedded goose files
    db/migrations  # versioned schema: 00001_init, 00002_..., ...
    httpapi     # router (app.go), middleware, *handlers.go, *_test.go
    ingest      # legacy batch parse/validate (events.go) + upserts (store.go)
    models      # GORM entities (camelCase JSON for SDK compat)
    otel        # OTLP/HTTP JSON → ingestion events + attribute map
    playground  # {{var}} renderer + OpenAI-compatible client (keys never stored)
    queue       # Queue interface; Memory (tests/dev) + Redis Streams (prod)
    worker      # DrainMemory (inline) + RunRedis (consumer group + DLQ)
  Dockerfile, .golangci.yml
frontend/                   # Svelte 5 + Vite SPA, DaisyUI, svelte-spa-router
  src/lib/api.ts            # typed fetch client (all API access lives here)
  src/pages/                # one file per route
  src/lib/api.test.ts       # vitest unit; e2e/ (playwright)
reference/langfuse          # git-ignored upstream clone — read-only API reference
docs/, scripts/smoke.sh     # this guide, API map, stack smoke test
```

## 3. Daily workflows (`make help`)

| Command | What it does |
| --- | --- |
| `make setup` | `cp .env.example .env` (once) + `npm install` |
| `make up` / `up-detached` / `down` / `clean` | Compose lifecycle (`clean` deletes volumes) |
| `make test` | Backend vet+tests and frontend check+tests |
| `make test-backend-race` | Race detector (slower; run before big PRs) |
| `make test-e2e` | Playwright (Chromium; dev server auto-started) |
| `make lint` | gofmt check + golangci-lint (toolchain-pinned install) |
| `make build` | Production Docker images |
| `make smoke [API_URL=…]` | Live stack check: register → project → key → ingest → trace visible |
| `make migrate-new NAME=…` | Scaffold `NNNN_name.sql` with next sequence |
| `make reference-update` | Refresh upstream clone |

## 4. Backend guide

**Conventions:** no globals (inject `Deps`), handlers return JSON errors via `fiber.NewError`, request structs live next to handlers, `slog` for worker logs, `gofumpt` formatting, route-first doc comments (`// GET /api/…`).

**Adding an endpoint**
1. Define models first if storage is needed (see below).
2. Add the handler in the matching `*handlers.go` (or a new one for a new domain).
3. Wire the route in `app.go` behind the right middleware: `requireJWT` (+ `requireProjectMember` for project scope) or `requireAPIKey` for `/api/public`.
4. Add tests in `phaseN_test.go` style using `newTestApp(t)` (isolated SQLite), `register/createProject/createKey` helpers, and `drain()` to simulate the worker.

**Adding a table**
1. Add the struct to `internal/models/models.go` (UUID `Base`, camelCase JSON, `AllModels()`).
2. `make migrate-new NAME=add_foo`, write Postgres DDL with `IF NOT EXISTS` where practical. Serializer-backed columns must stay **NULLABLE** — GORM writes explicit NULL for nil slices/maps even with column defaults (this bit us in `00002`).
3. Tests keep using `AutoMigrate(AllModels())` on SQLite; verify on real Postgres via `make smoke` or a scratch container.

**Auth model:** UI sessions are JWT (`Authorization: Bearer`), SDK traffic is BasicAuth `pk:sk` hashed with SHA-256 (secret shown once at creation). Revoked keys fail closed. `JWT_SECRET` must not be the placeholder (enforced at issue time).

**Queue/worker notes:** `store.Apply` must stay idempotent (upserts on `trace_id`/`observation_id`/`external_id`) because delivery is at-least-once. Score schema violations return `*ingest.PermanentError` → immediate DLQ, no retry. Single-score `POST /api/public/scores` maps permanent errors to `400`.

**Rate limiting:** per-IP sliding window on `/api/public` only (`PUBLIC_RATE_LIMIT_PER_MIN`, default 300, `0` disables). Health and UI routes are unlimited.

## 5. Frontend guide

**Conventions:** all HTTP lives in `src/lib/api.ts` (URL builders are pure and unit-tested; `fetch` wrappers take `base` + token args). Pages are route components in `src/pages/`; routing in `App.svelte` via `svelte-spa-router`. Styling is DaisyUI classes; no data-grid library — tables are server-paginated. Secrets (provider keys) stay in component state, never `localStorage`.

**Adding a page**
1. Add API helpers to `lib/api.ts` + pure-function tests in `lib/api.test.ts` (stub `global.fetch` with `vi.stubGlobal` for wrapper tests).
2. Create `src/pages/X.svelte` following `Projects.svelte` (project picker via `currentProjectId` store, error alert pattern).
3. Register the route + navbar link in `App.svelte`; extend `e2e/app.spec.ts` nav coverage.

**Auth state:** `token` and `currentProjectId` Svelte stores mirror `localStorage`. Login writes the token; logout clears it.

## 6. Testing strategy

| Layer | Tool | Location | Notes |
| --- | --- | --- | --- |
| Go unit/integration | `testify` + SQLite | `internal/*/*_test.go` | `newTestApp` isolates via drop+migrate; `drain()` simulates worker |
| Go race | `-race` | `make test-backend-race` | Catches queue/worker data races |
| Lint | `golangci-lint` v2 | `backend/.golangci.yml` | errcheck, govet, staticcheck, unused, gosec, misspell, gofumpt; ST1020 off (route-first comments), `shadow` off (noisy) |
| Frontend unit | `vitest` | `src/lib/api.test.ts` | Pure helpers + stubbed-fetch wrappers |
| E2E | `playwright` | `frontend/e2e/` | No-backend smoke (headings, nav, forms render) |
| Live | throwaway containers | ad-hoc python / `make smoke` | Fresh Postgres+Redis prove migrations + async path |
| CI | GitHub Actions | `.github/workflows/ci.yml` | Backend (fmt/vet/test/lint) + frontend (check/test/build/e2e) |

Coverage is a signal, not a gate: `go test -coverprofile` should show new handlers/models/lint-adjacent code covered; Redis-client internals and `db.Open` are verified live instead of mocked.

## 7. Gotchas we've already hit

1. **GORM `Update("col", slice)` skips the JSON serializer** — writes driver-stringified text. Use struct `Save()` for serializer columns (`prompt_handlers.go:moveLabel`).
2. **GORM writes NULL for nil slices/maps despite column DEFAULTs** — keep serializer columns nullable in migrations (`00002`).
3. **Go map iteration is random** — sort API outputs (`byModel`) or tests flake (~1/4 runs caught this).
4. **Fiber v2 has no `c.BasicAuth()`** — parse the header manually (`middleware.go:parseBasicAuth`).
5. **`golangci-lint` must be built with ≥ the repo toolchain** — the Makefile pins `GOTOOLCHAIN` from `go.mod`; CI uses setup-go 1.26.
6. **Svelte parses `{{…}}` in attributes** — avoid mustache examples in placeholders/attributes.
7. **SQLite shared-cache tests share one DB** — every test setup drops + re-migrates; never assume empty state.

## 8. Release checklist

1. `make lint && make test && make test-backend-race && make test-e2e && make build`
2. Fresh-DB boot check (migrations 0→latest) + `make smoke`
3. Bump nothing by hand — tags (`v0.x.y`) are the releases; migrations must stay append-only and backward-compatible with the previous image (expand before contract).
4. Update README feature matrix if user-facing behavior changed.
