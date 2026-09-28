# Traceprompt

Open-source LLM observability, prompt management, and evaluation — a Langfuse-compatible clone built with **Go + Fiber + GORM + Postgres + Redis** and **Svelte + Vite + DaisyUI**.

> Status: early scaffold (Phase 0). See `docs/` for the build plan. Upstream Langfuse is vendored read-only under `reference/langfuse` for API/data-model reference only.

## Quickstart

```bash
cp .env.example .env
docker compose up --build
# api:      http://localhost:3000/api/health
# frontend: http://localhost:5173
```

## Layout

```
backend/        Go + Fiber API + worker (GORM, Postgres, Redis)
frontend/       Svelte + Vite SPA (DaisyUI, no TanStack)
reference/      Upstream langfuse/langfuse, git-ignored, reference only
docs/           Architecture notes + Langfuse API mapping
docker-compose.yml
```

## Langfuse compatibility

Target endpoints (phased):

- `POST /api/public/ingestion` — legacy batch ingestion (207 semantics)
- `POST /api/public/otel/v1/traces` — OTLP/HTTP ingestion
- `GET /api/public/v2/observations`, `GET /api/public/v3/scores`, prompts/datasets APIs
- BasicAuth `pk-lf-...:sk-lf-...` for public API, JWT for UI API

See `docs/langfuse-reference.md` and `reference/langfuse` for the upstream contract.

## Development

```bash
# backend
cd backend && go test ./... && go run ./cmd/api

# frontend
cd frontend && npm install && npm run dev
npm test        # vitest unit
npm run test:e2e # playwright smoke (traces list/detail)
```

## Contributing

See `CONTRIBUTING.md`. Conventional commits, `gofumpt` + `golangci-lint`, `svelte-check`, tests required for every PR.
