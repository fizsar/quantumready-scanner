# shellcheck shell=bash
# Shared by backup.sh, restore-test.sh, test.sh and b2-verify.sh.

# restic runs from the official image, pinned by digest (bump tag and digest together).
RESTIC_IMAGE=${RESTIC_IMAGE:-restic/restic:0.19.1@sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510}
# PostgreSQL 17.11, the same digest as deploy/compose.prod.yaml (check_images.py checks it).
POSTGRES_IMAGE=${POSTGRES_IMAGE:-postgres:17-alpine@sha256:eee6c6e1b0894fa4fe0075eb054af9fd1cf39a35321c2d4110626cfe506411fb}
QR_DIR=${QR_DIR:-/opt/quantumready}
BACKUP_DIR=${BACKUP_DIR:-$QR_DIR/backups}
RESTIC_MOUNTS=()

# Tables whose rows never go into a backup (M1 usage counters and the domains
# of refused scans, deleted after 30 days): pg_dump keeps their schema but not
# their data, so a restore brings them back empty.
NO_DATA_TABLES=(usage_daily blocked_targets_daily)

# The services' roles (migrations 00004 and 00005). Roles belong to the
# cluster, not to a database, so pg_dump leaves them out; but the schema
# grants to them and its row-level security policies name them, so a restore
# into a new PostgreSQL fails ("role qr_app does not exist") unless they are
# created first, as restore-test.sh does (NOLOGIN, as the migrations leave
# them; migrate gives qr_app and qr_worker LOGIN and their passwords).
# shellcheck disable=SC2034 # used by restore-test.sh and test.sh
SERVICE_ROLES=(qr_app qr_worker qr_mailer)

# One JSON object {table: rows} for every table in the public schema, keys in
# order, so a manifest and a restored database compare as plain strings.
# MANIFEST_SQL counts every table; DUMP_MANIFEST_SQL (backup.sh) records 0 for
# NO_DATA_TABLES, so the restore drill, which counts for real, proves they
# came back empty.
# shellcheck disable=SC2016 # $ is SQL here
_rows='(xpath($$/row/c/text()$$, query_to_xml(format($$SELECT count(*) AS c FROM %I.%I$$, t.table_schema, t.table_name), false, true, $$$$)))[1]::text::bigint'
# shellcheck disable=SC2016 # $ is SQL here
_manifest_sql() {
	printf 'SELECT coalesce(json_object_agg(t.table_name, %s ORDER BY t.table_name), $${}$$)
  FROM information_schema.tables t
  WHERE t.table_schema = $$public$$ AND t.table_type = $$BASE TABLE$$' "$1"
}
_no_data=$(printf '$$%s$$,' "${NO_DATA_TABLES[@]}")
# shellcheck disable=SC2034 # used by the scripts that source this
MANIFEST_SQL=$(_manifest_sql "$_rows")
# shellcheck disable=SC2034
DUMP_MANIFEST_SQL=$(_manifest_sql "CASE WHEN t.table_name IN (${_no_data%,}) THEN 0 ELSE $_rows END")
# shellcheck disable=SC2034
DUMP_EXCLUDE=()
for _t in "${NO_DATA_TABLES[@]}"; do DUMP_EXCLUDE+=("--exclude-table-data=public.$_t"); done
unset _t _no_data

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
