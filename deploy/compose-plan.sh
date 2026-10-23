# shellcheck shell=bash
# What `docker compose up` would create or recreate, and the guard that keeps
# the database container from being recreated by surprise. deploy.sh sends
# this file to the VPS ahead of its own script; deploy/test-compose-plan.sh
# tests it against a throwaway compose project.
#
# Compose recreates a container when the hash of its service configuration
# differs from the com.docker.compose.config-hash label of the running one.
# That includes the image reference text: on 23/10/2026 changing
# postgres:17-alpine to postgres:17-alpine@sha256:... (the same image)
# restarted the database. `docker compose config --hash` computes the same
# hash from the new files, before anything changes.

# recreate_plan COMPOSE_FILE ENV_FILE: one line per service that up would
# touch, "SERVICE recreate" (configuration changed) or "SERVICE create" (no
# container yet). Nothing when everything would stay as it is.
recreate_plan() {
	local file=$1 env=$2 svc hash cid running
	docker compose -f "$file" --env-file "$env" config --hash '*' </dev/null | while read -r svc hash; do
		cid=$(docker compose -f "$file" --env-file "$env" ps -aq "$svc" </dev/null | head -1)
		if [ -z "$cid" ]; then
			echo "$svc create"
			continue
		fi
		running=$(docker inspect -f '{{index .Config.Labels "com.docker.compose.config-hash"}}' "$cid" </dev/null)
		[ "$hash" = "$running" ] || echo "$svc recreate"
	done
}

# db_guard PLAN ALLOW: refuses a plan that recreates the database container,
# unless ALLOW is 1 (QR_ALLOW_DB_RECREATE=1: a deliberate PostgreSQL change,
# after a backup and a dump). Creating it (first deploy) is fine.
db_guard() {
	grep -qx 'db recreate' <<<"$1" || return 0
	if [ "$2" = 1 ]; then
		echo "==> db will be recreated (QR_ALLOW_DB_RECREATE=1)"
		return 0
	fi
	echo "refusing to deploy: compose would recreate the database container (db)." >&2
	echo "If that is the plan (a PostgreSQL update), take a backup and a dump first," >&2
	echo "then run again with QR_ALLOW_DB_RECREATE=1. Nothing was changed on the server." >&2
	return 1
}
