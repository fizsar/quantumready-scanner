#!/usr/bin/env bash
# Deletes the pre-deploy dumps (deploy/predeploy-dump.sh) older than N days
# from /opt/quantumready/backups on the VPS. Run by hand only (no timer):
#
#   deploy/prune-predeploy-dumps.sh --dry-run        # lists what it would delete
#   deploy/prune-predeploy-dumps.sh [--days N]       # N defaults to 7
#
# Only files named pre-<name>-YYYYMMDD-HHMM.dump are candidates, and their age
# comes from that UTC date in the name (not the mtime); the daily copies
# (qr-*.dump) and anything else are never touched. Nothing is deleted unless
# the API answers /api/health with "status":"ok" and the last run of
# qr-backup.service succeeded within the last 26 h: the dumps are the fallback
# while that is not so. ls -l of the folder before and after.
#
# Test hooks (deploy/test-prune-predeploy-dumps.sh): QR_REMOTE_SHELL (command
# that runs a script from stdin, default "ssh $QR_DEPLOY_HOST bash -s --"),
# QR_DIR, QR_HEALTH_URL, QR_SYSTEMCTL (command standing in for systemctl) and
# QR_NOW (epoch seconds).
set -euo pipefail

days=7 dry=0
while [ $# -gt 0 ]; do
	case $1 in
	--dry-run) dry=1 ;;
	--days)
		days=${2:-}
		shift
		;;
	*)
		echo "usage: $0 [--dry-run] [--days N]" >&2
		exit 2
		;;
	esac
	shift
done
if [[ ! $days =~ ^[1-9][0-9]{0,2}$ ]]; then
	echo "--days must be a whole number from 1 to 999" >&2
	exit 2
fi
HOST=${QR_DEPLOY_HOST:-fizsar-vps}
read -ra remote <<<"${QR_REMOTE_SHELL:-ssh $HOST bash -s --}"

"${remote[@]}" "${QR_DIR:-/opt/quantumready}" "$days" "$dry" "${QR_HEALTH_URL:-http://127.0.0.1:8080/api/health}" \
	"${QR_SYSTEMCTL:-systemctl}" "${QR_NOW:-}" <<'REMOTE'
set -euo pipefail
dir=$1 days=$2 dry=$3 health_url=$4 systemctl=$5 now=${6:-$(date -u +%s)}
cd "$dir/backups"
echo "==> ls -l $dir/backups (before)"
ls -l </dev/null

health=$(curl -fsS --max-time 5 "$health_url" </dev/null 2>&1) || health="no answer: $health"
case $health in *'"status":"ok"'*) health_ok=1 ;; *) health_ok=0 ;; esac
echo "health: $health"
result=$($systemctl show qr-backup.service -p Result --value </dev/null)
status=$($systemctl show qr-backup.service -p ExecMainStatus --value </dev/null)
ended=$($systemctl show qr-backup.service -p ExecMainExitTimestamp --value </dev/null)
ended_s=0 # date -d "" would be today's midnight
[ -z "$ended" ] || ended_s=$(date -u -d "$ended" +%s 2>/dev/null </dev/null) || ended_s=0
age_h=$(((now - ended_s) / 3600))
if [ "$result" = success ] && [ "$status" = 0 ] && [ "$ended_s" -gt 0 ] && [ "$age_h" -lt 26 ]; then backup_ok=1; else backup_ok=0; fi
echo "last daily backup: Result=$result ExecMainStatus=$status ended=${ended:-never} (${age_h} h ago)"

delete=()
for f in pre-*.dump; do
	[ -e "$f" ] || continue
	if [[ ! $f =~ ^pre-[a-z0-9][a-z0-9-]*-([0-9]{8})-([0-9]{2})([0-9]{2})\.dump$ ]]; then
		echo "skip   $f (name without a date)"
		continue
	fi
	d=${BASH_REMATCH[1]}
	t=$(date -u -d "${d:0:4}-${d:4:2}-${d:6:2} ${BASH_REMATCH[2]}:${BASH_REMATCH[3]}" +%s 2>/dev/null </dev/null) || {
		echo "skip   $f (invalid date)"
		continue
	}
	age=$((now - t))
	shown=$(awk -v s="$age" 'BEGIN { printf "%.1f", s / 86400 }')
	if [ "$age" -gt $((days * 86400)) ]; then
		echo "delete $f ($shown days old)"
		delete+=("$f")
	else
		echo "keep   $f ($shown days old)"
	fi
done

if [ ${#delete[@]} -eq 0 ]; then
	echo "==> nothing older than $days days"
	exit 0
fi
checks="production healthy and last daily backup ok"
[ $health_ok = 1 ] && [ $backup_ok = 1 ] || checks="checks NOT passed: a real run would refuse"
if [ "$dry" = 1 ]; then
	echo "==> dry run: would delete ${#delete[@]} file(s) ($checks); nothing deleted"
	exit 0
fi
if [ $health_ok != 1 ] || [ $backup_ok != 1 ]; then
	echo "refusing to delete: production must be healthy and the last daily backup must have succeeded in the last 26 h" >&2
	exit 1
fi
rm -f -- "${delete[@]}"
echo "==> deleted ${#delete[@]} file(s)"
echo "==> ls -l $dir/backups (after)"
ls -l </dev/null
REMOTE
