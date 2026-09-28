# Traceprompt control plane. Every target is safe to re-run.
# Run `make help` for the list. Requires: docker, go 1.26+, node 24+.

SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE := docker compose
API_URL := http://localhost:3000
# Go toolchain from backend/go.mod (golangci-lint must be built with >= it).
GO_VER := $(shell grep '^go ' backend/go.mod | awk '{print $$2}')

## help: list targets
.PHONY: help
help:
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'

## setup: create .env and install frontend deps
.PHONY: setup
setup:
	test -f .env || cp .env.example .env
	@echo "-> review .env (JWT_SECRET, POSTGRES_PASSWORD)"
	cd frontend && npm install

## up: build and start the full stack
.PHONY: up
up:
	$(COMPOSE) up --build

## up-detached: start the full stack in background
.PHONY: up-detached
up-detached:
	$(COMPOSE) up --build -d

## down: stop the stack (keeps data volumes)
.PHONY: down
down:
	$(COMPOSE) down

## clean: stop the stack AND delete data volumes
.PHONY: clean
clean:
	$(COMPOSE) down -v

## test: backend + frontend unit tests
.PHONY: test
test: test-backend test-frontend

## test-backend: go vet + all backend tests
.PHONY: test-backend
test-backend:
	cd backend && go vet ./... && go test ./... -count=1

## test-backend-race: backend tests with race detector
.PHONY: test-backend-race
test-backend-race:
	cd backend && go test ./... -count=1 -race

## test-frontend: svelte-check + vitest
.PHONY: test-frontend
test-frontend:
	cd frontend && npm run check && npm test

## test-e2e: playwright browser tests (starts vite dev server itself)
.PHONY: test-e2e
test-e2e:
	cd frontend && npm run test:e2e

## lint: gofmt check + golangci-lint (installs it if missing)
.PHONY: lint
lint:
	test -z "$$(gofmt -l backend/)" || (gofmt -l backend/ && exit 1)
	cd backend && GOTOOLCHAIN="go$(GO_VER)" GOBIN="$$(go env GOPATH)/bin" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 && "$$(go env GOPATH)/bin/golangci-lint" run --timeout=5m ./...

## build: production docker images for api + web
.PHONY: build
build:
	docker build -f backend/Dockerfile -t traceprompt-api:local ./backend
	docker build -f frontend/Dockerfile -t traceprompt-web:local ./frontend

## smoke: verify health + auth + ingestion against a running stack
.PHONY: smoke
smoke:
	./scripts/smoke.sh $(API_URL)

## migrate-new: scaffold a goose migration (usage: make migrate-new NAME=add_foo)
.PHONY: migrate-new
migrate-new:
	test -n "$(NAME)" || (echo "usage: make migrate-new NAME=add_foo" && exit 1)
	@NEXT=$$(ls backend/internal/db/migrations/*.sql | sed 's/.*\///;s/_.*//' | sort -n | tail -1 | awk '{printf "%05d", $$1+1}'); \
	FILE="backend/internal/db/migrations/$${NEXT}_$(NAME).sql"; \
	printf -- '-- +goose Up\n-- %s\n\nSELECT 1;\n\n-- +goose Down\nSELECT 1;\n' "$(NAME)" > "$$FILE"; \
	echo "created $$FILE"

## reference-update: refresh the read-only upstream Langfuse clone
.PHONY: reference-update
reference-update:
	git -C reference/langfuse pull --depth=1 || git clone --depth=1 https://github.com/langfuse/langfuse.git reference/langfuse
