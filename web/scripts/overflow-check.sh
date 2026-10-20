#!/usr/bin/env bash
# No horizontal overflow on narrow screens: builds the site, serves it with
# the production nginx config and checks each page at WIDTH px (default 375)
# in a real headless Chromium (jsdom has no layout). Everything runs in a
# throwaway internal Docker network: no published ports, no Internet.
#
#   web/scripts/overflow-check.sh [WIDTH] [PAGE...]   (default: / /architecture /examples /privacy /demo)
set -euo pipefail
cd "$(dirname "$0")/.."
WIDTH=${1:-375}
shift || true
PAGES=("$@")
[ ${#PAGES[@]} -gt 0 ] || PAGES=(/ /architecture /examples /privacy /demo)
CHROME=${CHROME_IMAGE:-zenika/alpine-chrome:124}
NODE=$(grep -m1 -E '^FROM node:' Dockerfile | awk '{print $2}')
NGINX=$(grep -m1 -E '^FROM .*nginx' Dockerfile | awk '{print $2}')
NET=qr-overflow-$$

npm run -s build >/dev/null
serve=$(mktemp -d)
cp -r dist/. "$serve/"
mkdir -p "$serve/api"
echo '{"hosts":["fizsar.com","quantumready.fizsar.com"],"port":443}' >"$serve/api/targets"
chmod -R a+rX "$serve"
cleanup() {
	docker rm -f "$NET-web" "$NET-chrome" >/dev/null 2>&1 || true
	docker network rm "$NET" >/dev/null 2>&1 || true
	rm -rf "$serve"
}
trap cleanup EXIT
docker network create --internal "$NET" >/dev/null
docker run -d --name "$NET-web" --network "$NET" --network-alias web -v "$serve:/usr/share/nginx/html:ro" \
	-v "$PWD/nginx/default.conf:/etc/nginx/conf.d/default.conf:ro" \
	-v "$PWD/nginx/security-headers.conf:/etc/nginx/snippets/security-headers.conf:ro" "$NGINX" >/dev/null
docker run -d --name "$NET-chrome" --network "$NET" "$CHROME" --no-sandbox --headless --disable-gpu \
	--remote-debugging-address=0.0.0.0 --remote-debugging-port=9222 about:blank >/dev/null
chrome_ip=$(docker inspect -f "{{(index .NetworkSettings.Networks \"$NET\").IPAddress}}" "$NET-chrome")
for _ in $(seq 1 50); do
	docker run --rm --network "$NET" "$NODE" node -e "fetch('http://$chrome_ip:9222/json/version').then(r=>process.exit(r.ok?0:1),()=>process.exit(1))" 2>/dev/null && break
	sleep 0.3
done
urls=()
for p in "${PAGES[@]}"; do urls+=("http://web:8080$p"); done
docker run --rm --network "$NET" -v "$PWD/scripts/overflow-check.mjs:/check.mjs:ro" "$NODE" node /check.mjs "$chrome_ip" "$WIDTH" "${urls[@]}"
