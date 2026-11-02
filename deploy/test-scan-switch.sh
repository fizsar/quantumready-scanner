#!/usr/bin/env bash
# Tests deploy/scan-switch.sh on a throwaway compose project (an idle "db" and
# an "api" that only carries the environment), run locally instead of over
# SSH. Needs docker. No VPS.
# shellcheck disable=SC2015,SC2317 # ok/FAIL reporting never fails; cleanup runs from the trap
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fx=$(mktemp -d "${XDG_CACHE_HOME:-$HOME/.cache}/qr-switch-test.XXXXXX")
cleanup() {
	(cd "$fx" && docker compose down -t 0 --remove-orphans >/dev/null 2>&1) || true
	rm -rf "$fx"
}
trap cleanup EXIT
cd "$fx"

# The same image as production's database, idle. The api is healthy unless
# BREAK_OPEN=1 and it is open (a release that would not start when opened).
# A worker is stopped before opening: only the api may be started.
PG=$(grep -o 'postgres:17-alpine@sha256:[0-9a-f]*' "$here/compose.prod.yaml")
cat >compose.yaml <<EOF
name: qr-switch-test
services:
  db:
    image: $PG
    entrypoint: ["sleep", "600"]
    environment:
      DB_SETTING: \${DB_SETTING:-a}
  worker:
    image: $PG
    entrypoint: ["sleep", "600"]
  api:
    image: $PG
    entrypoint: ["sleep", "600"]
    environment:
      PUBLIC_SCAN_OPEN: \${PUBLIC_SCAN_OPEN:-false}
      ALTCHA_HMAC_KEY: \${ALTCHA_HMAC_KEY:-}
      BREAK_OPEN: \${BREAK_OPEN:-0}
    healthcheck:
      test: ["CMD-SHELL", "[ \"\$\$BREAK_OPEN\$\$PUBLIC_SCAN_OPEN\" != 1true ]"]
      interval: 1s
      timeout: 2s
      retries: 1
EOF
key=0123456789abcdef0123456789abcdef
printf 'DB_SETTING=a\nPUBLIC_SCAN_OPEN=false\nALTCHA_HMAC_KEY=%s\nBREAK_OPEN=0\n' "$key" >.env
chmod 600 .env
docker compose up -d --wait >/dev/null 2>&1

fails=0
ok() { echo "ok   $*"; }
bad() {
	echo "FAIL $*"
	fails=1
}
switch() { QR_REMOTE_SHELL="bash -s --" QR_DIR=$fx QR_SWITCH_API='' QR_DEPLOY_URL='' bash "$here/scan-switch.sh" "$@" >"$fx/out" 2>&1; }
id() { docker compose ps -aq "$1"; }
env_of() { docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$(id api)" | sed -n "s/^$1=//p"; }
line() { sed -n "s/^$1=//p" .env; }
db0=$(id db)

api0=$(id api)
docker compose stop -t 0 worker >/dev/null 2>&1
switch open && ok "open" || bad "open: $(cat "$fx/out")"
[ "$(docker inspect -f '{{.State.Running}}' "$(id worker)")" = false ] && ok "open: the stopped worker stays stopped (api only)" || bad "open: the worker was started"
[ "$(line PUBLIC_SCAN_OPEN)/$(env_of PUBLIC_SCAN_OPEN)" = true/true ] && ok "open: .env and the running api say true" || bad "open: .env $(line PUBLIC_SCAN_OPEN), api $(env_of PUBLIC_SCAN_OPEN)"
[ "$(id api)" != "$api0" ] && ok "open: api recreated" || bad "open: api not recreated"
[ "$(id db)" = "$db0" ] && ok "open: db untouched" || bad "open: db recreated"
[ "$(stat -c %a .env)" = 600 ] && [ ! -e .env.switch ] && ok "open: .env still 600, no copy left" || bad "open: mode $(stat -c %a .env), copy left: $([ -e .env.switch ] && echo yes || echo no)"
grep -q 'api recreate' "$fx/out" && ok "open: the plan was shown" || bad "open: no plan in the output"

api1=$(id api)
switch open && grep -q 'already true' "$fx/out" && [ "$(id api)" = "$api1" ] && ok "open again: nothing recreated" || bad "open again: $(cat "$fx/out")"

QR_SWITCH_DRY_RUN=1 switch close && grep -q 'dry run' "$fx/out" && [ "$(line PUBLIC_SCAN_OPEN)" = true ] && [ "$(id api)" = "$api1" ] &&
	ok "dry run: plan only, .env and api as they were" || bad "dry run: $(cat "$fx/out")"

switch close && [ "$(line PUBLIC_SCAN_OPEN)/$(env_of PUBLIC_SCAN_OPEN)" = false/false ] && [ "$(id db)" = "$db0" ] &&
	ok "close: false in .env and api, db untouched" || bad "close: $(cat "$fx/out")"

# Opening needs the proof-of-work key.
sed -i '/^ALTCHA_HMAC_KEY=/d' .env
before=$(sha256sum .env)
api2=$(id api)
! switch open && grep -q 'ALTCHA_HMAC_KEY' "$fx/out" && [ "$(sha256sum .env)" = "$before" ] && [ "$(id api)" = "$api2" ] &&
	ok "open without ALTCHA_HMAC_KEY: refused, nothing changed" || bad "open without key: $(cat "$fx/out")"
echo "ALTCHA_HMAC_KEY=$key" >>.env

# Something else differs from what runs (a .env edited by hand): refuse.
sed -i 's/^DB_SETTING=a/DB_SETTING=b/' .env
before=$(sha256sum .env)
! switch open && grep -q "exactly 'api recreate'" "$fx/out" && [ "$(sha256sum .env)" = "$before" ] && [ "$(id db)" = "$db0" ] &&
	ok "plan with more than the api: refused, .env put back" || bad "drift: $(cat "$fx/out")"
sed -i 's/^DB_SETTING=b/DB_SETTING=a/' .env

# The api does not come up healthy once open: back to closed.
sed -i 's/^BREAK_OPEN=0/BREAK_OPEN=1/' .env
docker compose up -d --no-deps --wait api >/dev/null 2>&1 # BREAK_OPEN applied while closed: healthy
! switch open && grep -q 'put back' "$fx/out" && [ "$(line PUBLIC_SCAN_OPEN)/$(env_of PUBLIC_SCAN_OPEN)" = false/false ] &&
	[ "$(docker inspect -f '{{.State.Health.Status}}' "$(id api)")" = healthy ] && [ "$(id db)" = "$db0" ] &&
	ok "api unhealthy when opened: .env put back and the api closed and healthy again" || bad "unhealthy: $(cat "$fx/out"); api $(env_of PUBLIC_SCAN_OPEN)"

usage=0
bash "$here/scan-switch.sh" maybe >/dev/null 2>&1 || usage=$?
[ "$usage" = 2 ] && ok "usage error on anything but open|close" || bad "usage exit $usage"

[ "$fails" = 0 ] && echo "scan-switch tests: all passed" || {
	echo "scan-switch tests: FAILED"
	exit 1
}
