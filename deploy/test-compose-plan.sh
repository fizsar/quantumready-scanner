#!/usr/bin/env bash
# Tests deploy/compose-plan.sh (what deploy.sh tells before `up`, and the
# database guard) on a throwaway compose project, and checks the plan against
# what compose then really does. Needs docker. No VPS.
# shellcheck disable=SC2015,SC2317 # ok/FAIL reporting never fails; cleanup runs from the trap
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=deploy/compose-plan.sh
. "$here/compose-plan.sh"
fx=$(mktemp -d "${XDG_CACHE_HOME:-$HOME/.cache}/qr-plan-test.XXXXXX")
cleanup() {
	(cd "$fx" && docker compose -f compose.yaml --env-file .env down -t 0 --remove-orphans >/dev/null 2>&1) || true
	rm -rf "$fx"
}
trap cleanup EXIT
cd "$fx"

# The same image as production's database, idle (no PostgreSQL started).
PG=$(grep -o 'postgres:17-alpine@sha256:[0-9a-f]*' "$here/compose.prod.yaml")
compose() { # compose DB_IMAGE [EXTRA_SERVICE]
	cat <<EOF
name: qr-plan-test
services:
  db:
    image: $1
    entrypoint: ["sleep", "600"]
    environment:
      DB_SETTING: \${DB_SETTING:-a}
  app:
    image: $PG
    entrypoint: ["sleep", "600"]
    environment:
      APP_VERSION: \${APP_VERSION:-1}
EOF
	[ -z "${2:-}" ] || printf '  %s:\n    image: %s\n    entrypoint: ["sleep", "600"]\n' "$2" "$PG"
}
fails=0
ok() { echo "ok   $*"; }
bad() {
	echo "FAIL $*"
	fails=1
}
plan_is() { # plan_is DESCRIPTION WANT COMPOSE_FILE ENV_FILE
	local got
	got=$(recreate_plan "$3" "$4" | sort | paste -sd ' ' -)
	[[ $got == "$2" ]] && ok "$1: ${got:-nothing}" || bad "$1: plan '$got', want '$2'"
}
ids() { docker compose -f compose.yaml --env-file .env ps -aq db app | sort | paste -sd ' ' -; }

compose "$PG" >compose.yaml
printf 'APP_VERSION=1\nDB_SETTING=a\n' >.env
plan_is "nothing running yet" "app create db create" compose.yaml .env
docker compose -f compose.yaml --env-file .env up -d >/dev/null 2>&1
plan_is "running as configured" "" compose.yaml .env

# A new release of the app only.
cp .env .env.next && sed -i 's/APP_VERSION=1/APP_VERSION=2/' .env.next
plan_is "new app version" "app recreate" compose.yaml .env.next
db_guard "$(recreate_plan compose.yaml .env.next)" 0 >/dev/null 2>&1 && ok "guard lets an app-only plan through" || bad "guard refused an app-only plan"

# The incident of 23/10/2026: the same image, written with an explicit registry.
compose "docker.io/library/$PG" >compose.next.yaml
plan_is "same image, other reference text" "db recreate" compose.next.yaml .env
db_guard "$(recreate_plan compose.next.yaml .env)" 0 2>err && bad "guard let a db recreate through" ||
	{ grep -q "QR_ALLOW_DB_RECREATE=1" err && ok "guard refuses a db recreate, and says how to allow it" || bad "guard message: $(cat err)"; }
db_guard "$(recreate_plan compose.next.yaml .env)" 1 >/dev/null && ok "guard allows it with QR_ALLOW_DB_RECREATE=1" || bad "guard refused with allow=1"

# A setting of the database itself, and a new service.
cp .env .env.db && sed -i 's/DB_SETTING=a/DB_SETTING=b/' .env.db
plan_is "database setting changed" "db recreate" compose.yaml .env.db
compose "$PG" extra >compose.extra.yaml
plan_is "a new service" "extra create" compose.extra.yaml .env

# The plan is what compose does: apply the new app version and compare.
before=$(ids)
db_before=$(docker compose -f compose.yaml --env-file .env ps -aq db)
mv .env.next .env
docker compose -f compose.yaml --env-file .env up -d >/dev/null 2>&1
[[ $(docker compose -f compose.yaml --env-file .env ps -aq db) == "$db_before" && $(ids) != "$before" ]] &&
	ok "compose then recreated app and kept db, as planned" || bad "compose did something else than planned"
plan_is "after up" "" compose.yaml .env

# deploy.sh uses it, in this order: the functions go first, the plan and the
# guard run before the new files replace the old ones and before `up`.
line() { grep -nF -- "$1" "$here/deploy.sh" | head -1 | cut -d: -f1; }
order=("cat deploy/compose-plan.sh" 'plan=$(recreate_plan compose.yaml.new .env.next)' 'if ! db_guard "$plan" "$ALLOW_DB"; then'
	"mv compose.yaml.new compose.yaml" "mv .env.next .env" "docker compose up -d")
prev=0 in_order=1
for s in "${order[@]}"; do
	n=$(line "$s")
	[[ -n $n && $n -gt $prev ]] || in_order=0
	prev=${n:-0}
done
[[ $in_order == 1 ]] && ok "deploy.sh: plan and guard before the files move and before up" || bad "deploy.sh: plan/guard missing or out of order"
grep -qF 'bash -s -- "$TAG" "${QR_ALLOW_DB_RECREATE:-0}"' "$here/deploy.sh" && ok "deploy.sh: QR_ALLOW_DB_RECREATE reaches the server (default 0)" || bad "deploy.sh: QR_ALLOW_DB_RECREATE not passed"
exit $fails
