#!/usr/bin/env bash
# Tests for backup.sh and restore-test.sh against a throwaway PostgreSQL
# (compose project qr-backup-test) and a local restic repository. Healthchecks
# pings go to a local HTTP server that records them. Needs docker and python3.
#   make test-backup
# shellcheck disable=SC2015,SC2016 # ok and bad never fail; $ in single quotes is SQL or bash -c
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=deploy/backup/common.sh
. "$here/common.sh"

fails=0
ok() { echo "ok   $*"; }
bad() {
	echo "FAIL $*"
	fails=$((fails + 1))
}
check() { # check DESCRIPTION COMMAND…
	local d=$1
	shift
	if "$@" >/dev/null; then ok "$d"; else bad "$d"; fi
}

# Lint with shellcheck if installed, else with its official image.
sc=(shellcheck)
command -v shellcheck >/dev/null || sc=(docker run --rm -v "$here/../..:/src:ro" -w /src koalaman/shellcheck:v0.10.0)
(cd "$here/../.." && "${sc[@]}" -x deploy/backup/*.sh deploy/*.sh) && ok shellcheck || bad shellcheck

# Under $HOME, not /tmp: it is bind-mounted into containers.
fx=$(mktemp -d "${XDG_CACHE_HOME:-$HOME/.cache}/qr-backup-test.XXXXXX")
cleanup() {
	(cd "$fx" && docker compose down -v --remove-orphans >/dev/null 2>&1) || true
	[ -z "${pinger:-}" ] || kill "$pinger" 2>/dev/null || true
	rm -rf "$fx"
}
trap cleanup EXIT
mkdir -p "$fx/backups" "$fx/repo"
cat >"$fx/compose.yaml" <<'EOF'
services:
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    tmpfs: [/var/lib/postgresql/data]
    healthcheck:
      test: [CMD, pg_isready, -h, 127.0.0.1, -U, qr, -d, quantumready]
      interval: 1s
      retries: 60
EOF
cat >"$fx/.env" <<'EOF'
COMPOSE_PROJECT_NAME=qr-backup-test
POSTGRES_USER=qr
POSTGRES_PASSWORD=test-only
POSTGRES_DB=quantumready
EOF
chmod 600 "$fx/.env"
touch "$fx/backups/pre-phase2-20261013T184241Z.dump" # a manual dump: must survive local rotation

# Healthchecks.io stand-in: one line per request, "path body".
python3 -I - "$fx/pings.log" "$fx/port" <<'EOF' &
import http.server, sys
log, portfile = sys.argv[1], sys.argv[2]
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length") or 0)).decode()
        with open(log, "a") as f:
            f.write(f"{self.path} {body}\n")
        self.send_response(200)
        self.end_headers()
    def log_message(self, *a):
        pass
s = http.server.HTTPServer(("127.0.0.1", 0), H)
open(portfile, "w").write(str(s.server_port))
s.serve_forever()
EOF
pinger=$!
for _ in $(seq 50); do [ -s "$fx/port" ] && break; sleep 0.1; done
touch "$fx/pings.log"
hc_base=http://127.0.0.1:$(cat "$fx/port")

export QR_DIR=$fx BACKUP_DIR=$fx/backups RESTIC_REPOSITORY=$fx/repo RESTIC_PASSWORD=test-only \
	HC_PING_URL=$hc_base/backup HC_RESTORE_PING_URL=$hc_base/restore
psql() { (cd "$fx" && docker compose exec -T db psql -U qr -d quantumready -tAq -v ON_ERROR_STOP=1 "$@" </dev/null); }
snapshots() { restic_run snapshots --json | python3 -Ic 'import json,sys; print(len(json.load(sys.stdin)))'; }
pings() { grep -c "^$1 ${2:-}" "$fx/pings.log" || true; }
qr_dumps() { find "$fx/backups" -maxdepth 1 -name "qr-*$1" | wc -l; }

(cd "$fx" && docker compose up -d --wait db >/dev/null 2>&1)
psql -c 'CREATE TABLE scans (id serial PRIMARY KEY, host text);
	CREATE TABLE findings (id serial PRIMARY KEY, scan_id int REFERENCES scans);
	CREATE TABLE "audit log" (id int); CREATE TABLE empty (id int);
	CREATE TABLE usage_daily (day date, event text, dim text, count bigint);
	CREATE TABLE blocked_targets_daily (day date, domain text, count bigint);
	INSERT INTO scans (host) VALUES ($$a.com$$), ($$b.com$$), ($$c.com$$);
	INSERT INTO findings (scan_id) SELECT 1 FROM generate_series(1, 5);
	INSERT INTO "audit log" VALUES (1);
	INSERT INTO usage_daily VALUES (current_date, $$page_view$$, $$/$$, 7), (current_date, $$lang_switch$$, $$en$$, 1);
	INSERT INTO blocked_targets_daily VALUES (current_date, $$example.org$$, 3);'

# NO_DATA_TABLES are the real M1 tables (a rename must not silently back them up).
for t in "${NO_DATA_TABLES[@]}"; do
	check "no-data table $t exists in migrations/" grep -qE "^CREATE TABLE $t \(" "$here"/../../migrations/*.sql
done

echo "--- backups"
check "init" bash "$here/backup.sh" init
check "backup 1 (with check)" env FORCE_CHECK=1 bash "$here/backup.sh"
sleep 1.1
psql -c 'INSERT INTO scans (host) VALUES ($$d.com$$)'
check "backup 2" bash "$here/backup.sh"
sleep 1.1
check "backup 3" bash "$here/backup.sh"

check "local: last 2 dumps and manifests kept" [ "$(qr_dumps .dump)/$(qr_dumps .manifest.json)" = 2/2 ]
check "local: manual dump untouched" [ -e "$fx/backups/pre-phase2-20261013T184241Z.dump" ]
check "local: no .partial files" [ "$(qr_dumps .partial)" = 0 ]
check "local: files are 0600" [ -z "$(find "$fx/backups" -name 'qr-*' ! -perm 600)" ]
latest=$(find "$fx/backups" -name 'qr-*.manifest.json' | sort | tail -1)
want='{ "audit log" : 1, "blocked_targets_daily" : 0, "empty" : 0, "findings" : 5, "scans" : 4, "usage_daily" : 0 }'
check "manifest = row counts, 0 for no-data tables ($(cat "$latest"))" [ "$(cat "$latest")" = "$want" ]
toc=$(cd "$fx" && docker compose exec -T db pg_restore --list <"${latest%.manifest.json}.dump")
for t in "${NO_DATA_TABLES[@]}"; do
	check "dump: $t schema kept, rows left out" bash -c 'grep -qE " TABLE public $1 " <<<"$2" && ! grep -qE " TABLE DATA public $1 " <<<"$2"' _ "$t" "$toc"
done
check "dump: other tables keep their rows" grep -qE " TABLE DATA public scans " <<<"$toc"
n=$(snapshots)
# keep-daily keeps the newest per day; restic also keeps the oldest snapshot while
# the daily/weekly/monthly slots are not yet filled: 3 same-day backups → 2.
check "retention: 3 same-day snapshots → newest + oldest (got $n)" [ "$n" = 2 ]
files=$(restic_run ls latest)
check "snapshot has dump, manifest, .env, compose.yaml" bash -c \
	'for p in /config/.env /config/compose.yaml; do grep -qx "$p" <<<"$1" || exit 1; done
	 grep -q "^/backup/qr-.*\.dump$" <<<"$1" && grep -q "^/backup/qr-.*\.manifest\.json$" <<<"$1"' _ "$files"
check "no stale locks" [ -z "$(restic_run list locks)" ]
check "pings: 3 start, 3 ok, 0 fail" [ "$(pings /backup/start)/$(pings /backup ok)/$(pings /backup/fail)" = 3/3/0 ]

echo "--- restore drill"
check "restore-test passes" bash "$here/restore-test.sh"
check "pings: restore start + ok" [ "$(pings /restore/start)/$(pings /restore ok)" = 1/1 ]
check "restore-test cleans up" [ -z "$(find "$fx/backups" -name 'restore-test.*')$(docker ps -aq -f name=qr-restore-test)" ]

# A snapshot whose manifest does not match its dump must fail the drill.
mkdir "$fx/tamper"
cp "$latest" "${latest%.manifest.json}.dump" "$fx/tamper/"
sed -i 's/"scans" : 4/"scans" : 999/' "$fx/tamper/${latest##*/}"
RESTIC_MOUNTS=(-v "$fx/tamper:/backup:ro")
restic_run backup --host quantumready --tag quantumready --quiet "/backup/${latest##*/}" "/backup/$(basename "${latest%.manifest.json}.dump")" >/dev/null
RESTIC_MOUNTS=()
if bash "$here/restore-test.sh" >/dev/null 2>"$fx/err"; then bad "restore-test detects a mismatch"; else
	check "restore-test detects a mismatch" grep -q '"scans" : 999' "$fx/err"
fi
check "fail ping names the file and line (restore-test.sh)" grep -Eq '^/restore/fail restore test failed: exit [0-9]+ at restore-test\.sh:[0-9]+$' "$fx/pings.log"

# A dump that carries the rows of a no-data table must fail the drill too,
# even with a manifest that says 0 (the drill counts for real).
mkdir "$fx/withdata"
(cd "$fx" && docker compose exec -T db pg_dump -U qr -d quantumready -Fc </dev/null >"$fx/withdata/qr-20990101T000000Z.dump")
psql -c "$DUMP_MANIFEST_SQL" >"$fx/withdata/qr-20990101T000000Z.manifest.json"
RESTIC_MOUNTS=(-v "$fx/withdata:/backup:ro")
restic_run backup --host quantumready --tag quantumready --quiet /backup/qr-20990101T000000Z.dump /backup/qr-20990101T000000Z.manifest.json >/dev/null
RESTIC_MOUNTS=()
if bash "$here/restore-test.sh" >/dev/null 2>"$fx/err"; then bad "restore-test fails if no-data rows were backed up"; else
	check "restore-test fails if no-data rows were backed up" grep -q '"usage_daily" : 2 }' "$fx/err"
fi
check "pings: restore fail (2)" [ "$(pings /restore/fail)" = 2 ]
# A failure inside common.sh (restic, missing repository) names common.sh.
mkdir "$fx/empty-repo"
if RESTIC_REPOSITORY=$fx/empty-repo bash "$here/restore-test.sh" >/dev/null 2>&1; then bad "restore-test with a missing repository"; else
	check "fail ping names common.sh for a restic failure" grep -Eq '^/restore/fail restore test failed: exit [0-9]+ at common\.sh:[0-9]+$' "$fx/pings.log"
fi

echo "--- failed dump"
before=$(snapshots)
(cd "$fx" && docker compose stop db >/dev/null 2>&1)
if bash "$here/backup.sh" >/dev/null 2>"$fx/err"; then bad "backup fails when the database is down"; else
	ok "backup fails when the database is down"
fi
check "no new dump, no .partial left" [ "$(qr_dumps .dump)/$(qr_dumps .partial)" = 2/0 ]
check "no new snapshot" [ "$(snapshots)" = "$before" ]
check "pings: backup fail" [ "$(pings /backup/fail)" = 1 ]
check "fail ping names the file and line (backup.sh)" grep -Eq '^/backup/fail backup failed: exit [0-9]+ at backup\.sh:[0-9]+$' "$fx/pings.log"

echo
if [ "$fails" = 0 ]; then echo "backup tests: all passed"; else
	echo "backup tests: $fails FAILED"
	exit 1
fi
