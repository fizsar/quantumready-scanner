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

# deploy.sh reads the digests the Release recorded as statuses of the commit (release/api,
# release/web; newest first, as GitHub lists them). A fake gh answers from
# FAKE_STATUSES; the default has both images, plus an older api status.
A=sha256:$(printf 'a%.0s' {1..64})
W=sha256:$(printf 'b%.0s' {1..64})
C=sha256:$(printf 'c%.0s' {1..64})
mkdir -p "$tmp/bin"
cat >"$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
echo "$*" >>"$FAKE_GH_LOG"
case $FAKE_STATUSES in
none) echo '[]' ;;
failed) printf '[{"context":"release/api","state":"failure","description":"%s"},{"context":"release/web","state":"success","description":"%s"}]\n' "$A" "$W" ;;
bad) printf '[{"context":"release/api","state":"success","description":"latest"},{"context":"release/web","state":"success","description":"%s"}]\n' "$W" ;;
*) printf '[{"context":"release/api","state":"success","description":"%s"},{"context":"ci","state":"success","description":"x"},{"context":"release/web","state":"success","description":"%s"},{"context":"release/api","state":"success","description":"%s"}]\n' "$A" "$W" "$C" ;;
esac
EOF
chmod +x "$tmp/bin/gh"
export QR_GH=$tmp/bin/gh FAKE_GH_LOG=$tmp/gh.log A W C

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

digests() { # digests WANT DESCRIPTION STATUSES TAG [PATTERN] [PIN_UNRECORDED]
	local want=$1 d=$2 got
	if FAKE_STATUSES=$3 QR_DEPLOY_PIN_UNRECORDED=${6:-} QR_DEPLOY_DRY_RUN=1 bash deploy/deploy.sh "$4" >"$tmp/out" 2>&1; then got=accept; else got=refuse; fi
	if [[ $got == "$want" ]] && { [[ -z ${5:-} ]] || grep -qE -- "$5" "$tmp/out"; }; then echo "ok   digests: $d"; else
		echo "FAIL digests: $d (want $want, got $got${5:+, output must match $5})"; cat "$tmp/out"; fails=1
	fi
}
digests accept "the newest recorded digest of each image" ok "sha-$onmain" "api ghcr.io/fizsar/quantumready-saas-api@$A"
digests accept "web too" ok "sha-$onmain" "web ghcr.io/fizsar/quantumready-saas-web@$W"
if grep -q "$C" "$tmp/out"; then echo "FAIL an older status was used"; fails=1; else echo "ok   digests: older statuses ignored"; fi
if grep -q "commits/$(git rev-parse "$onmain")/statuses" "$tmp/gh.log"; then echo "ok   digests: asked for the full commit"; else
	echo "FAIL gh call"; cat "$tmp/gh.log"; fails=1
fi
digests accept "latest deploys origin/main's own commit, by its sha- tag" ok latest "tag sha-$onmain"
digests refuse "no recorded digest" none "sha-$onmain" "recorded no digest"
digests accept "no recorded digest, pinned on the VPS when allowed" none "sha-$onmain" "resolved on the VPS" 1
digests refuse "a failed status is no digest" failed "sha-$onmain"
digests refuse "a malformed digest" bad "sha-$onmain"
exit $fails
