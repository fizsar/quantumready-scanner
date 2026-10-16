#!/usr/bin/env bash
# Tests which commits deploy.sh accepts: deploy/test-deploy-ref.sh. Dry run: no SSH,
# nothing leaves this machine. Builds a throwaway repository with an
# "origin" that has main, showcase and another branch.
set -euo pipefail
script=$(cd "$(dirname "$0")" && pwd)/deploy.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

git init -q --bare "$tmp/origin.git"
git init -q "$tmp/work"
cd "$tmp/work"
git config user.email t@example.com
git config user.name test
commit() { git commit -q --allow-empty -m "$1"; git rev-parse --short=7 HEAD; }
base=$(commit base)
git branch -M main
onmain=$(commit on-main)
git checkout -q -b showcase "$base"
onshowcase=$(commit on-showcase)
git checkout -q -b other "$base"
onother=$(commit on-other)
git checkout -q --detach "$base"
local_only=$(commit never-pushed)
git remote add origin "$tmp/origin.git"
git push -q origin main showcase other
mkdir -p deploy && cp "$script" deploy/deploy.sh

fails=0
check() { # check WANT TAG
	if QR_DEPLOY_DRY_RUN=1 bash deploy/deploy.sh "$2" >"$tmp/out" 2>&1; then got=accept; else got=refuse; fi
	if [[ $got == "$1" ]]; then echo "ok   $1 $2"; else echo "FAIL want $1, got $got: $2"; cat "$tmp/out"; fails=1; fi
}
check accept "sha-$onmain"
check accept "sha-$base"        # on both branches
check accept "sha-$onshowcase"
check accept latest
check refuse "sha-$onother"     # pushed, but to another branch
check refuse "sha-$local_only"  # never pushed
check refuse "sha-0000000"      # unknown commit
check refuse "v1.0"             # not a tag deploy.sh knows
grep -q "is not on origin/main nor origin/showcase" <(QR_DEPLOY_DRY_RUN=1 bash deploy/deploy.sh "sha-$onother" 2>&1 || true) \
	|| { echo "FAIL: the refusal does not explain why"; fails=1; }
exit $fails
