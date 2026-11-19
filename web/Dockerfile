# syntax=docker/dockerfile:1

# Web image: Vite build served by nginx-unprivileged (uid 101, port 8080).

FROM node:24.21-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-fund --no-audit
COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.30-alpine-slim@sha256:5d8bb9437d74a600ec5dbf90c035451c724dc33eac8d3a71078f6f8612c345f6
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.source="https://github.com/fizsar/quantumready-saas" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
# The pinned base still ships musl 1.2.6-r2, zlib 1.3.2-r0 and nginx 1.30.6-r1, with
# fixable HIGH/CRITICAL vulnerabilities (musl CVE-2026-107891, zlib CVE-2026-85091;
# nginx CVE-2026-113425, -121810, -123007, -123633, -118899, -123634, fixed in
# 1.30.7-r0; Trivy in release.yml), and no newer digest is published yet: install
# at least the fixed versions (a minimum, not an exact version: Alpine keeps only
# the latest revision, so an exact pin would break the build at the next one). The
# image's own nginx.conf stays (apk leaves the package's as nginx.conf.apk-new).
# deploy/check_base_pins.py (CI, Docker web job) fails once the base image's digest
# carries them all: then drop these three lines.
USER root
RUN apk add --no-cache 'musl>=1.2.6-r3' 'musl-utils>=1.2.6-r3' 'zlib>=1.3.2-r1' 'nginx>=1.30.7-r0'   # base-pins
USER 101
COPY nginx/default.conf /etc/nginx/conf.d/default.conf
COPY nginx/security-headers.conf /etc/nginx/snippets/security-headers.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD ["wget", "-q", "--spider", "http://127.0.0.1:8080/healthz"]
