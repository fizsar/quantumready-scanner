#!/usr/bin/env bash
# Tests deploy/check-installed.sh without the VPS: the "remote" is bash on this
# machine over a fake root that holds HEAD's deploy/backup as installed.
# shellcheck disable=SC2015 # ok/FAIL reporting never fails
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
root=$tmp/root
bin=$root/opt/quantumready/bin units=$root/etc/systemd/system

install_head() {
	rm -rf "$root"
	mkdir -p "$bin" "$units"
	for f in common.sh backup.sh restore-test.sh; do git show "HEAD:deploy/backup/$f" >"$bin/$f"; done
	for u in $(git ls-tree --name-only HEAD deploy/backup/systemd/ | grep -E '/qr-'); do git show "HEAD:$u" >"$units/${u##*/}"; done
	chmod 755 "$bin"/*.sh
	chmod 644 "$units"/qr-*
	echo unrelated >"$units/other.service" # not ours: ignored
}

owner="$(id -un)/$(id -gn)"
export QR_REMOTE_SHELL="bash -s --" QR_INSTALL_ROOT=$root QR_EXPECT_OWNER=$owner
fails=0
check() { # check WANT(pass|fail) DESCRIPTION [GREP]
	local want=$1 d=$2 pattern=${3:-}
	if bash deploy/check-installed.sh HEAD >"$tmp/out" 2>&1; then got=pass; else got=fail; fi
	if [[ $got == "$want" ]] && { [[ -z $pattern ]] || grep -qE -- "$pattern" "$tmp/out"; }; then echo "ok   $d"; else
		echo "FAIL $d (want $want, got $got${pattern:+, output must match $pattern})"
		cat "$tmp/out"
		fails=1
	fi
}

install_head
before=$(cd "$root" && find . -type f -exec sha256sum {} + | sort)
check pass "identical install" "identical to"
after=$(cd "$root" && find . -type f -exec sha256sum {} + | sort)
[[ $before == "$after" ]] && echo "ok   read-only: nothing changed on the 'VPS'" || { echo "FAIL the check changed installed files"; fails=1; }

echo "echo patched by hand" >>"$bin/backup.sh"
check fail "a changed script, with its diff" '^\+echo patched by hand$'
install_head
sed -i 's/03:30/04:30/' "$units/qr-backup.timer"
check fail "a changed unit" 'qr-backup.timer: content differs'
install_head
chmod 644 "$bin/restore-test.sh"
check fail "a script without x" 'restore-test.sh: mode -rw-r--r--, want -rwxr-xr-x'
install_head
rm "$units/qr-restore-test.service"
check fail "a missing unit" 'qr-restore-test.service: missing'
install_head
echo old >"$bin/old-backup.sh"
chmod 755 "$bin/old-backup.sh"
check fail "a leftover script" 'old-backup.sh: installed but not in'
install_head
QR_EXPECT_OWNER=root/root check fail "files not owned by root" 'owner .*, want root/root'

if bash deploy/check-installed.sh 0000000 >/dev/null 2>&1; then echo "FAIL unknown commit accepted"; fails=1; else echo "ok   unknown commit refused"; fi
exit $fails
