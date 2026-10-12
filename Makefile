SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
# Local run of the production stack: build images (compose.build.yaml) tagged with the git version.
PROD := QR_TAG=$(VERSION) QR_VERSION=$(VERSION) QR_COMMIT=$(COMMIT) \
	docker compose -f deploy/compose.prod.yaml -f deploy/compose.build.yaml --env-file .env

.PHONY: help dev test lint build clean prod-up prod-down deploy

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-6s %s\n", $$1, $$2}'

dev: .env web/node_modules ## Start PostgreSQL, the API (:8080) and Vite (:5173)
	docker compose up -d --wait db
	@set -a; . ./.env; set +a; \
	go run ./cmd/migrate && \
	trap 'kill 0' EXIT; \
	go run -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" ./cmd/api & \
	npm --prefix web run dev; \
	wait

test: ## Run Go tests (integration tests too if the dev database is running)
	@if [ -f .env ] && docker compose ps --status running --services 2>/dev/null | grep -qx db; then \
		set -a; . ./.env; set +a; export QR_TEST_DATABASE_URL="$$DATABASE_URL"; \
		echo "make test: dev PostgreSQL is up, running integration tests too"; \
	fi; \
	go test -race -count=1 ./...

lint: web/node_modules ## golangci-lint (incl. go vet, gofmt, goimports), oxlint and tsc
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found: https://golangci-lint.run/docs/welcome/install/"; exit 1; }
	golangci-lint run ./...
	npm --prefix web run lint
	npm --prefix web run typecheck

build: web/node_modules ## Build bin/{api,migrate,worker} and web/dist
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...
	npm --prefix web run build

clean: ## Remove build output
	rm -rf bin web/dist

prod-up: .env ## Build and start the production stack locally (127.0.0.1:8080 and :8081)
	@docker network inspect web >/dev/null 2>&1 || docker network create web
	$(PROD) up -d --build --wait

prod-down: ## Stop the production stack (keeps the database volume)
	$(PROD) down

deploy: ## Deploy to the VPS: make deploy TAG=sha-xxxxxxx (or TAG=latest)
	@test -n "$(TAG)" || { echo "usage: make deploy TAG=sha-xxxxxxx|latest"; exit 2; }
	deploy/deploy.sh $(TAG)

.env:
	cp .env.example .env
	@echo "Created .env from .env.example (local development values)."

web/node_modules: web/package-lock.json
	npm --prefix web ci --no-fund --no-audit
	@touch $@
