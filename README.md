# Traceprompt

Open-source LLM observability, prompt management, and evaluation — a Langfuse-compatible clone built with **Go + Fiber + GORM + Postgres + Redis** and **Svelte + Vite + DaisyUI**.

## Features

| Area | Status | Notes |
| --- | --- | --- |
| Tracing (legacy batch ingestion, 207 semantics) | ✅ | `POST /api/public/ingestion`, span/generation/event aliases |
| OTel ingestion (OTLP/HTTP) | ✅ | `POST /api/public/otel/v1/traces`, `langfuse.*` + `gen_ai.*` mapping |
| Observations / scores reads (v2 / v3) | ✅ | Cursor pagination, typed score values |
| Projects, users, `pk-lf-`/`sk-lf-` API keys | ✅ | JWT UI auth, BasicAuth public API, revocation |
| Prompt management | ✅ | Immutable versions, movable `production`/`latest` labels, SDK fetch |
| Datasets + experiment runs | ✅ | Items, runs, item→trace links, public runner API |
| Metrics dashboard | ✅ | Counts, tokens, latency, per-day, by-model |
| Playground | ✅ | OpenAI-compatible proxy, keys never stored, optional trace logging |
| Sessions / scores UI | ✅ | Grouped conversations, evaluation views |
| Versioned Postgres migrations | ✅ | goose, embedded, run on boot |
| Public API rate limiting | ✅ | Per-IP sliding window (`PUBLIC_RATE_LIMIT_PER_MIN`) |

## Quickstart

```bash
cp .env.example .env   # set JWT_SECRET + POSTGRES_PASSWORD
docker compose up --build
# api:      http://localhost:3000/api/health
# frontend: http://localhost:5173
```

Register at `http://localhost:5173/#/login`, create a project, mint an API key under **Keys**, then send your first trace:

```bash
curl -u pk-lf-...:sk-lf-... http://localhost:3000/api/public/ingestion \
  -H 'Content-Type: application/json' -d '{"batch":[
    {"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z",
     "body":{"id":"t1","name":"hello","userId":"u1"}},
    {"id":"e2","type":"generation-create","timestamp":"2026-01-01T00:00:01Z",
     "body":{"id":"o1","traceId":"t1","name":"llm","model":"gpt-4o-mini",
             "input":"hi","output":"hello","usage":{"input":5,"output":7,"total":12}}}
  ]}'
```

Point any OpenTelemetry SDK at `POST /api/public/otel/v1/traces` with BasicAuth, or fetch a prompt from your app:

```bash
curl -u pk-lf-...:sk-lf-... http://localhost:3000/api/public/prompts/greeting
```

## Layout

```
backend/                  Go + Fiber API + worker
  cmd/api | cmd/worker    Entrypoints (migrations run on boot)
  internal/{auth,config,db,httpapi,ingest,models,otel,playground,queue,worker}
  internal/db/migrations  Versioned Postgres schema (goose, embedded)
frontend/                 Svelte 5 + Vite SPA (DaisyUI, no data-grid lib)
reference/                Upstream langfuse/langfuse, git-ignored, reference only
docs/                     Architecture notes + Langfuse API mapping
docker-compose.yml        postgres16, redis7, api, worker, web
```

## Configuration

| Var | Default | Purpose |
| --- | --- | --- |
| `APP_ENV` | `development` | `production` disables debug logging |
| `PORT` | `3000` | API listen port |
| `DATABASE_URL` | local postgres | Postgres DSN (migrations auto-apply) |
| `REDIS_URL` | local redis | Streams queue; API falls back to in-memory drain if unreachable |
| `JWT_SECRET` | — | **Required.** UI session signing (rejects placeholder) |
| `PUBLIC_RATE_LIMIT_PER_MIN` | `300` | Per-IP cap on `/api/public` (0 disables) |
| `VITE_API_BASE_URL` | `http://localhost:3000` | Frontend → API |

## Development

```bash
# backend (needs Go 1.26+)
cd backend && go test ./... && go run ./cmd/api

# frontend (needs Node 24+)
cd frontend && npm install && npm run dev
npm test        # vitest unit
npm run test:e2e # playwright smoke
```

New Postgres tables go in `backend/internal/db/migrations/NNNN_name.sql` (goose format, `IF NOT EXISTS` where practical). Tests use SQLite AutoMigrate and are unaffected.

## Contributing

See `CONTRIBUTING.md`. Conventional commits, `gofumpt` + `golangci-lint`, `svelte-check`, tests required for every PR. Never copy code from `reference/langfuse` — API shapes only.
