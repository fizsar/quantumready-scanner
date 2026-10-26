SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
# Local run of the production stack: build images (compose.build.yaml) tagged with the git version.
PROD := QR_TAG=$(VERSION) QR_VERSION=$(VERSION) QR_COMMIT=$(COMMIT) \
	docker compose -f deploy/compose.prod.yaml -f deploy/compose.build.yaml --env-file .env

# Pinned by digest; bump tag and digest together (CI uses the same image).
SQLC_IMAGE := sqlc/sqlc:1.31.1@sha256:70f53171d27b2424e9358869975455a6e955a5aa8e58a998a270a6e34e525537

.PHONY: help dev test lint build clean prod-up prod-down deploy sqlc check-compose check-files test-deploy test-backup b2-verify fuzz tlslab-up tlslab-check tlslab-testssl tlslab-down tlslab-test tlslab-scan

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-6s %s\n", $$1, $$2}'

dev: .env web/node_modules ## Start PostgreSQL, the API (:8080), the worker and Vite (:5173)
	docker compose up -d --wait db
	@set -a; . ./.env; set +a; \
	go run ./cmd/migrate && \
	trap 'kill 0' EXIT; \
	go run -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" ./cmd/api & \
	WORKER_HEARTBEAT_FILE=$${TMPDIR:-/tmp}/qr-worker-heartbeat go run ./cmd/worker & \
	npm --prefix web run dev; \
	wait

test: web/node_modules ## Run Go tests (integration tests too if the dev database is running) and web tests
	@if [ -f .env ] && docker compose ps --status running --services 2>/dev/null | grep -qx db; then \
		set -a; . ./.env; set +a; export QR_TEST_DATABASE_URL="$$DATABASE_URL"; \
		echo "make test: dev PostgreSQL is up, running integration tests too"; \
	fi; \
	go test -race -count=1 ./...
	npm --prefix web test

FUZZTIME ?= 1m
FUZZ_TARGETS := FuzzReadResponse FuzzParseServerHello FuzzParseServerKeyExchange FuzzReadSSLv2Response FuzzParseClientHello FuzzParseServerFlight
fuzz: ## Fuzz the raw TLS parsers (FUZZTIME per target, default 1m)
	@for t in $(FUZZ_TARGETS); do \
		echo "== $$t"; go test -run='^$$' -fuzz="^$$t$$" -fuzztime=$(FUZZTIME) ./internal/scanner/rawtls || exit 1; \
	done

lint: web/node_modules ## golangci-lint (incl. go vet, gofmt, goimports), oxlint and tsc
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found: https://golangci-lint.run/docs/welcome/install/"; exit 1; }
	golangci-lint run ./...
	npm --prefix web run lint
	npm --prefix web run typecheck

build: web/node_modules ## Build bin/{api,migrate,worker,qradmin,qrscan} and web/dist
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...
	npm --prefix web run build

sqlc: ## Regenerate internal/store from migrations/ and internal/store/queries/
	docker run --rm --user $$(id -u):$$(id -g) -v "$$PWD:/src" -w /src $(SQLC_IMAGE) generate

check-compose: ## Check the security policy of deploy/compose.prod.yaml (networks, limits, ports, digests) and that every image is pinned
	QR_TAG=check POSTGRES_USER=u POSTGRES_PASSWORD=p POSTGRES_DB=d \
	docker compose -f deploy/compose.prod.yaml config --format json | python3 deploy/check_compose.py
	python3 deploy/check_images.py

check-files: ## No executable binaries and no files over 1 MB in the repository (exceptions in deploy/check_files.py)
	python3 deploy/check_files.py

test-deploy: ## Test the deploy helpers without the VPS (deploy.sh refs, check-installed.sh, predeploy-dump.sh, image pins, compose plan, repository files)
	bash deploy/test-deploy-ref.sh
	bash deploy/test-check-installed.sh
	bash deploy/test-predeploy-dump.sh
	bash deploy/test-check-images.sh
	bash deploy/test-compose-plan.sh
	bash deploy/test-check-files.sh

test-backup: ## Test the backup scripts (local restic repository, throwaway PostgreSQL)
	bash deploy/backup/test.sh

b2-verify: ## Check a no-deleteFiles B2 key with restic (needs ~/.config/quantumready/b2-test.env)
	bash deploy/backup/b2-verify.sh

clean: ## Remove build output
	rm -rf bin web/dist

prod-up: .env ## Build and start the production stack locally (127.0.0.1:8080 and :8081)
	@docker network inspect web >/dev/null 2>&1 || docker network create web
	QR_TRUSTED_PROXIES=$$(docker network inspect web -f '{{range .IPAM.Config}}{{.Subnet}},{{end}}' | sed 's/,$$//') \
	$(PROD) up -d --build --wait

prod-down: ## Stop the production stack (keeps the database volume)
	$(PROD) down

deploy: ## Deploy to the VPS: make deploy TAG=sha-xxxxxxx (or TAG=latest)
	@test -n "$(TAG)" || { echo "usage: make deploy TAG=sha-xxxxxxx|latest"; exit 2; }
	bash deploy/deploy.sh $(TAG)

.env:
	cp .env.example .env
	@echo "Created .env from .env.example (local development values)."

web/node_modules: web/package-lock.json
	npm --prefix web ci --no-fund --no-audit
	@touch $@

tlslab-up: ## Start the TLS test lab (weak servers, internal network only) and check its isolation
	bash test/tlslab/lab.sh up

tlslab-check: ## Check the TLS test lab isolation again
	bash test/tlslab/lab.sh check

tlslab-testssl: ## Reference run of testssl.sh against every lab server (test/tlslab/reference)
	bash test/tlslab/lab.sh testssl

tlslab-down: ## Remove the TLS test lab and check nothing is left (TLSLAB_RMI=1 also removes its images)
	bash test/tlslab/lab.sh down

tlslab-test: ## Integration tests of the scanner against the TLS lab, compared with testssl.sh (needs make tlslab-up)
	bash test/tlslab/lab.sh gotest

tlslab-scan: ## qrscan -lab inside the TLS lab: make tlslab-scan ARGS="-facts tls13"
	bash test/tlslab/lab.sh scan $(ARGS)
