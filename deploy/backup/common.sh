# shellcheck shell=bash
# Shared by backup.sh, restore-test.sh, test.sh and b2-verify.sh.

# restic runs from the official image, pinned by digest (bump tag and digest together).
RESTIC_IMAGE=${RESTIC_IMAGE:-restic/restic:0.19.1@sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510}
POSTGRES_IMAGE=${POSTGRES_IMAGE:-postgres:17-alpine}
QR_DIR=${QR_DIR:-/opt/quantumready}
BACKUP_DIR=${BACKUP_DIR:-$QR_DIR/backups}
RESTIC_MOUNTS=()

# One JSON object {table: rows} for every table in the public schema, keys in
# order, so a manifest and a restored database compare as plain strings.
# shellcheck disable=SC2016,SC2034 # $ is SQL here; used by the scripts that source this
MANIFEST_SQL='SELECT coalesce(json_object_agg(t.table_name,
    (xpath($$/row/c/text()$$, query_to_xml(format($$SELECT count(*) AS c FROM %I.%I$$, t.table_schema, t.table_name), false, true, $$$$)))[1]::text::bigint
    ORDER BY t.table_name), $${}$$)
  FROM information_schema.tables t
  WHERE t.table_schema = $$public$$ AND t.table_type = $$BASE TABLE$$'

# restic ARGS… — runs restic in a container as the calling user (restored
# files stay ours), without a cache (small repository). RESTIC_MOUNTS adds -v
# options; a local repository path is mounted at the same path.
restic_run() {
	local repo=()
	case "$RESTIC_REPOSITORY" in /*) repo=(-v "$RESTIC_REPOSITORY:$RESTIC_REPOSITORY") ;; esac
	docker run --rm -i --user "$(id -u):$(id -g)" --hostname quantumready \
		-e RESTIC_REPOSITORY -e RESTIC_PASSWORD \
		-e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_DEFAULT_REGION \
		"${repo[@]}" "${RESTIC_MOUNTS[@]}" "$RESTIC_IMAGE" --no-cache "$@"
}

# hc URL [SUFFIX] [BODY] — pings Healthchecks.io (/start, /fail or success).
# Optional (empty URL: no-op) and never fatal: a monitoring hiccup must not
# fail a good backup; a missing success ping is what raises the alarm.
hc() {
	local url=$1 suffix=${2:-} body=${3:-}
	[ -n "$url" ] || return 0
	curl -fsS -m 10 --retry 3 -o /dev/null --data-raw "$body" "$url$suffix" ||
		echo "warning: healthcheck ping ($suffix) failed" >&2
}
