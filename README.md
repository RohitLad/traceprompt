# Traceprompt

**Open-source LLM observability, prompt management, and evaluation.** Trace every model call, version your prompts without redeploys, score quality, run experiments, and iterate in a playground — self-hosted in minutes with Docker.

Traceprompt speaks the [Langfuse](https://langfuse.com) API, so existing Langfuse SDK setups work by changing only the base URL.

## 5-minute quickstart

Prerequisites: Docker.

```bash
git clone https://github.com/<you>/traceprompt.git
cd traceprompt
cp .env.example .env   # then set JWT_SECRET and POSTGRES_PASSWORD
make up                # or: docker compose up --build
```

Open the UI at **http://localhost:5173**, register an account, and create a project. Under **Keys**, mint an API key — you'll get a public key (`pk-lf-…`) and a secret (`sk-lf-…`, shown once).

Send your first trace:

```bash
curl -u pk-lf-...:sk-lf-... http://localhost:3000/api/public/ingestion \
  -H 'Content-Type: application/json' -d '{"batch":[
    {"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z",
     "body":{"id":"trace-1","name":"support-chat","userId":"user-42"}},
    {"id":"e2","type":"generation-create","timestamp":"2026-01-01T00:00:01Z",
     "body":{"id":"obs-1","traceId":"trace-1","name":"answer","model":"gpt-4o-mini",
             "input":"How do I reset my password?","output":"Click Settings → Reset…",
             "usage":{"input":12,"output":9,"total":21}}}
  ]}'
```

Refresh **Traces** in the UI — your trace is there with latency, tokens, and cost-ready usage. Everything below builds on this loop.

## Using Traceprompt

### Observe

- **Traces** — every request with its nested steps (LLM calls, tool calls, retrievals). Filter by user, session, model; open any trace for the full timeline with inputs/outputs.
- **Sessions** — multi-turn conversations grouped by `sessionId`.
- **Scores** — quality signals on traces: log them from code, from eval runs, or by hand in a review queue.
- **Dashboard** — volume, tokens, latency, and per-model breakdowns.

Send data with any Langfuse SDK (Python/JS) pointed at `http://localhost:3000`, via the ingestion API above, or via native OpenTelemetry:

```bash
# OTLP/HTTP — works with any OTEL-instrumented app
curl -u pk-lf-...:sk-lf-... http://localhost:3000/api/public/otel/v1/traces \
  -H 'Content-Type: application/json' -d '{"resourceSpans":[…]}'
```

Use span attributes such as `langfuse.user.id`, `langfuse.session.id`, `langfuse.tags`, and `gen_ai.request.model` / `gen_ai.usage.input_tokens` to populate first-class fields.

### Iterate on prompts

Create prompts in **Prompts** (text or chat, with `{{variables}}`). Each save is an immutable version; move the `production` label to roll out without redeploying your app. Fetch the live version at runtime:

```bash
curl -u pk-lf-...:sk-lf-... http://localhost:3000/api/public/prompts/greeting
# {"name":"greeting","version":2,"prompt":"Hello {{name}}",…}
```

Test changes in the **Playground** against any OpenAI-compatible endpoint before promoting. Your provider key is sent per-request and never stored.

### Evaluate

1. Define allowed score schemas under **Evals** (e.g. `quality`: numeric 0–1). Out-of-schema scores are rejected at ingest instead of silently polluting your data.
2. Build test sets under **Datasets** (input + expected output), run your app against them, and link each result to its trace.
3. Send borderline production traces to an **annotation queue** for human review and scoring.

## Configuration

| Variable | Default | What it does |
| --- | --- | --- |
| `JWT_SECRET` | *(required)* | Signs UI sessions. Generate one: `openssl rand -hex 32`. The placeholder value is refused. |
| `POSTGRES_PASSWORD` | `traceprompt` | Database password (also used inside `DATABASE_URL` by compose). |
| `PORT` | `3000` | API port. |
| `APP_ENV` | `development` | Set `production` for quieter logs. |
| `PUBLIC_RATE_LIMIT_PER_MIN` | `300` | Per-IP cap on `/api/public` (SDK batching-friendly). `0` disables — dev only. |
| `VITE_API_BASE_URL` | `http://localhost:3000` | Where the web UI finds the API. |

Run `make help` for day-to-day commands (`make up`, `make test`, `make smoke`, `make clean`, …). Migrations apply automatically on boot.

## Self-hosting notes

- `docker compose up` runs Postgres 16, Redis 7, the API, the ingestion worker, and the web UI. Data lives in `postgres-data` / `redis-data` volumes; `make clean` deletes them.
- Back up Postgres (`pg_dump`) on your own schedule; Redis holds only transient ingestion batches.
- Put the stack behind TLS (Caddy, Traefik, or your cloud LB) before exposing it. Never commit `.env`.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| UI shows “backend unreachable” | API not running or `VITE_API_BASE_URL` wrong (rebuild web image after changing it). |
| Traces sent but not visible | Worker not running, or Redis unreachable — API logs `using in-memory queue` as a fallback hint. Wait a few seconds and refresh. |
| `401 invalid credentials` from SDKs | Wrong key pair, or the key was revoked under **Keys**. |
| `429 rate limit exceeded` | Lower SDK flush frequency or raise `PUBLIC_RATE_LIMIT_PER_MIN`. |
| Score rejected with `400` | Value violates the score schema defined under **Evals**. |

## Is it production-ready?

Traceprompt runs its full suite (Go unit + race detector, Svelte unit, Playwright, `golangci-lint`, Docker builds) on every PR, migrates its schema with versioned SQL, and dead-letters poison ingestion events instead of retrying forever. That said: it is a young project — review the [developer docs](docs/DEVELOPMENT.md), start with non-critical workloads, and please [report issues](https://github.com/<you>/traceprompt/issues) with reproduction steps.

MIT licensed. See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md).
