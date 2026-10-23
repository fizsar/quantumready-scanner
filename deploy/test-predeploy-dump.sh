#!/usr/bin/env bash
# Tests deploy/predeploy-dump.sh without the VPS: the "remote" is bash on this
# machine (the script still arrives through stdin, as over ssh) against a
# throwaway PostgreSQL (compose project qr-predeploy-test). Needs docker.
# shellcheck disable=SC2015,SC2317 # ok and bad never fail; cleanup runs from the trap
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fx=$(mktemp -d "${XDG_CACHE_HOME:-$HOME/.cache}/qr-predeploy-test.XXXXXX")
cleanup() {
	(cd "$fx" && docker compose down -v --remove-orphans >/dev/null 2>&1) || true
	rm -rf "$fx"
}
trap cleanup EXIT
mkdir "$fx/backups"
cat >"$fx/compose.yaml" <<'EOF'
name: qr-predeploy-test
services:
  db:
    image: postgres:17-alpine@sha256:eee6c6e1b0894fa4fe0075eb054af9fd1cf39a35321c2d4110626cfe506411fb
    environment: { POSTGRES_USER: qr, POSTGRES_PASSWORD: test-only, POSTGRES_DB: quantumready }
    tmpfs: [/var/lib/postgresql/data]
    healthcheck:
      test: [CMD, pg_isready, -h, 127.0.0.1, -U, qr, -d, quantumready]
      interval: 1s
      retries: 60
EOF
(cd "$fx" && docker compose up -d --wait db >/dev/null 2>&1)
(cd "$fx" && docker compose exec -T db psql -U qr -d quantumready -qc '
	CREATE TABLE scans (id int); INSERT INTO scans VALUES (1);
	CREATE TABLE empty (id int);
	CREATE TABLE usage_daily (n int); INSERT INTO usage_daily VALUES (1);
	CREATE TABLE blocked_targets_daily (d text); INSERT INTO blocked_targets_daily VALUES ($$example.org$$);' </dev/null)

export QR_REMOTE_SHELL="bash -s --" QR_DIR=$fx
fails=0
ok() { echo "ok   $*"; }
bad() {
	echo "FAIL $*"
	fails=1
}

if bash "$here/predeploy-dump.sh" pre-test >"$fx/out" 2>&1; then ok "dump runs"; else bad "dump runs"; cat "$fx/out"; fi
f=$(find "$fx/backups" -name 'pre-test-*.dump')
[[ $(basename "$f") =~ ^pre-test-[0-9]{8}-[0-9]{4}\.dump$ ]] && ok "named pre-test-YYYYMMDD-HHMM.dump" || bad "file name: $f"
[[ $(stat -c %a "$f") == 600 ]] && ok "mode 600" || bad "mode $(stat -c %a "$f")"
[[ $(head -c 5 "$f") == PGDMP ]] && ok "custom-format archive" || bad "not PGDMP"
# The last line comes after pg_dump: the script was not eaten by a command reading stdin.
[[ $(tail -1 "$fx/out") == "==> dump ok" ]] && ok "the whole script ran (stdin not swallowed)" || bad "script cut short: $(tail -3 "$fx/out")"
grep -qx 'tables with data: empty scans ' "$fx/out" && ok "summary: tables with data" || bad "summary: $(grep 'tables with' "$fx/out")"
grep -qx 'tables without data: blocked_targets_daily usage_daily ' "$fx/out" && ok "no-data tables: schema only" || bad "summary: $(grep 'without data' "$fx/out")"
[[ -z $(find "$fx/backups" -name '*.partial' -o -name '*.toc') ]] && ok "no temporary files left" || bad "temporary files left"

sum=$(sha256sum "$f")
if bash "$here/predeploy-dump.sh" pre-test >/dev/null 2>&1; then
	# Only possible if the minute changed: then it is a second file and the first is untouched.
	[[ $(find "$fx/backups" -name 'pre-test-*.dump' | wc -l) == 2 && $(sha256sum "$f") == "$sum" ]] &&
		ok "an existing dump is never overwritten (new minute, new file)" || bad "a second dump overwrote the first"
else ok "an existing dump is never overwritten"; fi
for n in "" "Pre-M1" "../x" "a b"; do
	if bash "$here/predeploy-dump.sh" "$n" >/dev/null 2>&1; then bad "name '$n' accepted"; else ok "name '$n' refused"; fi
done

(cd "$fx" && docker compose stop db >/dev/null 2>&1)
if bash "$here/predeploy-dump.sh" pre-down >/dev/null 2>&1; then bad "dump with the database down succeeded"; else ok "fails with the database down"; fi
[[ -z $(find "$fx/backups" -name 'pre-down*') ]] && ok "and leaves nothing behind" || bad "files left after a failure"
exit $fails
