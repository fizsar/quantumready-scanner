#!/usr/bin/env bash
# Deploy a QuantumReady release to the VPS.
#
#   deploy/deploy.sh sha-14226c7   # a specific build (preferred)
#   deploy/deploy.sh latest        # origin/main's newest commit, by its sha- tag
#
# Only commits of origin/main can be deployed: the branch whose CI-tested
# builds release.yml publishes. (Until 16/11/2026, origin/showcase too: the
# branch production came from before main.) QR_DEPLOY_DRY_RUN=1 stops after that check, without SSH
# (deploy/test-deploy-ref.sh).
#
# Images by digest (security review of 01/11/2026): release.yml records the
# digest of each image it pushed, after the Trivy gate, as a status of the
# commit (release/api, release/web). This script reads them with gh and the
# VPS runs exactly those (QR_API_IMAGE, QR_WEB_IMAGE = image@sha256:...;
# deploy/pin-images.sh), after checking that the tag still resolves to them.
# A release from before that has no recorded digest: QR_DEPLOY_PIN_UNRECORDED=1
# resolves its tag once on the VPS and pins that.
#
# Copies deploy/compose.prod.yaml *from that commit* to /opt/quantumready,
# sets QR_TAG and the pinned images in the VPS .env, pulls, starts (migrations
# run first) and checks health locally on the VPS and from outside. Rollback =
# run it again with the previous tag, which is printed at the start of every
# deploy. A deploy that succeeds appends its tag to deploy/deployments.txt
# (commit it).
#
# Before anything changes it prints which services compose will create or
# recreate (deploy/compose-plan.sh) and stops if that includes the database,
# unless QR_ALLOW_DB_RECREATE=1 (a deliberate PostgreSQL change, after a
# backup and a dump). On 23/10/2026 a new image reference text restarted it.
set -euo pipefail

TAG=${1:-}
HOST=${QR_DEPLOY_HOST:-fizsar-vps}
DIR=/opt/quantumready
URL=${QR_DEPLOY_URL:-https://quantumready.fizsar.com}

if [[ ! $TAG =~ ^(sha-[0-9a-f]{7,40}|latest)$ ]]; then
	echo "usage: $0 <sha-xxxxxxx|latest>" >&2
	exit 2
fi

cd "$(git rev-parse --show-toplevel)"
git fetch -q origin
if [[ $TAG == latest ]]; then
	ref=origin/main
else
	ref=${TAG#sha-}
	if ! git cat-file -e "$ref^{commit}" 2>/dev/null; then
		echo "refusing to deploy $TAG: commit $ref not found (not pushed, or a typo)" >&2
		exit 1
	fi
fi
commit=$(git rev-parse "$ref^{commit}")
on_branch() { git merge-base --is-ancestor "$commit" "origin/$1" 2>/dev/null; }
if ! on_branch main; then
	echo "refusing to deploy $TAG: commit ${commit:0:7} is not on origin/main;" >&2
	echo "only main's builds are published (release.yml). Merge it into main first." >&2
	exit 1
fi
# latest is origin/main's newest commit, deployed by its own sha- tag (the
# one its Release recorded), never by the moving :latest.
if [[ $TAG == latest ]]; then
	TAG=sha-$(git rev-parse --short=7 "$commit")
fi

# The digests the Release recorded for this commit: "api REF" and "web REF"
# lines (REF = image@sha256:...). Only the newest status of each context
# counts, and only if it is a success with a well-formed digest.
release_refs() {
	"${QR_GH:-gh}" api "repos/${QR_REPO:-fizsar/quantumready-saas}/commits/$1/statuses?per_page=100" </dev/null |
		python3 -c '
import json, re, sys
seen = set()
for s in json.load(sys.stdin):
    c = s.get("context", "")
    if c not in ("release/api", "release/web") or c in seen:
        continue
    seen.add(c)
    d = s.get("description") or ""
    if s.get("state") == "success" and re.fullmatch(r"sha256:[0-9a-f]{64}", d):
        name = c.split("/")[1]
        print(name, "ghcr.io/fizsar/quantumready-saas-" + name + "@" + d)
'
}
refs=$(release_refs "$commit")
api_ref=$(sed -n 's/^api //p' <<<"$refs")
web_ref=$(sed -n 's/^web //p' <<<"$refs")
pin_unrecorded=${QR_DEPLOY_PIN_UNRECORDED:-0}
for name in api web; do
	r=${name}_ref
	if [ -z "${!r}" ]; then
		if [ "$pin_unrecorded" != 1 ]; then
			echo "refusing to deploy $TAG: the Release recorded no digest for $name (status release/$name of ${commit:0:7})." >&2
			echo "Releases from before 01/11/2026 have none: QR_DEPLOY_PIN_UNRECORDED=1 resolves the tag once on the VPS and pins it." >&2
			exit 1
		fi
		echo "==> $name: no digest recorded by the Release; resolved on the VPS and pinned (QR_DEPLOY_PIN_UNRECORDED=1)"
	else
		echo "==> $name ${!r}"
	fi
done
echo "==> tag $TAG"

if [[ ${QR_DEPLOY_DRY_RUN:-} == 1 ]]; then
	echo "==> dry run: $TAG (${commit:0:7}) can be deployed"
	exit 0
fi

echo "==> $HOST: deploying $TAG (compose from $(git rev-parse --short "$ref"))"
# shellcheck disable=SC2029 # $DIR is meant to expand locally.
git show "$ref:deploy/compose.prod.yaml" | ssh "$HOST" "cat > $DIR/compose.yaml.new"

# The remote script: the plan functions, then the deploy itself.
{
	cat deploy/compose-plan.sh deploy/pin-images.sh
	cat <<'REMOTE'
set -euo pipefail
TAG=$1 ALLOW_DB=$2 API_REF=$3 WEB_REF=$4 PIN_UNRECORDED=$5
cd /opt/quantumready
prev=$(sed -n 's/^QR_TAG=//p' .env)
echo "==> previous tag: ${prev:-none} (rollback: deploy/deploy.sh ${prev:-<tag>})"
set_env() { # set_env FILE KEY VALUE: replace or append
	if grep -q "^$2=" "$1"; then
		sed -i "s|^$2=.*|$2=$3|" "$1"
	else
		echo "$2=$3" >> "$1"
	fi
}
# The next .env next to compose.yaml.new: nothing in use changes until the
# plan is accepted.
cp -p .env .env.next
set_env .env.next QR_TAG "$TAG"
# The API believes X-Forwarded-For only from Caddy's network.
proxies=$(docker network inspect web -f '{{range .IPAM.Config}}{{.Subnet}},{{end}}' </dev/null)
proxies=${proxies%,} # no trailing comma
[ -n "$proxies" ] || { rm -f .env.next; echo "cannot read the subnet of the web network" >&2; exit 1; }
set_env .env.next QR_TRUSTED_PROXIES "$proxies"
# The images by digest: what the Release scanned and recorded.
if ! pin_images .env.next "$TAG" "$API_REF" "$WEB_REF" "$PIN_UNRECORDED"; then
	rm -f compose.yaml.new .env.next
	exit 1
fi
plan=$(recreate_plan compose.yaml.new .env.next)
echo "==> compose will create or recreate: $(echo "${plan:-nothing}" | paste -sd ' ' -)"
if ! db_guard "$plan" "$ALLOW_DB"; then
	rm -f compose.yaml.new .env.next
	exit 1
fi
mv compose.yaml.new compose.yaml
mv .env.next .env
docker compose pull --quiet
docker compose up -d --wait --remove-orphans
docker compose ps --format 'table {{.Service}}\t{{.Image}}\t{{.Status}}'
# Assign first: a failing curl inside "echo $(...)" would not stop the script.
api=$(curl -fsS --max-time 5 http://127.0.0.1:8080/api/health)
echo "==> api (127.0.0.1:8080): $api"
curl -fsS -o /dev/null --max-time 5 http://127.0.0.1:8081/
echo "==> web (127.0.0.1:8081): ok"
REMOTE
} | ssh "$HOST" bash -s -- "$TAG" "${QR_ALLOW_DB_RECREATE:-0}" "$api_ref" "$web_ref" "$pin_unrecorded"

public=$(curl -fsS --max-time 10 "$URL/api/health")
echo "==> public ($URL): $public"
# The record deploy/ghcr_cleanup.py keeps images by: the commit's tag, also
# for latest.
printf '%s sha-%s\n' "$(date -u +%Y-%m-%dT%H:%MZ)" "${commit:0:7}" >> deploy/deployments.txt
echo "==> done: $TAG (recorded in deploy/deployments.txt: commit it)"
