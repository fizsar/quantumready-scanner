#!/usr/bin/env bash
# Deploy a QuantumReady release to the VPS.
#
#   deploy/deploy.sh sha-14226c7   # a specific build (preferred)
#   deploy/deploy.sh latest        # whatever main last published
#
# Only commits of origin/main or origin/showcase can be deployed: those are
# the branches whose CI-tested builds release.yml publishes (showcase never
# moves :latest). QR_DEPLOY_DRY_RUN=1 stops after that check, without SSH
# (deploy/test-deploy-ref.sh).
#
# Copies deploy/compose.prod.yaml *from that commit* to /opt/quantumready,
# sets QR_TAG in the VPS .env, pulls, starts (migrations run first) and checks
# health locally on the VPS and from outside. Rollback = run it again with the
# previous tag, which is printed at the start of every deploy.
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
if ! on_branch main && ! on_branch showcase; then
	echo "refusing to deploy $TAG: commit ${commit:0:7} is not on origin/main nor origin/showcase;" >&2
	echo "only those branches' builds are published (release.yml). Push it to one of them first." >&2
	exit 1
fi
if [[ ${QR_DEPLOY_DRY_RUN:-} == 1 ]]; then
	echo "==> dry run: $TAG (${commit:0:7}) can be deployed"
	exit 0
fi

echo "==> $HOST: deploying $TAG (compose from $(git rev-parse --short "$ref"))"
# shellcheck disable=SC2029 # $DIR is meant to expand locally.
git show "$ref:deploy/compose.prod.yaml" | ssh "$HOST" "cat > $DIR/compose.yaml.new"

# The remote script: the plan functions, then the deploy itself.
{
	cat deploy/compose-plan.sh
	cat <<'REMOTE'
set -euo pipefail
TAG=$1 ALLOW_DB=$2
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
} | ssh "$HOST" bash -s -- "$TAG" "${QR_ALLOW_DB_RECREATE:-0}"

public=$(curl -fsS --max-time 10 "$URL/api/health")
echo "==> public ($URL): $public"
echo "==> done: $TAG"
