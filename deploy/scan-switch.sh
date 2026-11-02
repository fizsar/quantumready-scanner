#!/usr/bin/env bash
# Open or close public scanning on the VPS without redeploying images:
#
#   deploy/scan-switch.sh close   # PUBLIC_SCAN_OPEN=false: back to the allowlist (SCAN_ALLOWLIST)
#   deploy/scan-switch.sh open    # PUBLIC_SCAN_OPEN=true: any domain (needs ALTCHA_HMAC_KEY)
#   QR_SWITCH_DRY_RUN=1 deploy/scan-switch.sh close   # only the plan; nothing changes
#
# Sets PUBLIC_SCAN_OPEN in /opt/quantumready/.env and recreates the api only
# (docker compose up -d --no-deps --pull never --wait api): the same image,
# no pull, no other service, never db. Before anything starts it checks with
# recreate_plan (deploy/compose-plan.sh, as deploy.sh does) that compose would
# recreate exactly the api; otherwise it puts the .env back and stops without
# changing anything. Afterwards GET /api/targets must say "open": true|false,
# on the VPS (127.0.0.1:8080) and from outside; if the api does not come up
# healthy, the .env goes back and the api is recreated as it was. About 15 s.
#
# Test hooks (deploy/test-scan-switch.sh): QR_REMOTE_SHELL (runs a script from
# stdin; default "ssh $QR_DEPLOY_HOST bash -s --"), QR_DIR, QR_SWITCH_API (the
# API on the VPS; empty skips its check) and QR_DEPLOY_URL (empty skips the
# public check).
set -euo pipefail

case ${1:-} in
open) want=true ;;
close) want=false ;;
*)
	echo "usage: $0 open|close" >&2
	exit 2
	;;
esac
here=$(cd "$(dirname "$0")" && pwd)
HOST=${QR_DEPLOY_HOST:-fizsar-vps}
URL=${QR_DEPLOY_URL-https://quantumready.fizsar.com}
read -ra remote <<<"${QR_REMOTE_SHELL:-ssh $HOST bash -s --}"
start=$(date +%s)

{
	cat "$here/compose-plan.sh"
	cat <<'REMOTE'
set -euo pipefail
DIR=$1 WANT=$2 DRY=$3 API=$4
cd "$DIR"
set_env() { # set_env FILE KEY VALUE: replace or append
	if grep -q "^$2=" "$1"; then sed -i "s|^$2=.*|$2=$3|" "$1"; else echo "$2=$3" >>"$1"; fi
}
targets_open() { curl -fsS --max-time 5 "$API/api/targets" </dev/null | grep -o '"open":[a-z]*' | cut -d: -f2; }
cur=$(sed -n 's/^PUBLIC_SCAN_OPEN=//p' .env)
cur=${cur:-false}
echo "==> PUBLIC_SCAN_OPEN: $cur -> $WANT"
if [ "$WANT" = true ] && ! grep -qE '^ALTCHA_HMAC_KEY=[0-9a-f]{32,}$' .env; then
	echo "refusing to open: ALTCHA_HMAC_KEY is not set in .env (the proof of work of open scanning)" >&2
	exit 1
fi
cp -p .env .env.switch # the way back, with the same mode (600)
restore() { mv .env.switch .env; }
set_env .env PUBLIC_SCAN_OPEN "$WANT"
plan=$(recreate_plan compose.yaml .env </dev/null | sort | paste -sd ' ' -)
echo "==> compose would create or recreate: ${plan:-nothing}"
if [ "$cur" = "$WANT" ] && [ -z "$plan" ]; then
	rm -f .env.switch
	echo "==> already $WANT; nothing to recreate"
elif [ "$plan" != "api recreate" ]; then
	restore
	echo "refusing: the plan must be exactly 'api recreate' (something else differs from what runs); .env put back" >&2
	exit 1
elif [ "$DRY" = 1 ]; then
	restore
	echo "==> dry run: .env put back, nothing recreated"
	exit 0
elif ! docker compose up -d --no-deps --pull never --wait api </dev/null >/dev/null 2>.switch.err; then
	cat .switch.err >&2
	restore
	echo "the api did not come up healthy: .env put back, recreating the api as it was" >&2
	docker compose up -d --no-deps --pull never --wait api </dev/null >/dev/null 2>&1 || true
	rm -f .switch.err
	exit 1
else
	rm -f .env.switch .switch.err
fi
if [ -n "$API" ]; then
	got=$(targets_open || true)
	[ "$got" = "$WANT" ] || { echo "GET $API/api/targets says open=${got:-?}, want $WANT" >&2; exit 1; }
	echo "==> api (VPS): open=$got"
fi
REMOTE
} | "${remote[@]}" "${QR_DIR:-/opt/quantumready}" "$want" "${QR_SWITCH_DRY_RUN:-0}" "${QR_SWITCH_API-http://127.0.0.1:8080}"

[ "${QR_SWITCH_DRY_RUN:-0}" = 1 ] && exit 0
if [ -n "$URL" ]; then
	got=$(curl -fsS --max-time 10 "$URL/api/targets" | grep -o '"open":[a-z]*' | cut -d: -f2 || true)
	[ "$got" = "$want" ] || {
		echo "GET $URL/api/targets says open=${got:-?}, want $want" >&2
		exit 1
	}
	echo "==> public ($URL): open=$got"
fi
echo "==> done in $(($(date +%s) - start)) s: public scanning is $([ "$want" = true ] && echo OPEN || echo closed)"
