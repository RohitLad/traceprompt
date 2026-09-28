# Langfuse reference map

Upstream: `reference/langfuse` (git-ignored, `--depth=1` clone). **Do not copy code** — use only to confirm API/data-model shapes. All Traceprompt code is original MIT.

## Where to look upstream

| Concern | Upstream path |
| --- | --- |
| Public REST contract (ingestion, observations v2, scores v3) | `web/public/generated/api-client/openapi.yml`, `packages/shared` |
| Ingestion batch semantics (207 + per-event errors, 3.5MB cap) | `worker/`, `packages/shared/src/server/ingestion` |
| Data model (trace → observations → scores, sessions) | `web/src/__tests__`, Prisma schema under `web/` + `docs/observability/data-model` at https://langfuse.com/docs/observability/data-model |
| OTel mapping (`x-langfuse-*` attrs) | `packages/` OTEL docs + https://langfuse.com/docs/api |
| Prompt versioning/labels | `web/src/features/prompts` |
| Auth (org/project/api keys `pk-lf-`/`sk-lf-`) | `web/src/features/auth`, `ee/` (reference only — reimplement OSS-safe subset) |

## Traceprompt mapping (Phase 0 → 5)

- Phase 1: `internal/auth` (JWT + BasicAuth pk/sk), `projects`, `api_keys`
- Phase 2: `POST /api/public/ingestion` → Redis Stream `ingest` → worker → Postgres `traces`/`observations`; `GET /api/public/v2/observations`
- Phase 3: `POST /api/public/otel/v1/traces`, `POST/GET /api/public/scores` + v3 read, sessions UI
- Phase 4: prompts CRUD + `get_prompt` cache semantics + playground proxy
- Phase 5: datasets/experiments, metrics aggregations in SQL (no ClickHouse yet)

Refresh the reference with: `git -C reference/langfuse pull --depth=1` (or re-clone).
