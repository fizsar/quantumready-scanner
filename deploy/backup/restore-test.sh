#!/usr/bin/env bash
# Restore drill: restores the latest restic snapshot into a throwaway
# PostgreSQL (separate container, no network, data in tmpfs) and checks that
# its tables and row counts match the manifest recorded at backup time. The
# manifest records 0 for NO_DATA_TABLES (common.sh) and the drill counts them
# for real: a backup that carried their rows fails.
# Never touches the production database. Run monthly by qr-restore-test.timer.
#
# Environment (backup.env): RESTIC_*, AWS_*, HC_RESTORE_PING_URL.
set -Eeuo pipefail # -E: the ERR trap also fires inside functions
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=deploy/backup/common.sh
. "$here/common.sh"
: "${RESTIC_REPOSITORY:?set in backup.env}" "${RESTIC_PASSWORD:?set in backup.env}"

mkdir -p "$BACKUP_DIR"
# Under BACKUP_DIR, not /tmp: the Docker daemon must see it (the service runs with PrivateTmp).
work=$(mktemp -d "$BACKUP_DIR/restore-test.XXXXXX")
pg=qr-restore-test-$$
cleanup() {
	docker rm -f "$pg" >/dev/null 2>&1 || true
	rm -rf "$work"
}
on_error() {
	local rc=$? line=$1
	echo "restore test FAILED (exit $rc, line $line)" >&2
	hc "${HC_RESTORE_PING_URL:-}" /fail "restore test failed: exit $rc at line $line"
	exit "$rc"
}
trap 'on_error $LINENO' ERR
trap cleanup EXIT

hc "${HC_RESTORE_PING_URL:-}" /start
RESTIC_MOUNTS=(-v "$work:/restore")
restic_run restore latest --host quantumready --tag quantumready --target /restore
dump=$(find "$work/backup" -name 'qr-*.dump' | head -1)
manifest=${dump%.dump}.manifest.json
[ -s "$dump" ] && [ -s "$manifest" ]

pw=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
docker run -d --name "$pg" --network none --tmpfs /var/lib/postgresql/data \
	-e POSTGRES_PASSWORD="$pw" -e POSTGRES_DB=restore "$POSTGRES_IMAGE" >/dev/null
# TCP readiness: during initdb the server only listens on the Unix socket.
for _ in $(seq 1 60); do
	docker exec "$pg" pg_isready -h 127.0.0.1 -U postgres -d restore -q && break
	sleep 1
done
docker exec "$pg" pg_isready -h 127.0.0.1 -U postgres -d restore -q

docker exec -i "$pg" pg_restore -h 127.0.0.1 -U postgres -d restore --no-owner --exit-on-error <"$dump"
got=$(docker exec "$pg" psql -h 127.0.0.1 -U postgres -d restore -tAq -v ON_ERROR_STOP=1 -c "$MANIFEST_SQL")
want=$(cat "$manifest")
if [ "$got" != "$want" ]; then
	echo "MISMATCH in ${dump##*/}" >&2
	echo "  manifest: $want" >&2
	echo "  restored: $got" >&2
	false
fi

summary="ok ${dump##*/} rows=$got"
echo "restore test $summary"
hc "${HC_RESTORE_PING_URL:-}" "" "$summary"
