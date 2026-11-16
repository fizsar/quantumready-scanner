#!/usr/bin/env bash
# No horizontal overflow on narrow screens: builds the site, serves it with
# the production nginx config and checks each page at WIDTH px (default 375)
# in a real headless Chromium (jsdom has no layout), with every "Under the
# hood" panel and every <details> open. Reports are served from the API's
# golden answers (internal/httpapi/testdata/apijson): a 2026.3 report with the
# probe log of a real scan (web/src/test/v2), a lab server with many findings
# and a scan judged with 2026.2. Everything runs in a throwaway internal
# Docker network: no published ports, no Internet.
#
#   web/scripts/overflow-check.sh [WIDTH] [PAGE...]   (default: / /architecture /examples /privacy /about-scans /lab/historia and the reports, ES and EN)
#   OVERFLOW_OPEN=1 web/scripts/overflow-check.sh ...  (the home page with the scanner open)
set -euo pipefail
cd "$(dirname "$0")/.."
WIDTH=${1:-375}
shift || true
PAGES=("$@")
# Fixed ids for the reports served below (one per answer: the server ignores ?lang=).
FIZSAR_ES=00000000-0000-4000-8000-000000000001 FIZSAR_EN=00000000-0000-4000-8000-000000000002
EXPORT_ES=00000000-0000-4000-8000-000000000003 EXPORT_EN=00000000-0000-4000-8000-000000000004
OLD_ES=00000000-0000-4000-8000-000000000005
[ ${#PAGES[@]} -gt 0 ] || PAGES=(/?lang=es /architecture?lang=es /examples?lang=es /privacy?lang=es /about-scans?lang=es /lab/historia?lang=es /?lang=en /architecture?lang=en /examples?lang=en /privacy?lang=en /about-scans?lang=en /lab/historia?lang=en
	"/scans/$FIZSAR_ES?lang=es" "/scans/$FIZSAR_EN?lang=en" "/scans/$EXPORT_ES?lang=es" "/scans/$EXPORT_EN?lang=en" "/scans/$OLD_ES?lang=es")
CHROME=${CHROME_IMAGE:-zenika/alpine-chrome:124@sha256:eb3378c1ed0079f94db054a5fe1aaa790a254ec0d6bbc67eda052420d86a179d}
NODE=$(grep -m1 -E '^FROM node:' Dockerfile | awk '{print $2}')
NGINX=$(grep -m1 -E '^FROM .*nginx' Dockerfile | awk '{print $2}')
NET=qr-overflow-$$

npm run -s build >/dev/null
serve=$(mktemp -d)
cp -r dist/. "$serve/"
mkdir -p "$serve/api"
# OVERFLOW_OPEN=1: the home page as with PUBLIC_SCAN_OPEN=true (its own texts).
if [ "${OVERFLOW_OPEN:-}" = 1 ]; then
	echo '{"hosts":["fizsar.com","quantumready.fizsar.com"],"port":443,"open":true,"proof":true}' >"$serve/api/targets"
else
	echo '{"hosts":["fizsar.com","quantumready.fizsar.com"],"port":443}' >"$serve/api/targets"
fi
mkdir -p "$serve/api/scans"
golden=../internal/httpapi/testdata/apijson
node - "$serve/api/scans" "$golden" <<JS
const fs = require('node:fs')
const [out, golden] = process.argv.slice(2)
const v2 = JSON.parse(fs.readFileSync('src/test/v2/fizsar.result.json', 'utf8'))
const put = (id, file, result) => {
  const scan = JSON.parse(fs.readFileSync(golden + '/' + file, 'utf8'))
  scan.id = id
  if (result) scan.report.result = result
  fs.writeFileSync(out + '/' + id, JSON.stringify(scan))
}
put('$FIZSAR_ES', 'fizsar.com.es.json', v2)
put('$FIZSAR_EN', 'fizsar.com.en.json', v2)
put('$EXPORT_ES', 'export.es.json')
put('$EXPORT_EN', 'export.en.json')
put('$OLD_ES', 'fizsar.com-2026.2.es.json')
JS
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
docker run -d --name "$NET-chrome" --network "$NET" --shm-size=1g "$CHROME" --no-sandbox --headless --disable-gpu \
	--remote-debugging-address=0.0.0.0 --remote-debugging-port=9222 about:blank >/dev/null
chrome_ip=$(docker inspect -f "{{(index .NetworkSettings.Networks \"$NET\").IPAddress}}" "$NET-chrome")
for _ in $(seq 1 50); do
	docker run --rm --network "$NET" "$NODE" node -e "fetch('http://$chrome_ip:9222/json/version').then(r=>process.exit(r.ok?0:1),()=>process.exit(1))" 2>/dev/null && break
	sleep 0.3
done
urls=()
for p in "${PAGES[@]}"; do urls+=("http://web:8080$p"); done
docker run --rm --network "$NET" -v "$PWD/scripts/overflow-check.mjs:/check.mjs:ro" "$NODE" node /check.mjs "$chrome_ip" "$WIDTH" "${urls[@]}"
