#!/usr/bin/env bash
# Deploy a QuantumReady release to the VPS.
#
#   deploy/deploy.sh sha-14226c7   # a specific build (preferred)
#   deploy/deploy.sh latest        # whatever main last published
#
# Copies deploy/compose.prod.yaml *from that commit* to /opt/quantumready,
# sets QR_TAG in the VPS .env, pulls, starts (migrations run first) and checks
# health locally on the VPS and from outside. Rollback = run it again with the
# previous tag, which is printed at the start of every deploy.
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
if [[ $TAG == latest ]]; then
	git fetch -q origin main
	ref=origin/main
else
	ref=${TAG#sha-}
	git cat-file -e "$ref^{commit}" 2>/dev/null || git fetch -q origin main
fi

echo "==> $HOST: deploying $TAG (compose from $(git rev-parse --short "$ref"))"
# shellcheck disable=SC2029 # $DIR is meant to expand locally.
git show "$ref:deploy/compose.prod.yaml" | ssh "$HOST" "cat > $DIR/compose.yaml.new"

ssh "$HOST" bash -s -- "$TAG" <<'REMOTE'
set -euo pipefail
TAG=$1
cd /opt/quantumready
prev=$(sed -n 's/^QR_TAG=//p' .env)
echo "==> previous tag: ${prev:-none} (rollback: deploy/deploy.sh ${prev:-<tag>})"
mv compose.yaml.new compose.yaml
if grep -q '^QR_TAG=' .env; then
	sed -i "s/^QR_TAG=.*/QR_TAG=$TAG/" .env
else
	echo "QR_TAG=$TAG" >> .env
fi
docker compose pull --quiet
docker compose up -d --wait --remove-orphans
docker compose ps --format 'table {{.Service}}\t{{.Image}}\t{{.Status}}'
# Assign first: a failing curl inside "echo $(...)" would not stop the script.
api=$(curl -fsS --max-time 5 http://127.0.0.1:8080/api/health)
echo "==> api (127.0.0.1:8080): $api"
curl -fsS -o /dev/null --max-time 5 http://127.0.0.1:8081/
echo "==> web (127.0.0.1:8081): ok"
REMOTE

public=$(curl -fsS --max-time 10 "$URL/api/health")
echo "==> public ($URL): $public"
echo "==> done: $TAG"
