SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
# Local run of the production stack: build images (compose.build.yaml) tagged with the git version.
PROD := QR_TAG=$(VERSION) QR_VERSION=$(VERSION) QR_COMMIT=$(COMMIT) \
	QR_API_IMAGE=ghcr.io/fizsar/quantumready-saas-api:$(VERSION) QR_WEB_IMAGE=ghcr.io/fizsar/quantumready-saas-web:$(VERSION) \
	docker compose -f deploy/compose.prod.yaml -f deploy/compose.build.yaml --env-file .env

# Pinned by digest; bump tag and digest together (CI uses the same image).
SQLC_IMAGE := sqlc/sqlc:1.31.1@sha256:70f53171d27b2424e9358869975455a6e955a5aa8e58a998a270a6e34e525537

.PHONY: help dev psl signup-code test lint build clean prod-up prod-down deploy sqlc check-compose check-files test-deploy test-backup b2-verify fuzz tlslab-up tlslab-check tlslab-testssl tlslab-down tlslab-test tlslab-scan

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-6s %s\n", $$1, $$2}'

dev: .env web/node_modules ## Start PostgreSQL, Mailpit (:8025), the API (:8080), the worker, the mailer and Vite (:5173)
	docker compose up -d --wait db mailpit
	@set -a; . ./.env; set +a; \
	go run ./cmd/migrate && \
	trap 'kill 0' EXIT; \
	as() { echo "$$DATABASE_URL" | sed -E "s#//[^@]*@#//$$1:$$2@#"; }; \
	DATABASE_URL=$$(as qr_app "$$QR_APP_DB_PASSWORD") go run -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" ./cmd/api & \
	DATABASE_URL=$$(as qr_worker "$$QR_WORKER_DB_PASSWORD") WORKER_HEARTBEAT_FILE=$${TMPDIR:-/tmp}/qr-worker-heartbeat go run ./cmd/worker & \
	MAILER_HEARTBEAT_FILE=$${TMPDIR:-/tmp}/qr-mailer-heartbeat go run ./cmd/mailer & \
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
FUZZ_TARGETS := FuzzReadResponse FuzzParseServerHello FuzzParseServerKeyExchange FuzzReadSSLv2Response FuzzParseClientHello FuzzParseServerFlight FuzzParseTLS13Certificate FuzzReadCertificateFlight FuzzDecryptedFlight
# package:target pairs outside rawtls.
FUZZ_OTHER := internal/altcha:FuzzVerify
fuzz: ## Fuzz the raw TLS parsers and the ALTCHA verifier (FUZZTIME per target, default 1m)
	@for t in $(FUZZ_TARGETS) $(FUZZ_OTHER); do \
		pkg=./internal/scanner/rawtls; case $$t in *:*) pkg=./$${t%%:*}; t=$${t#*:};; esac; \
		echo "== $$t"; go test -run='^$$' -fuzz="^$$t$$" -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
	done

lint: web/node_modules ## golangci-lint (incl. go vet, gofmt, goimports), oxlint and tsc
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found: https://golangci-lint.run/docs/welcome/install/"; exit 1; }
	golangci-lint run ./...
	npm --prefix web run lint
	npm --prefix web run typecheck

psl:## Update the embedded Public Suffix List (newest commit at least 14 days old; see internal/domains)
	internal/domains/update-psl.sh

signup-code: .env ## Create a closed-beta signup code in the dev database: make signup-code NOTE="for Ana"
	@set -a; . ./.env; set +a; go run ./cmd/qradmin signup-code -note "$(NOTE)"

build: web/node_modules ## Build bin/{api,migrate,worker,mailer,qradmin,qrscan} and web/dist
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...
	npm --prefix web run build

sqlc: ## Regenerate internal/store from migrations/ and internal/store/queries/
	docker run --rm --user $$(id -u):$$(id -g) -v "$$PWD:/src" -w /src $(SQLC_IMAGE) generate

check-compose: ## Check the security policy of deploy/compose.prod.yaml (networks, limits, ports, digests) and that every image is pinned
	QR_API_IMAGE=ghcr.io/fizsar/quantumready-saas-api@sha256:0000000000000000000000000000000000000000000000000000000000000000 QR_WEB_IMAGE=ghcr.io/fizsar/quantumready-saas-web@sha256:0000000000000000000000000000000000000000000000000000000000000000 \
	QR_TAG=check POSTGRES_USER=u POSTGRES_PASSWORD=p POSTGRES_DB=d QR_APP_DB_PASSWORD=a QR_WORKER_DB_PASSWORD=w \
	docker compose -f deploy/compose.prod.yaml --profile mail config --format json | python3 deploy/check_compose.py
	python3 deploy/check_images.py

check-files: ## No executable binaries and no files over 1 MB in the repository (exceptions in deploy/check_files.py)
	python3 deploy/check_files.py

test-deploy: ## Test the deploy helpers without the VPS (deploy.sh refs, check-installed.sh, predeploy-dump.sh, prune-predeploy-dumps.sh, image pins, compose plan, scan switch, repository files, GHCR cleanup plan)
	bash deploy/test-deploy-ref.sh
	bash deploy/test-pin-images.sh
	bash deploy/test-check-installed.sh
	bash deploy/test-predeploy-dump.sh
	bash deploy/test-prune-predeploy-dumps.sh
	bash deploy/test-check-images.sh
	bash deploy/test-compose-plan.sh
	bash deploy/test-scan-switch.sh
	bash deploy/test-check-files.sh
	python3 deploy/test_ghcr_cleanup.py

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
