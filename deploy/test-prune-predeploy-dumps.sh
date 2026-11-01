#!/usr/bin/env bash
# Tests deploy/prune-predeploy-dumps.sh without the VPS: the "remote" is bash
# on this machine (the script still arrives through stdin, as over ssh), with
# a stand-in for systemctl, /api/health served from a file and a fixed clock.
# shellcheck disable=SC2015,SC2317 # ok and bad never fail; cleanup runs from the trap
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fx=$(mktemp -d "${XDG_CACHE_HOME:-$HOME/.cache}/qr-prune-test.XXXXXX")
trap 'rm -rf "$fx"' EXIT

# systemctl show qr-backup.service -p <prop> --value, from $fx/unit.<prop>
cat >"$fx/systemctl" <<EOF
#!/usr/bin/env bash
cat "$fx/unit.\$4"
EOF
chmod +x "$fx/systemctl"
now=$(date -u -d '2026-10-30 12:00' +%s)
export QR_REMOTE_SHELL="bash -s --" QR_DIR=$fx QR_HEALTH_URL=file://$fx/health QR_SYSTEMCTL=$fx/systemctl QR_NOW=$now

setup() { # healthy production, a good backup 8 h ago, and the dumps
	rm -rf "$fx/backups" && mkdir "$fx/backups"
	echo '{"status":"ok","version":"07bbc31"}' >"$fx/health"
	echo success >"$fx/unit.Result"
	echo 0 >"$fx/unit.ExecMainStatus"
	echo 'Fri 2026-10-30 03:42:34 UTC' >"$fx/unit.ExecMainExitTimestamp"
	for f in pre-sprint1-20261023-2114.dump pre-go1273-20261023-2217.dump pre-r283-20261026-2235.dump \
		pre-old-20261023-1159.dump pre-edge-20261023-1200.dump qr-20261020T034217Z.dump pre-nodate.dump pre-bad-20261399-0000.dump notes.txt; do
		echo x >"$fx/backups/$f"
	done
	# The mtime says nothing: an old file named as recent is kept.
	touch -d '2020-01-01' "$fx/backups/pre-r283-20261026-2235.dump"
}
left() { (cd "$fx/backups" && printf '%s ' *); }
fails=0
ok() { echo "ok   $*"; }
bad() {
	echo "FAIL $*"
	fails=1
}
run() { bash "$here/prune-predeploy-dumps.sh" "$@" >"$fx/out" 2>&1; }

all="notes.txt pre-bad-20261399-0000.dump pre-edge-20261023-1200.dump pre-go1273-20261023-2217.dump pre-nodate.dump pre-old-20261023-1159.dump pre-r283-20261026-2235.dump pre-sprint1-20261023-2114.dump qr-20261020T034217Z.dump "
setup
run --dry-run && ok "dry run exits 0" || bad "dry run failed: $(cat "$fx/out")"
[[ $(left) == "$all" ]] && ok "dry run deletes nothing" || bad "dry run deleted: $(left)"
grep -qx 'delete pre-old-20261023-1159.dump (7.0 days old)' "$fx/out" && ok "lists 7 days and 1 minute as to delete" || bad "$(grep old "$fx/out")"
grep -qx 'keep   pre-edge-20261023-1200.dump (7.0 days old)' "$fx/out" && ok "exactly 7 days is kept (more than N days)" || bad "$(grep edge "$fx/out")"
grep -qx 'keep   pre-sprint1-20261023-2114.dump (6.6 days old)' "$fx/out" && ok "keeps younger than 7 days" || bad "$(grep sprint1 "$fx/out")"
grep -q 'would delete 1 file(s) (production healthy' "$fx/out" && ok "dry run says what a real run would do" || bad "$(tail -1 "$fx/out")"

setup
run && ok "real run exits 0" || bad "real run failed: $(cat "$fx/out")"
[[ $(left) == "${all/pre-old-20261023-1159.dump /}" ]] && ok "deletes only pre-*.dump older than 7 days (by name, not mtime)" || bad "left: $(left)"
[[ $(grep -c '^==> ls -l' "$fx/out") == 2 ]] && ok "ls -l before and after" || bad "ls -l: $(grep -c '^==> ls -l' "$fx/out")"
grep -q 'skip   pre-nodate.dump' "$fx/out" && grep -q 'skip   pre-bad-20261399-0000.dump (invalid date)' "$fx/out" && ok "skips names without a valid date" || bad "skips"

setup
run --days 4 && [[ $(left) == "notes.txt pre-bad-20261399-0000.dump pre-nodate.dump pre-r283-20261026-2235.dump qr-20261020T034217Z.dump " ]] &&
	ok "--days 4" || bad "--days 4 left: $(left)"

refuse() { # $1 description, then options; the fixture is already broken
	if run "${@:2}"; then bad "$1: deleted anyway"; else
		grep -q 'refusing to delete' "$fx/out" && [[ $(left) == "$all" ]] && ok "$1: refuses, nothing deleted" || bad "$1: $(tail -2 "$fx/out")"
	fi
}
setup && echo '{"status":"degraded"}' >"$fx/health" && refuse "health not ok"
setup && rm "$fx/health" && refuse "health does not answer"
setup && echo failed >"$fx/unit.Result" && refuse "last backup failed"
setup && echo 1 >"$fx/unit.ExecMainStatus" && refuse "last backup exit status 1"
setup && echo 'Wed 2026-10-28 03:42:34 UTC' >"$fx/unit.ExecMainExitTimestamp" && refuse "last backup older than 26 h"
# With the real clock: date -d "" would be today's midnight, less than 26 h ago.
setup && echo '' >"$fx/unit.ExecMainExitTimestamp" && QR_NOW=$(($(date -u +%s) + 60)) refuse "backup never ran" --days 1
setup && echo '{"status":"degraded"}' >"$fx/health" && run --dry-run && grep -q 'a real run would refuse' "$fx/out" &&
	ok "dry run warns that a real run would refuse" || bad "dry run with failed checks: $(tail -1 "$fx/out")"

setup
rm "$fx"/backups/pre-old-*
run && grep -qx '==> nothing older than 7 days' "$fx/out" && ok "nothing to do" || bad "nothing to do: $(tail -1 "$fx/out")"
for a in "--days 0" "--days x" "--days" "--force"; do
	# shellcheck disable=SC2086 # word splitting wanted
	if run $a; then bad "'$a' accepted"; else ok "'$a' refused"; fi
done
exit $fails
