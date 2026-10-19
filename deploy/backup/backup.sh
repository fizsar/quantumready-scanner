#!/usr/bin/env bash
# Daily backup of the QuantumReady database to restic (Backblaze B2, encrypted
# at the source). Run by qr-backup.service; see CLAUDE.md ("Copias de seguridad").
#
#   backup.sh        dump, upload, apply retention, keep the last KEEP_LOCAL dumps locally
#   backup.sh init   create the restic repository (once)
#
# Environment (backup.env): RESTIC_REPOSITORY, RESTIC_PASSWORD,
# AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_DEFAULT_REGION, HC_PING_URL.
set -Eeuo pipefail # -E: the ERR trap also fires inside functions
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=deploy/backup/common.sh
. "$here/common.sh"
: "${RESTIC_REPOSITORY:?set in backup.env}" "${RESTIC_PASSWORD:?set in backup.env}"
KEEP_LOCAL=${KEEP_LOCAL:-2}

if [ "${1:-}" = init ]; then
	restic_run init
	exit
fi

partial=()
on_error() {
	local rc=$? line=$1
	rm -f "${partial[@]}" # a failed dump never survives as a valid-looking file
	echo "backup FAILED (exit $rc, line $line)" >&2
	hc "${HC_PING_URL:-}" /fail "backup failed: exit $rc at line $line"
	exit "$rc"
}
trap 'on_error $LINENO' ERR

hc "${HC_PING_URL:-}" /start
cd "$QR_DIR"
set -a
# shellcheck source=/dev/null
. ./.env
set +a
umask 077
mkdir -p "$BACKUP_DIR"

ts=$(date -u +%Y%m%dT%H%M%SZ)
dump=$BACKUP_DIR/qr-$ts.dump
manifest=$BACKUP_DIR/qr-$ts.manifest.json
partial=("$dump.partial" "$manifest.partial")

# 1. Dump (without the rows of NO_DATA_TABLES), check it is a readable archive,
#    record the row counts (0 for NO_DATA_TABLES).
docker compose exec -T db pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc "${DUMP_EXCLUDE[@]}" </dev/null >"$dump.partial"
docker compose exec -T db pg_restore --list <"$dump.partial" >/dev/null
docker compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAq -v ON_ERROR_STOP=1 \
	-c "$DUMP_MANIFEST_SQL" </dev/null >"$manifest.partial"
mv "$dump.partial" "$dump"
mv "$manifest.partial" "$manifest"
partial=()

# 2. Upload the dump, its manifest, .env and compose.yaml (encrypted by restic).
RESTIC_MOUNTS=(-v "$BACKUP_DIR:/backup:ro" -v "$QR_DIR/.env:/config/.env:ro" -v "$QR_DIR/compose.yaml:/config/compose.yaml:ro")
restic_run backup --host quantumready --tag quantumready \
	"/backup/${dump##*/}" "/backup/${manifest##*/}" /config/.env /config/compose.yaml

# 3. Retention. Each snapshot has different file names, so group by host and
# tags (the default, host+paths, would put every snapshot in its own group and
# never forget anything).
RESTIC_MOUNTS=()
restic_run forget --host quantumready --tag quantumready --group-by host,tags \
	--keep-daily 7 --keep-weekly 4 --keep-monthly 6 --prune
if [ "$(date -u +%u)" = 7 ] || [ "${FORCE_CHECK:-0}" = 1 ]; then
	restic_run check --read-data-subset=5% # weekly: actually read part of the data
fi

# 4. Local copies: keep the last KEEP_LOCAL (other files in backups/ are left alone).
find "$BACKUP_DIR" -maxdepth 1 -name 'qr-*.dump' -printf '%f\n' | sort -r | tail -n +"$((KEEP_LOCAL + 1))" |
	while read -r old; do rm -f "$BACKUP_DIR/$old" "$BACKUP_DIR/${old%.dump}.manifest.json"; done

summary="ok ${dump##*/} $(stat -c %s "$dump") bytes rows=$(cat "$manifest")"
echo "backup $summary"
hc "${HC_PING_URL:-}" "" "$summary"
