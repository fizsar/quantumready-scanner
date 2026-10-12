# QuantumReady SaaS

[![CI](https://github.com/fizsar/quantumready-saas/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/fizsar/quantumready-saas/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)

**ES** · [EN](#english)

Auditoría de preparación post-cuántica (PQC) para endpoints TLS de pymes y empresas europeas: soporte de ML-KEM híbrido, versiones TLS, cifrados y algoritmos de certificado, con informes que mapean NIS2, DORA y la hoja de ruta PQC de la UE.

Estado: en desarrollo. https://quantumready.fizsar.com

## Desarrollo

Requisitos: Go 1.26+, Node 24+, Docker.

```sh
make dev     # PostgreSQL (Docker) + API en :8080 + web en http://localhost:5173
make test    # tests de Go
make lint    # golangci-lint, oxlint, tsc
make build   # bin/api, bin/migrate, bin/worker y web/dist
make prod-up # stack de producción (deploy/compose.prod.yaml) en 127.0.0.1:8080 y :8081
```

`make dev` crea `.env` a partir de `.env.example` si no existe. Nunca subas `.env` al repositorio.

---

## English

Post-quantum (PQC) readiness audits for the TLS endpoints of European SMEs and companies: hybrid ML-KEM support, TLS versions, ciphers and certificate algorithms, with reports mapped to NIS2, DORA and the EU PQC roadmap.

Status: in development. https://quantumready.fizsar.com

### Development

Requirements: Go 1.26+, Node 24+, Docker.

```sh
make dev     # PostgreSQL (Docker) + API on :8080 + web on http://localhost:5173
make test    # Go tests
make lint    # golangci-lint, oxlint, tsc
make build   # bin/api, bin/migrate, bin/worker and web/dist
make prod-up # production stack (deploy/compose.prod.yaml) on 127.0.0.1:8080 and :8081
```

`make dev` creates `.env` from `.env.example` if missing. Never commit `.env`.

---

## Licencia / License

[AGPL-3.0-only](LICENSE). Copyright (C) 2026 Fiz Sar López.
