# syntax=docker/dockerfile:1

# API image: static Go binaries on distroless (nonroot, no shell).
# Ships /api, /migrate (one-shot compose service), /worker (scan queue),
# /mailer (email outbox) and /qradmin (signup codes, exclusions, opt-outs and
# usage stats; run by hand).
# Only /worker carries the scanner's GODEBUG settings (see cmd/worker).

FROM golang:1.27.3-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build
WORKDIR /src
COPY go.mod go.sum ./
# Modules in a layer, not in a cache mount: the CI's layer cache (buildx,
# type=gha) keeps layers but not cache mounts.
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG VERSION=dev
ARG COMMIT=unknown
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/ ./cmd/api ./cmd/migrate ./cmd/worker ./cmd/mailer ./cmd/qradmin

FROM gcr.io/distroless/static-debian13:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.source="https://github.com/fizsar/quantumready-saas" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
COPY --from=build /out/api /out/migrate /out/worker /out/mailer /out/qradmin /
USER nonroot:nonroot
ENV API_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 CMD ["/api", "healthcheck"]
ENTRYPOINT ["/api"]
