# syntax=docker/dockerfile:1

# Web image: Vite build served by nginx-unprivileged (uid 101, port 8080).

FROM node:24.21-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-fund --no-audit
COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.30-alpine-slim
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.source="https://github.com/fizsar/quantumready-saas" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
COPY nginx/default.conf /etc/nginx/conf.d/default.conf
COPY nginx/security-headers.conf /etc/nginx/snippets/security-headers.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD ["wget", "-q", "--spider", "http://127.0.0.1:8080/healthz"]
