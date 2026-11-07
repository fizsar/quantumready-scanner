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
# The pinned base still ships musl 1.2.6-r2 and zlib 1.3.2-r0, with fixable HIGH
# vulnerabilities (CVE-2026-107891, CVE-2026-85091; Trivy in release.yml): install
# the fixed versions, pinned (zlib 1.4-r0: the one Alpine 3.24 now carries, same
# libz.so.1). Drop this when the base image's digest carries them.
USER root
RUN apk add --no-cache 'musl=1.2.6-r3' 'musl-utils=1.2.6-r3' 'zlib=1.4-r0'
USER 101
COPY nginx/default.conf /etc/nginx/conf.d/default.conf
COPY nginx/security-headers.conf /etc/nginx/snippets/security-headers.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD ["wget", "-q", "--spider", "http://127.0.0.1:8080/healthz"]
