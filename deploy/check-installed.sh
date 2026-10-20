#!/usr/bin/env bash
# Compares what is installed on the VPS by hand (the backup scripts in
# /opt/quantumready/bin and the qr-* systemd units) with deploy/backup at a
# commit. Read-only: one ssh call that tars those files to stdout.
#
#   deploy/check-installed.sh 56a3ed0
#
# Exit 0 when identical: same content, owner root:root, mode 755 for scripts
# and 644 for units, nothing missing and no other *.sh or qr-* files. Exit 1
# otherwise, with a diff of each differing file. Use it before and after
# reinstalling, and compare with what is installed, not only with the
# previous deploy (M1: the VPS had main's scripts, showcase's lacked a fix).
#
# Test hooks (deploy/test-check-installed.sh): QR_REMOTE_SHELL (command that
# runs a script from stdin, default "ssh $QR_DEPLOY_HOST bash -s --"),
# QR_INSTALL_ROOT (prefix of the installed paths), QR_EXPECT_OWNER.
set -euo pipefail

REV=${1:-}
[[ -n $REV ]] || { echo "usage: $0 <commit>" >&2; exit 2; }
HOST=${QR_DEPLOY_HOST:-fizsar-vps}
ROOT=${QR_INSTALL_ROOT:-}
OWNER=${QR_EXPECT_OWNER:-root/root}
read -ra remote <<<"${QR_REMOTE_SHELL:-ssh $HOST bash -s --}"

cd "$(git rev-parse --show-toplevel)"
if ! commit=$(git rev-parse --verify -q "$REV^{commit}"); then
	echo "unknown commit: $REV" >&2
	exit 2
fi

# Expected: "mode installed-path repo-path", installed paths relative to /.
expected() {
	local f
	for f in common.sh backup.sh restore-test.sh; do
		echo "-rwxr-xr-x opt/quantumready/bin/$f deploy/backup/$f"
	done
	git ls-tree --name-only "$commit" deploy/backup/systemd/ | grep -E '/qr-[^/]+\.(service|timer)$' |
		while read -r f; do echo "-rw-r--r-- etc/systemd/system/${f##*/} $f"; done
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
"${remote[@]}" "$ROOT" >"$tmp/installed.tar" <<'REMOTE'
set -euo pipefail
cd "${1:-}/"
shopt -s nullglob
files=(opt/quantumready/bin/*.sh etc/systemd/system/qr-*)
tar -cf - --no-recursion "${files[@]}" </dev/null
REMOTE
mkdir "$tmp/root"
tar -tvf "$tmp/installed.tar" | awk '{print $1, $2, $NF}' >"$tmp/listing"
tar -xf "$tmp/installed.tar" -C "$tmp/root" --no-same-owner

bad=0
fail() {
	echo "DIFF $*"
	bad=$((bad + 1))
}
while read -r mode path src; do
	got=$(awk -v p="$path" '$3 == p {print $1, $2}' "$tmp/listing")
	if [[ -z $got ]]; then
		fail "/$path: missing"
		continue
	fi
	ok=1
	[[ ${got% *} == "$mode" ]] || { fail "/$path: mode ${got% *}, want $mode"; ok=0; }
	[[ ${got#* } == "$OWNER" ]] || { fail "/$path: owner ${got#* }, want $OWNER"; ok=0; }
	if ! git show "$commit:$src" | diff -u --label "${commit:0:7}:$src" --label "installed:/$path" - "$tmp/root/$path"; then
		fail "/$path: content differs from ${commit:0:7}:$src"
		ok=0
	fi
	[[ $ok == 0 ]] || echo "ok   /$path"
done < <(expected)
while read -r _ _ path; do
	expected | awk -v p="$path" '$2 == p {found=1} END {exit !found}' || fail "/$path: installed but not in ${commit:0:7}"
done <"$tmp/listing"

if [[ $bad == 0 ]]; then
	echo "==> installed files are identical to ${commit:0:7}"
else
	echo "==> installed files differ from ${commit:0:7}: $bad difference(s)"
	exit 1
fi
