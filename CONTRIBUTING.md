# Contributing to Traceprompt

Thanks for considering a contribution — this project aims to stay lean, tested, and Langfuse-compatible.

## Workflow

1. Fork + branch from `main` (`feat/...`, `fix/...`, `docs/...`).
2. Keep changes small and focused. No drive-by refactors.
3. Add/extend tests for every behavior change (Go `testify`, frontend `vitest`, e2e `playwright` for UI flows).
4. Run locally before pushing:
   ```bash
   cd backend && gofmt -l . && go vet ./... && go test ./...
   cd frontend && npm run check && npm test
   ```
5. Conventional commits (`feat:`, `fix:`, `docs:`, `chore:`, `test:`). Squash-merge.

## Code standards

- Go: `gofumpt`, `golangci-lint`, structured `slog`, no globals for config/DB, context-propagated.
- Svelte: Svelte 5 runes, DaisyUI classes, no TanStack unless justified in PR description.
- Never copy code from `reference/langfuse` — use it only to confirm API shapes. All code here must be original (MIT).

## Security

Do not commit secrets. Use `.env.example` placeholders. Report vulnerabilities via `SECURITY.md`.
