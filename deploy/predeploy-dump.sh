#!/usr/bin/env bash
# Pre-deploy dump on the VPS, kept next to the daily ones:
#
#   deploy/predeploy-dump.sh pre-m1   # -> /opt/quantumready/backups/pre-m1-YYYYMMDD-HHMM.dump
#
# pg_dump -Fc inside the db container (credentials from the container's own
# environment, never from .env), mode 600, validated with pg_restore --list,
# which it summarizes as the tables that carry data. Like backup.sh, the rows
# of NO_DATA_TABLES (deploy/backup/common.sh) are left out: a dump is a
# backup too. Every command gets </dev/null, because the remote script itself
# comes through stdin. On failure no file (or .partial) is left.
#
# Test hooks (deploy/test-predeploy-dump.sh): QR_REMOTE_SHELL (command that
# runs a script from stdin, default "ssh $QR_DEPLOY_HOST bash -s --"), QR_DIR.
set -euo pipefail

NAME=${1:-}
if [[ ! $NAME =~ ^[a-z0-9][a-z0-9-]{0,39}$ ]]; then
	echo "usage: $0 <name>   (lower case, digits and -, e.g. pre-m1)" >&2
	exit 2
fi
HOST=${QR_DEPLOY_HOST:-fizsar-vps}
read -ra remote <<<"${QR_REMOTE_SHELL:-ssh $HOST bash -s --}"
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=deploy/backup/common.sh
. "$here/backup/common.sh"

"${remote[@]}" "${QR_DIR:-/opt/quantumready}" "$NAME" "${DUMP_EXCLUDE[@]}" <<'REMOTE'
set -euo pipefail
dir=$1 name=$2
shift 2
cd "$dir"
f=backups/$name-$(date -u +%Y%m%d-%H%M).dump
[ ! -e "$f" ] || { echo "refusing: $dir/$f already exists" >&2; exit 1; }
trap 'rm -f "$f.partial" "$f.toc"' EXIT
umask 077
db() { docker compose -f compose.yaml exec -T db "$@"; }
db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc "$@"' _ "$@" </dev/null >"$f.partial"
[ "$(head -c 5 "$f.partial")" = PGDMP ] || { echo "not a pg_dump custom archive" >&2; exit 1; }
db pg_restore --list <"$f.partial" >"$f.toc"
mv "$f.partial" "$f"
chmod 600 "$f"
echo "==> $dir/$f"
ls -l "$f"
echo "tables with data: $(awk '$4 == "TABLE" && $5 == "DATA" {print $7}' "$f.toc" | sort -u | tr '\n' ' ')"
echo "tables without data: $(comm -23 <(awk '$4 == "TABLE" && $5 != "DATA" {print $6}' "$f.toc" | sort -u) <(awk '$4 == "TABLE" && $5 == "DATA" {print $7}' "$f.toc" | sort -u) | tr '\n' ' ')"
echo "==> dump ok"
REMOTE
