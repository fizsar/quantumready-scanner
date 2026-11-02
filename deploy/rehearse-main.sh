#!/usr/bin/env bash
# Local rehearsal of the first deploy of main (Apertura step 7) and of the
# rollback to showcase, with the VPS protocol (docs/despliegue-main.md):
#
#   deploy/rehearse-main.sh [DUMP]
#   QR_REHEARSE_MAIN=<commit> deploy/rehearse-main.sh [DUMP]   # default origin/main
#   QR_REHEARSE_FROM=<commit> ...   # what production runs, and the rollback (default 62eac88)
#
#   no DUMP: showcase starts empty; two real scans (fizsar.com and
#            quantumready.fizsar.com), one of them aged 40 days by SQL.
#   DUMP:    the production dump (pg_dump -Fc) is restored first.
#
# Stages: FROM (showcase 62eac88 by default) -> main (origin/main) -> FROM -> main.
# Each deploy does what deploy.sh does on the VPS: compose.prod.yaml of that
# commit, .env.next, pin_images, recreate_plan + db_guard (db must not be
# recreated), pull, up --wait, health. Everything lives in /dev/shm (RAM):
# the database volume is a tmpfs, the GHCR login a throwaway DOCKER_CONFIG.
# The caller deletes the dump. Needs docker, gh (read:packages), python3 with
# PyYAML, and ports 127.0.0.1:8080 and :8081 free.
# shellcheck disable=SC2015,SC2317 # ok and bad never fail; cleanup runs from the trap
set -uo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
ROOT=/dev/shm/qrr
W=$ROOT/quantumready # the project name is the directory's, as on the VPS
DUMP=${1:-}
ORIGIN=https://quantumready.fizsar.com
SHOWCASE=${QR_REHEARSE_FROM:-62eac88} # where production starts, and where a rollback goes
MAIN=$(git -C "$REPO" rev-parse --short=7 "${QR_REHEARSE_MAIN:-origin/main}")
FAILS=0

say() { printf '\n== %s\n' "$*"; }
ok() { echo "   ok: $*"; }
bad() { echo "   FAIL: $*"; FAILS=$((FAILS + 1)); }
die() { echo "   FATAL: $*"; exit 1; }
dc() { (cd "$W" && docker compose "$@"); }
psql_owner() { docker exec -i quantumready-db-1 psql -X -q -v ON_ERROR_STOP=1 -U quantumready -d quantumready -tA "$@"; }

STARTED=0 # set once this run owns the project: cleanup touches nothing before
cleanup() {
	if [ "$STARTED" = 1 ]; then
		[ -f "$W/compose.yaml" ] && dc down -v --remove-orphans >/dev/null 2>&1
		docker rm -f qrr-restore >/dev/null 2>&1
		docker volume rm quantumready_db-data >/dev/null 2>&1
		echo "cleanup: stack and its volume removed"
	fi
	rm -rf "$ROOT"
	echo "cleanup: $ROOT removed"
}
trap cleanup EXIT

[ -n "$(docker ps -aq --filter label=com.docker.compose.project=quantumready)" ] && die "a compose project 'quantumready' exists already"
docker volume inspect quantumready_db-data >/dev/null 2>&1 && die "volume quantumready_db-data exists already"
STARTED=1
rm -rf "$ROOT"
mkdir -p "$W" "$ROOT/docker"
chmod 700 "$ROOT" "$W" "$ROOT/docker"
export DOCKER_CONFIG=$ROOT/docker
[ -d "$HOME/.docker/cli-plugins" ] && ln -s "$HOME/.docker/cli-plugins" "$DOCKER_CONFIG/cli-plugins"
gh auth token | docker login ghcr.io -u fizsar --password-stdin >/dev/null 2>&1 || die "GHCR login"

# The helpers deploy.sh sends to the VPS, from main (the checkout that deploys).
# shellcheck disable=SC1091
. "$REPO/deploy/compose-plan.sh"
# shellcheck disable=SC1091
. "$REPO/deploy/pin-images.sh"
set_env() { if grep -q "^$2=" "$1"; then sed -i "s|^$2=.*|$2=$3|" "$1"; else echo "$2=$3" >>"$1"; fi; }

# The database volume in RAM; the same edit in every compose, so the plan
# compares them as on the VPS.
tmpfsify() {
	python3 -c '
import sys, yaml
d = yaml.safe_load(sys.stdin)
d["volumes"]["db-data"] = {"driver_opts": {"type": "tmpfs", "device": "tmpfs", "o": "size=1g,uid=70,gid=70,mode=0700"}}
yaml.safe_dump(d, sys.stdout, sort_keys=False)'
}

# deploy COMMIT: what deploy.sh does, locally.
deploy() {
	local commit=$1 tag=sha-$1 dry refs api_ref web_ref pin=0 plan
	say "deploy $tag"
	dry=$(cd "$REPO" && QR_DEPLOY_DRY_RUN=1 QR_DEPLOY_PIN_UNRECORDED=1 bash deploy/deploy.sh "$tag" 2>&1) || die "deploy.sh refuses $tag: $dry"
	api_ref=$(sed -n 's/^==> api \(ghcr.io[^ ]*\)$/\1/p' <<<"$dry")
	web_ref=$(sed -n 's/^==> web \(ghcr.io[^ ]*\)$/\1/p' <<<"$dry")
	[ -z "$api_ref$web_ref" ] && pin=1
	echo "   deploy.sh dry run: $(tail -1 <<<"$dry"); recorded digests: ${api_ref:+yes}${api_ref:-no (QR_DEPLOY_PIN_UNRECORDED=1)}"
	git -C "$REPO" show "$commit:deploy/compose.prod.yaml" | tmpfsify >"$W/compose.yaml.new"
	cp -p "$W/.env" "$W/.env.next"
	set_env "$W/.env.next" QR_TAG "$tag"
	set_env "$W/.env.next" QR_TRUSTED_PROXIES "$(docker network inspect web -f '{{range .IPAM.Config}}{{.Subnet}},{{end}}' | sed 's/,$//')"
	(cd "$W" && pin_images .env.next "$tag" "$api_ref" "$web_ref" "$pin") >"$ROOT/pin.log" 2>&1 || { cat "$ROOT/pin.log"; die "pin_images"; }
	sed 's/^/   /' "$ROOT/pin.log"
	plan=$(cd "$W" && recreate_plan compose.yaml.new .env.next)
	echo "   compose will create or recreate: $(echo "${plan:-nothing}" | paste -sd ' ' -)"
	if [ -f "$W/compose.yaml" ] && ! db_guard "$plan" 0 2>/dev/null; then
		bad "the plan recreates db"
		return 1
	fi
	mv "$W/compose.yaml.new" "$W/compose.yaml"
	mv "$W/.env.next" "$W/.env"
	dc pull --quiet >/dev/null 2>&1 || die "pull"
	if ! dc up -d --wait --remove-orphans >/tmp/qrr-up.log 2>&1; then
		tail -5 /tmp/qrr-up.log
		docker logs quantumready-migrate-1 2>&1 | tail -5
		die "up --wait failed for $tag"
	fi
	curl -fsS --max-time 5 http://127.0.0.1:8080/api/health >/dev/null && ok "api health" || bad "api health"
	curl -fsS -o /dev/null --max-time 5 http://127.0.0.1:8081/ && ok "web" || bad "web"
	echo "   migrate: $(docker logs quantumready-migrate-1 2>&1 | grep -oE '"msg":"[^"]*"' | sort | uniq -c | tr -s ' ' | paste -sd ';' -)"
}

# scan HOST: POST, wait until finished; prints the id.
scan() {
	local id st
	id=$(curl -fsS -X POST -H "Origin: $ORIGIN" -H 'Content-Type: application/json' \
		-d "{\"host\":\"$1\"}" http://127.0.0.1:8080/api/scans | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])') || { bad "POST /api/scans $1"; return 1; }
	for _ in $(seq 60); do
		st=$(curl -fsS "http://127.0.0.1:8080/api/scans/$id" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
		case $st in done | failed) break ;; esac
		sleep 2
	done
	echo "$id $st"
}

# report ID: status and grade as the running API serves them.
report() {
	curl -sS -w ' http=%{http_code}' "http://127.0.0.1:8080/api/scans/$1" | python3 -c '
import json, sys
raw = sys.stdin.read(); body, code = raw.rsplit(" http=", 1)
try:
    d = json.loads(body); r = d.get("report") or {}
    print(code, d.get("status"), d.get("intensity", "-"), r.get("grade"), r.get("score"), (r.get("readiness") or {}).get("score"))
except Exception:
    print(code, body[:80])'
}

logs_clean() {
	local n
	n=$(for s in api worker; do docker logs "quantumready-$s-1" 2>&1; done | grep -cE '"level":"(WARN|ERROR)"')
	[ "$n" = 0 ] && ok "no WARN/ERROR in api and worker logs" || {
		echo "   $n WARN/ERROR lines:"
		for s in api worker; do docker logs "quantumready-$s-1" 2>&1 | grep -E '"level":"(WARN|ERROR)"' | cut -c1-200 | sort | uniq -c | sed 's/^/     /'; done
	}
}

# --- showcase, as in production today --------------------------------------
cat >"$W/.env" <<EOF
POSTGRES_USER=quantumready
POSTGRES_PASSWORD=$(openssl rand -hex 32)
POSTGRES_DB=quantumready
SCAN_TARGET_COOLDOWN=5s
EOF
chmod 600 "$W/.env"
say "the .env lines main needs (generated here as on the VPS; showcase ignores them)"
{
	echo "QR_APP_DB_PASSWORD=$(openssl rand -hex 32)"
	echo "QR_WORKER_DB_PASSWORD=$(openssl rand -hex 32)"
	echo "ALTCHA_HMAC_KEY=$(openssl rand -hex 32)"
	echo "PUBLIC_ORIGIN=$ORIGIN"
	echo "ACCOUNTS_ENABLED=false"
	echo "PUBLIC_SCAN_OPEN=false"
} >>"$W/.env"
ok "added: $(grep -oE '^(QR_APP_DB_PASSWORD|QR_WORKER_DB_PASSWORD|ALTCHA_HMAC_KEY|PUBLIC_ORIGIN|ACCOUNTS_ENABLED|PUBLIC_SCAN_OPEN)' "$W/.env" | paste -sd ' ' -)"

if [ -n "$DUMP" ]; then
	say "restore the production dump into an empty database"
	git -C "$REPO" show "$SHOWCASE:deploy/compose.prod.yaml" | tmpfsify >"$W/compose.yaml"
	set_env "$W/.env" QR_TAG "sha-$SHOWCASE"
	dc up -d --wait db >/dev/null 2>&1 || die "db up"
	docker exec -i quantumready-db-1 pg_restore -U quantumready -d quantumready --exit-on-error <"$DUMP" || die "pg_restore of the dump"
	ok "restored: $(psql_owner -c "select count(*) || ' scans, ' || count(*) filter (where created_at < now() - interval '30 days') || ' older than 30 days' from scans")"
	echo "   goose: $(psql_owner -c "select string_agg(version_id::text, ',' order by version_id) from goose_db_version where is_applied and version_id > 0")"
fi
deploy "$SHOWCASE"
if [ -z "$DUMP" ]; then
	read -r OLD st < <(scan fizsar.com)
	read -r AGED st2 < <(scan quantumready.fizsar.com)
	[ "$st$st2" = "donedone" ] && ok "two scans on showcase" || bad "showcase scans: $st $st2"
	# Its expiry too: main sets one when the scan is created (00021); showcase did not.
	psql_owner -c "update scans set created_at = created_at - interval '40 days', finished_at = finished_at - interval '40 days', expires_at = expires_at - interval '40 days' where id = '$AGED'" >/dev/null
	ok "aged $AGED by 40 days (a production scan older than 30 days)"
else
	OLD=$(psql_owner -c "select id from scans where status = 'done' and created_at > now() - interval '30 days' order by created_at desc limit 1")
	AGED=$(psql_owner -c "select id from scans where status = 'done' and created_at < now() - interval '30 days' order by created_at limit 1")
fi
echo "   old scan $OLD: $(report "$OLD")"
[ -n "$AGED" ] && echo "   aged scan $AGED: $(report "$AGED")"
BEFORE=$(psql_owner -c "select count(*) from scans")
OLDCOUNT=$(psql_owner -c "select count(*) from scans where created_at < now() - interval '30 days' and status in ('done','failed')")
echo "   finished scans older than 30 days, by day (what the first purge deletes): $(psql_owner -c "select coalesce(string_agg(d || ' ' || n, ', ' order by d), 'none') from (select created_at::date d, count(*) n from scans where created_at < now() - interval '30 days' and status in ('done','failed') group by 1) x")"
echo "   scans by status: $(psql_owner -c "select string_agg(status || ' ' || n, ', ' order by status) from (select status, count(*) n from scans group by 1) x"); newest $(psql_owner -c "select max(created_at)::date from scans"), oldest $(psql_owner -c "select min(created_at)::date from scans")"

# --- main ---------------------------------------------------------------------
deploy "$MAIN" || die "deploy of main"
# Every migration of that commit applied: 0 and the numbers of its files.
want=$( (
	echo 0
	git -C "$REPO" ls-tree --name-only "$MAIN" migrations/ | sed -n 's|^migrations/0*\([0-9][0-9]*\)_.*\.sql$|\1|p'
) | sort -n | paste -sd, -)
got=$(psql_owner -c "select string_agg(version_id::text, ',' order by version_id) from goose_db_version where is_applied")
[ "$got" = "$want" ] && ok "goose: $got" || bad "goose: $got, want $want"
docker logs quantumready-migrate-1 2>&1 | grep -q 'login users' && ok "migrate gave LOGIN: $(docker logs quantumready-migrate-1 2>&1 | grep -o '"roles":[^}]*' | head -1)" || bad "no 'login users' line in migrate"
users=$(psql_owner -c "select string_agg(distinct usename, ',' order by usename) from pg_stat_activity where usename like 'qr_%'")
[ "$users" = "qr_app,qr_worker" ] && ok "services log in as $users" || bad "logged-in service users: $users"
priv=$(psql_owner -c "select string_agg(rolname || ':' || rolsuper::text || '/' || rolbypassrls::text || '/' || rolcanlogin::text, ' ' order by rolname) from pg_roles where rolname like 'qr_%'")
echo "   roles (super/bypassrls/login): $priv"
img=$(docker inspect -f '{{.Config.Image}}' quantumready-api-1)
net=$(docker network ls --format '{{.Name}}' | grep -E '^quantumready_backend$')
# shellcheck source=/dev/null # the throwaway .env written above
. "$W/.env"
refusal=$(timeout 60 docker run --rm --network "$net" -e DATABASE_URL="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@db:5432/$POSTGRES_DB?sslmode=disable" -e API_ADDR=:8080 "$img" 2>&1 | tail -1)
grep -q 'superuser' <<<"$refusal" && ok "the API as the owner refuses to start" || bad "API as owner: $refusal"
echo "   targets: $(curl -fsS http://127.0.0.1:8080/api/targets)"
code=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/auth/me)
[ "$code" = 404 ] && ok "/api/auth/me 404 (accounts off)" || bad "/api/auth/me $code"
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Origin: $ORIGIN" -H 'Content-Type: application/json' -d '{"host":"example.com"}' http://127.0.0.1:8080/api/scans)
[ "$code" = 403 ] && ok "a domain outside the allowlist: 403 (scanner closed)" || bad "example.com: $code"
sleep 5 # the worker's purge runs at start
echo "   old scan $OLD: $(report "$OLD")"
if [ -n "$AGED" ]; then
	r=$(report "$AGED")
	[[ $r == 404* ]] && ok "aged scan purged (404)" || bad "aged scan: $r"
fi
AFTER=$(psql_owner -c "select count(*) from scans")
echo "   scans: $BEFORE before, $AFTER after the first purge ($OLDCOUNT finished ones were older than 30 days)"
echo "   worker: $(docker logs quantumready-worker-1 2>&1 | grep -i 'expired public scans' | grep -oE '"(msg|deleted|count|n)":[^,}]*' | paste -sd ' ' - | head -c 200)"
nullexp=$(psql_owner -c "select count(*) from scans where organization_id is null and expires_at is null")
[ "$nullexp" = 0 ] && ok "every public scan has an expiry" || bad "$nullexp public scans without expires_at"
read -r NEW st < <(scan quantumready.fizsar.com)
[ "$st" = "done" ] && ok "new scan on main: $(report "$NEW")" || bad "new scan on main: $st"
logs_clean

say "restore drill on a dump of the main database (as restore-test.sh does)"
docker exec quantumready-db-1 pg_dump -U quantumready -d quantumready -Fc >"$ROOT/main.dump"
docker run -d --name qrr-restore --network none -e POSTGRES_PASSWORD=x -e POSTGRES_DB=restore \
	postgres:17-alpine@sha256:eee6c6e1b0894fa4fe0075eb054af9fd1cf39a35321c2d4110626cfe506411fb >/dev/null
for _ in $(seq 30); do docker exec qrr-restore pg_isready -U postgres -h 127.0.0.1 >/dev/null 2>&1 && break; sleep 1; done
# As restore-test.sh does: the service roles first.
read -ra SERVICE_ROLES < <(bash -c '. "$1/deploy/backup/common.sh" >/dev/null 2>&1; echo "${SERVICE_ROLES[@]}"' _ "$REPO")
echo "   service roles created before pg_restore: ${SERVICE_ROLES[*]:-none (common.sh without SERVICE_ROLES)}"
for role in "${SERVICE_ROLES[@]}"; do
	docker exec qrr-restore psql -h 127.0.0.1 -U postgres -d restore -q -c "CREATE ROLE $role NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE" </dev/null
done
if out=$(docker exec -i qrr-restore pg_restore -h 127.0.0.1 -U postgres -d restore --no-owner --exit-on-error <"$ROOT/main.dump" 2>&1); then
	ok "pg_restore --no-owner --exit-on-error works on the main schema"
else
	bad "restore drill fails on the main schema: $(head -2 <<<"$out" | tr '\n' ' ' | cut -c1-260)"
fi
docker rm -f qrr-restore >/dev/null
rm -f "$ROOT/main.dump"

# --- rollback to showcase, with main's migrations applied --------------------
deploy "$SHOWCASE" || die "rollback to showcase"
echo "   old scan $OLD: $(report "$OLD")"
echo "   main's scan $NEW: $(report "$NEW")"
# Main hands out a public result of the last hour instead of scanning again:
# two hours older, fizsar.com gets a new scan.
psql_owner -c "update scans set created_at = created_at - interval '2 hours', finished_at = finished_at - interval '2 hours' where host = 'fizsar.com'" >/dev/null
read -r RB st < <(scan fizsar.com)
[ "$RB" != "$OLD" ] && ok "the rollback scan is a new one" || bad "the rollback scan is the old one ($RB)"
[ "$st" = "done" ] && ok "new scan on showcase after the rollback: $(report "$RB")" || bad "scan after rollback: $st"
logs_clean

# --- and forward again --------------------------------------------------------
deploy "$MAIN" || die "main again"
for id in "$OLD" "$NEW" "$RB"; do echo "   $id: $(report "$id")"; done
echo "   rollback scan expiry: $(psql_owner -c "select coalesce(expires_at::text, 'NULL') || ', created ' || created_at::date from scans where id = '$RB'")"
logs_clean

say "result: $FAILS failure(s)"
exit "$FAILS"
