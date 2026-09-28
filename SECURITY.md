# Security Policy

## Supported versions

Only `main` is supported during early development. Pin to tagged releases once v0.1.0 ships.

## Reporting a vulnerability

Open a GitHub Security Advisory or email the maintainers (see `CODEOWNERS` once added). Do not open a public issue for sensitive reports.

We aim to acknowledge within 48h and ship a fix or mitigation within 14 days.

## Expectations

- Never commit secrets (`.env`, API keys, JWT secrets). CI scans for leaked patterns.
- Backend runs as nonroot distroless; Postgres/Redis only bind to localhost in dev unless explicitly exposed.
- Dependencies are pinned via `go.sum` / `package-lock.json`; review Renovate/Dependabot PRs promptly.
