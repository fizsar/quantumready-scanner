#!/usr/bin/env bash
# Tests deploy/pin-images.sh (the part of deploy.sh that runs on the VPS)
# with a fake docker: no registry, no VPS.
# shellcheck disable=SC2015 # ok/FAIL reporting never fails
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

A=sha256:$(printf 'a%.0s' {1..64})
W=sha256:$(printf 'b%.0s' {1..64})
X=sha256:$(printf 'c%.0s' {1..64})
API=ghcr.io/fizsar/quantumready-saas-api
WEB=ghcr.io/fizsar/quantumready-saas-web

# fake docker: "pull REF" succeeds; "image inspect -f ... REF" prints the
# repo digest the tag resolves to, from $tmp/registry (one "REF DIGEST" a line).
cat >"$tmp/docker" <<'EOF'
#!/usr/bin/env bash
case "$1" in
pull) echo "pulled $2" >>"$FAKE_DIR/log" ;;
image)
	ref=${*: -1}
	d=$(awk -v r="$ref" '$1 == r {print $2}' "$FAKE_DIR/registry")
	[ -n "$d" ] || exit 1
	echo "${ref%:*}@$d" ;;
*) exit 2 ;;
esac
EOF
chmod +x "$tmp/docker"
export FAKE_DIR=$tmp DOCKER=$tmp/docker

fails=0
run() { # run EXPECT DESCRIPTION TAG API_REF WEB_REF ALLOW_UNRECORDED
	local expect=$1 d=$2 got
	printf 'QR_TAG=old\nOTHER=1\n' >"$tmp/env"
	: >"$tmp/log"
	# shellcheck source=deploy/pin-images.sh
	if (. "$here/pin-images.sh" && pin_images "$tmp/env" "$3" "$4" "$5" "$6") >"$tmp/out" 2>&1; then got=pass; else got=fail; fi
	if [[ $got == "$expect" ]]; then echo "ok   $d"; else echo "FAIL $d (want $expect, got $got)"; cat "$tmp/out"; fails=1; fi
}
env_has() { grep -qx "$1" "$tmp/env" && echo "ok   .env has $1" || { echo "FAIL .env lacks $1"; cat "$tmp/env"; fails=1; }; }

printf '%s %s\n%s %s\n' "$API:sha-1234567" "$A" "$WEB:sha-1234567" "$W" >"$tmp/registry"

run pass "recorded digests that the tag still resolves to" sha-1234567 "$API@$A" "$WEB@$W" 0
env_has "QR_API_IMAGE=$API@$A"
env_has "QR_WEB_IMAGE=$WEB@$W"
env_has "OTHER=1"

run fail "the tag moved after the Release recorded it (re-pushed)" sha-1234567 "$API@$X" "$WEB@$W" 0
grep -q "QR_API_IMAGE" "$tmp/env" && { echo "FAIL .env changed on a refusal"; fails=1; } || echo "ok   .env untouched on a refusal"

run fail "a reference that is not our image by digest" sha-1234567 "$API:sha-1234567" "$WEB@$W" 0
run fail "another registry" sha-1234567 "docker.io/evil/api@$A" "$WEB@$W" 0
run fail "no recorded digest and no permission to pin one" sha-1234567 "" "" 0

run pass "no recorded digest (older release): resolved once and pinned" sha-1234567 "" "" 1
env_has "QR_API_IMAGE=$API@$A"
env_has "QR_WEB_IMAGE=$WEB@$W"
grep -q "not recorded by the Release" "$tmp/out" && echo "ok   the pin of an unrecorded tag is announced" || { echo "FAIL no warning"; fails=1; }

run fail "unrecorded and the tag does not exist" sha-7654321 "" "" 1
exit $fails
