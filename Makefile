SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help dev test lint build clean

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-6s %s\n", $$1, $$2}'

dev: .env web/node_modules ## Start PostgreSQL, the API (:8080) and Vite (:5173)
	docker compose up -d --wait db
	@set -a; . ./.env; set +a; \
	trap 'kill 0' EXIT; \
	go run -ldflags "-X main.version=$(VERSION)" ./cmd/api & \
	npm --prefix web run dev; \
	wait

test: ## Run Go tests (integration tests too if the dev database is running)
	@if [ -f .env ] && docker compose ps --status running --services 2>/dev/null | grep -qx db; then \
		set -a; . ./.env; set +a; export QR_TEST_DATABASE_URL="$$DATABASE_URL"; \
		echo "make test: dev PostgreSQL is up, running integration tests too"; \
	fi; \
	go test -race -count=1 ./...

lint: web/node_modules ## gofmt, go vet, oxlint and TypeScript type check
	@unformatted=$$(gofmt -l $$(go list -f '{{.Dir}}' ./...)); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	npm --prefix web run lint
	npm --prefix web run typecheck

build: web/node_modules ## Build bin/api, bin/worker and web/dist
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...
	npm --prefix web run build

clean: ## Remove build output
	rm -rf bin web/dist

.env:
	cp .env.example .env
	@echo "Created .env from .env.example (local development values)."

web/node_modules: web/package-lock.json
	npm --prefix web ci --no-fund --no-audit
	@touch $@
