#!/usr/bin/env bash
# Tests the digest rules of deploy/check_compose.py and deploy/check_images.py
# on a copy of the repository, mutated one way at a time. Needs docker compose
# (to render the production compose) and python3.
# shellcheck disable=SC2015,SC2317 # ok/FAIL reporting never fails; checkers run through $run
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git ls-files -z | grep -zv '^web/src/test/lab/' | xargs -0 tar -cf - | tar -xf - -C "$tmp/" 2>/dev/null || true
cp -r "$tmp" "$tmp.orig" && trap 'rm -rf "$tmp" "$tmp.orig"' EXIT

fails=0
compose() { # check_compose.py on the copy's production compose
	(cd "$tmp" && QR_API_IMAGE=${API_IMAGE:-ghcr.io/fizsar/quantumready-saas-api@sha256:$(printf '0%.0s' {1..64})} \
		QR_WEB_IMAGE=ghcr.io/fizsar/quantumready-saas-web@sha256:$(printf '0%.0s' {1..64}) QR_TAG=check POSTGRES_USER=u POSTGRES_PASSWORD=p POSTGRES_DB=d QR_APP_DB_PASSWORD=a QR_WORKER_DB_PASSWORD=w \
		docker compose -f deploy/compose.prod.yaml --profile mail config --format json | python3 deploy/check_compose.py)
}
images() { python3 deploy/check_images.py "$tmp"; }
expect() { # expect pass|fail DESCRIPTION CHECKER [GREP]
	local want=$1 d=$2 run=$3 pattern=${4:-} got
	if $run >"$tmp.out" 2>&1; then got=pass; else got=fail; fi
	if [[ $got == "$want" ]] && { [[ -z $pattern ]] || grep -qE -- "$pattern" "$tmp.out"; }; then echo "ok   $d"; else
		echo "FAIL $d (want $want, got $got${pattern:+, output must match $pattern})"
		cat "$tmp.out"
		fails=1
	fi
}
reset() { rm -rf "$tmp" && cp -r "$tmp.orig" "$tmp"; }
edit() { python3 - "$tmp/$1" "$2" "$3" <<'EOF'
import sys
p, old, new = sys.argv[1:]
s = open(p).read()
assert s.count(old) >= 1, (p, old)
open(p, "w").write(s.replace(old, new, 1))
EOF
}
PG_DIGEST=$(grep -o 'postgres:17-alpine@sha256:[0-9a-f]*' deploy/compose.prod.yaml)

expect pass "production compose: every image pinned (ours by the recorded digest)" compose
API_IMAGE=ghcr.io/fizsar/quantumready-saas-api:sha-1234567 expect fail "production compose: our image by tag" compose "api: image .* must be pinned by digest"
API_IMAGE=docker.io/evil/api@sha256:$(printf '0%.0s' {1..64}) expect fail "production compose: another image by digest" compose "api: image .* must be ghcr.io/fizsar/quantumready-saas-api@"
expect pass "repository: every image pinned, one PostgreSQL digest" images

edit deploy/compose.prod.yaml "$PG_DIGEST" "postgres:17-alpine"
expect fail "production compose: postgres without digest" compose "db: image 'postgres:17-alpine' must be pinned by digest"
expect fail "repository: postgres without digest in production" images "compose.prod.yaml:[0-9]+: postgres:17-alpine is not pinned"
reset

edit deploy/compose.prod.yaml "$PG_DIGEST" "postgres:17-alpine@sha256:$(printf '0%.0s' {1..64})"
expect pass "production compose: another digest is still a digest" compose
expect fail "repository: production bumped alone (Dependabot) -> the others must follow" images "PostgreSQL is pinned to different images"
reset

edit deploy/compose.prod.yaml "  web:
    image: \"\${QR_WEB_IMAGE" "  extra:
    image: redis:7
  web:
    image: \"\${QR_WEB_IMAGE"
expect fail "production compose: a new service without digest" compose "extra: image 'redis:7'"
reset

edit Dockerfile "FROM gcr.io/distroless/static-debian13:nonroot@sha256:" "FROM gcr.io/distroless/static-debian13:nonroot #"
expect fail "repository: a Dockerfile FROM without digest" images "Dockerfile:[0-9]+: gcr.io/distroless/static-debian13:nonroot is not pinned"
reset

edit web/Dockerfile "FROM node:24.21-alpine@sha256:" "FROM node:24.21-alpine #"
expect fail "repository: the web Dockerfile" images "web/Dockerfile:[0-9]+: node:24.21-alpine"
reset

edit .github/workflows/ci.yml "$PG_DIGEST" "postgres:17-alpine"
expect fail "repository: the CI service container" images "ci.yml:[0-9]+: postgres:17-alpine is not pinned"
reset

edit deploy/backup/common.sh "$PG_DIGEST" "postgres:17-alpine"
expect fail "repository: the restore drill's image" images "common.sh:[0-9]+: postgres:17-alpine is not pinned"
reset

edit web/scripts/overflow-check.sh "zenika/alpine-chrome:124@sha256:" "zenika/alpine-chrome:124 #"
expect fail "repository: a tool image in a script" images "overflow-check.sh:[0-9]+: zenika/alpine-chrome:124"
reset

edit deploy/backup/test.sh "koalaman/shellcheck:v0.10.0@sha256:" "koalaman/shellcheck:v0.10.0 #"
expect fail "repository: a docker run tool" images "test.sh:[0-9]+: koalaman/shellcheck:v0.10.0 is not pinned"
reset

edit Dockerfile "AS build" "AS builder"
expect pass "repository: build stages and FROM scratch are not images" images
exit $fails
