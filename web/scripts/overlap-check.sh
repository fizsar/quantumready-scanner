#!/usr/bin/env bash
# /lab/historia scrolled from top to bottom in real engines, Chromium and
# WebKit (Playwright, pinned image), with phone and desktop profiles: no text
# over the 3D stage without its own panel, no box over the shape's free area
# on wide screens, never an empty screen, panels that fit (overlap-check.mjs). The site is built and served with the production
# nginx config in a throwaway internal Docker network (no Internet).
# Without a GPU in the container the stage shows the still images: the same
# element and the same visibility rules as the 3D canvas.
#
#   web/scripts/overlap-check.sh [PROFILES] [PATH...]
#   PROFILES: engine:WxHxDPR,... (default: chromium at 375×667, 375×812,
#             393×852 DPR 3, and on desktop 1280×800, 1440×900 and
#             1920×1080 at DPR 1 and 2 and 1634×916; webkit at 375×667,
#             393×852 and 393×659 (an iPhone with Safari’s toolbars), DPR 3,
#             and 1440×900 DPR 2)
#   BASE=https://… web/scripts/overlap-check.sh   checks a site already up
#             (for instance production, as any visitor would) instead of a build.
set -euo pipefail
cd "$(dirname "$0")/.."
PROFILES=${1:-chromium:375x667x2,chromium:375x812x1,chromium:393x852x3,chromium:1280x800x1,chromium:1280x800x2,chromium:1440x900x1,chromium:1440x900x2,chromium:1634x916x1,chromium:1920x1080x1,chromium:1920x1080x2,webkit:375x667x3,webkit:393x852x3,webkit:393x659x3,webkit:1440x900x2}
shift || true
[ $# -gt 0 ] || set -- '/lab/historia?lang=es' '/lab/historia?lang=en'
# The same version as the playwright package in package.json.
PW=mcr.microsoft.com/playwright:v1.64.1-noble@sha256:47b510dd9230bf0b9bfc9b20102241e577ea7a99334ab4c3ca19f19770e9b8ea
[ -d node_modules/playwright ] || npm ci --no-fund --no-audit

if [ -n "${BASE:-}" ]; then
	docker run --rm --ipc=host -v "$PWD:/web:ro" -w /web "$PW" node scripts/overlap-check.mjs "$BASE" "$PROFILES" "$@"
	exit
fi

NGINX=$(grep -m1 -E '^FROM .*nginx' Dockerfile | awk '{print $2}')
NET=qr-overlap-$$
npm run -s build >/dev/null
serve=$(mktemp -d)
cp -r dist/. "$serve/"
mkdir -p "$serve/api"
echo '{"hosts":["fizsar.com","quantumready.fizsar.com"],"port":443,"open":true,"proof":true}' >"$serve/api/targets"
chmod -R a+rX "$serve"
cleanup() {
	docker rm -f "$NET-web" >/dev/null 2>&1 || true
	docker network rm "$NET" >/dev/null 2>&1 || true
	rm -rf "$serve"
}
trap cleanup EXIT
docker network create --internal "$NET" >/dev/null
docker run -d --name "$NET-web" --network "$NET" --network-alias web -v "$serve:/usr/share/nginx/html:ro" \
	-v "$PWD/nginx/default.conf:/etc/nginx/conf.d/default.conf:ro" \
	-v "$PWD/nginx/security-headers.conf:/etc/nginx/snippets/security-headers.conf:ro" "$NGINX" >/dev/null
docker run --rm --ipc=host --network "$NET" -v "$PWD:/web:ro" -w /web "$PW" node scripts/overlap-check.mjs http://web:8080 "$PROFILES" "$@"
