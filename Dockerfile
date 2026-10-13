# syntax=docker/dockerfile:1

# API image: static Go binaries on distroless (nonroot, no shell).
# Ships /api, /migrate (one-shot compose service) and /worker (scan queue).
# Only /worker carries the scanner's GODEBUG settings (see cmd/worker).

FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG VERSION=dev
ARG COMMIT=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/ ./cmd/api ./cmd/migrate ./cmd/worker

FROM gcr.io/distroless/static-debian13:nonroot
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.source="https://github.com/fizsar/quantumready-saas" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
COPY --from=build /out/api /out/migrate /out/worker /
USER nonroot:nonroot
ENV API_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 CMD ["/api", "healthcheck"]
ENTRYPOINT ["/api"]
