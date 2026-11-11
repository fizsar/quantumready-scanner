#!/usr/bin/env bash
# TLS test lab (milestone 2.7.2). Deliberately vulnerable servers on an
# internal Docker network, with no published ports; never probe anything else.
#
#   lab.sh up        build, start and check isolation (make tlslab-up)
#   lab.sh check     check isolation again
#   lab.sh testssl [SERVER…]   reference run of testssl.sh against the lab servers (default: all)
#   lab.sh down      remove everything and check nothing is left (make tlslab-down)
#   lab.sh gotest    integration tests of the scanner against the lab (make tlslab-test)
#   lab.sh scan ARGS…        cmd/qrscan -lab ARGS… inside the lab (make tlslab-scan ARGS="-facts tls13")
#   lab.sh run IMAGE ARGS…   run a client container on the lab network
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
PROJECT=qr-tlslab
NET=qr-tlslab
RUNNER_LABEL=qr-tlslab=runner
TESTSSL_IMAGE=ghcr.io/testssl/testssl.sh:3.2@sha256:51918b1e954df0acee8c08d3ad5ce306710e4fefe49ed0e0f92728ecf9e7ff34
PROBE_IMAGE=qr-tlslab/openssl35:3.5.9
GO_IMAGE=golang:1.27.4@sha256:08bcb99a38f85a265f9a93d1d3323e2440eec333380f3ea26ee0dec9264bb41c

compose() { docker compose -f "$here/compose.yaml" "$@"; }
# Listening sockets on the host: "proto local-address:port".
listeners() { ss -Hltun | awk '{print $1, $5}' | sort -u; }
# The host's main address (the WSL address under WSL, the VM's on a CI runner).
host_ip() { ip -4 route get 1.1.1.1 | awk '{for (i = 1; i < NF; i++) if ($i == "src") print $(i + 1)}'; }

fails=0
ok() { echo "  ok    $*"; }
bad() {
	echo "  FAIL  $*"
	fails=$((fails + 1))
}

# run IMAGE ARGS… — a throwaway client on the lab network (removed on exit).
run() { docker run --rm --network "$NET" --label "$RUNNER_LABEL" --cap-drop ALL --security-opt no-new-privileges "$@"; }

check() {
	local before=${1:-} ids id name ports nets ip
	echo "== isolation check"
	ids=$(compose ps -aq)
	[ -n "$ids" ] || { bad "no lab containers"; return; }
	for id in $ids; do
		name=$(docker inspect -f '{{.Name}}' "$id")
		ports=$(docker inspect -f '{{json .HostConfig.PortBindings}}|{{json .NetworkSettings.Ports}}' "$id")
		nets=$(docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' "$id")
		if [[ $ports =~ ^(\{\}|null)\|(\{\}|null)$ ]] && [ "$nets" = "$NET " ]; then
			ok "${name#/}: no published ports, only network $NET"
		else
			bad "${name#/}: ports $ports, networks $nets"
		fi
	done
	if [ "$(docker network inspect -f '{{.Internal}}' "$NET")" = true ]; then
		ok "network $NET is internal (no gateway to the host or the Internet): $(docker network inspect -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}' "$NET")"
	else
		bad "network $NET is not internal"
	fi

	echo "== host listeners (0.0.0.0, [::], * and the host address $(host_ip))"
	listeners | grep -E " (0\.0\.0\.0|\[::\]|\*|$(host_ip | sed 's/\./\\./g')):" | sed 's/^/        /' || echo "        (none)"
	if [ -n "$before" ]; then
		local new
		new=$(comm -13 <(echo "$before") <(listeners))
		if [ -z "$new" ]; then
			ok "no new listening socket on the host since before the lab started"
		else
			bad "new listening sockets: $new"
		fi
	fi
	for ip in "$(host_ip)" 127.0.0.1 0.0.0.0; do
		if timeout 2 bash -c "exec 3<>/dev/tcp/$ip/443" 2>/dev/null; then
			bad "something accepts connections on $ip:443"
		else
			ok "nothing accepts connections on $ip:443"
		fi
	done

	echo "== from inside the lab network"
	if run --entrypoint /usr/bin/timeout "$PROBE_IMAGE" 5 openssl s_client -connect 1.1.1.1:443 </dev/null >/dev/null 2>&1; then
		bad "the lab network reaches the Internet"
	else
		ok "no route to the Internet (1.1.1.1:443 unreachable)"
	fi
	local up
	# shellcheck disable=SC2016 # $s expands inside the container
	up=$(run --entrypoint /bin/bash "$PROBE_IMAGE" -c \
		'for s in '"$(compose config --services | tr '\n' ' ')"'; do timeout 2 bash -c "exec 3<>/dev/tcp/$s/443" 2>/dev/null && echo -n "$s "; done')
	ok "servers answering on :443: $up"
	[ "$fails" = 0 ] || { echo "ISOLATION CHECK FAILED ($fails)"; return 1; }
	echo "isolation check passed"
}

case "${1:-}" in
up)
	before=$(listeners)
	# TLSLAB_NO_BUILD=1: the images are already built (the CI builds them with a cache).
	[ -n "${TLSLAB_NO_BUILD:-}" ] || compose build -q
	compose up -d --wait --quiet-pull
	check "$before"
	;;
check)
	check
	;;
testssl)
	out=$here/reference
	mkdir -p "$out"
	docker network inspect "$NET" >/dev/null 2>&1 || { echo "lab is not running: make tlslab-up"; exit 1; }
	shift
	servers=("$@")
	[ ${#servers[@]} -gt 0 ] || mapfile -t servers < <(compose config --services)
	for s in "${servers[@]}"; do
		echo "== testssl.sh $s"
		rm -f "$out/$s.json" "$out/$s.log"
		# testssl.sh exits non-zero when it finds problems: that is the point here.
		run --user "$(id -u):$(id -g)" -v "$out:/out" "$TESTSSL_IMAGE" --quiet --color 0 --warnings off \
			--jsonfile-pretty "/out/$s.json" --logfile "/out/$s.log" -p -E -f -S -P "$s:443" >/dev/null 2>&1 || true
		[ -s "$out/$s.json" ] || { echo "no output for $s"; exit 1; }
	done
	python3 -I "$here/summarize.py" "$out" >/dev/null
	echo "summary: $out/SUMMARY.md"
	;;
down)
	compose down -v --remove-orphans ${TLSLAB_RMI:+--rmi local} >/dev/null 2>&1 || true
	docker ps -aq --filter "label=$RUNNER_LABEL" | xargs -r docker rm -f >/dev/null
	c=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" | wc -l)
	c=$((c + $(docker ps -aq --filter "label=$RUNNER_LABEL" | wc -l)))
	n=$(docker network ls -q --filter "name=^$NET\$" | wc -l)
	n=$((n + $(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT" | wc -l)))
	v=$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" | wc -l)
	echo "left after down: containers=$c networks=$n volumes=$v"
	[ "$c$n$v" = 000 ] || exit 1
	;;
run)
	shift
	run "$@"
	;;
gotest | scan)
	# Go in a container on the lab network, with the repository read-only and
	# the host module cache (the lab has no Internet). gotest runs the
	# integration tests (-tags tlslab); scan runs cmd/qrscan -lab.
	docker network inspect "$NET" >/dev/null 2>&1 || { echo "lab is not running: make tlslab-up"; exit 1; }
	repo=$(cd "$here/../.." && pwd)
	results=$here/results
	modcache=${GOMODCACHE:-$(go env GOMODCACHE 2>/dev/null || echo "$HOME/go/pkg/mod")}
	mkdir -p "$results"
	# shellcheck disable=SC2054 # the commas are --tmpfs mount options
	gorun=(run --user "$(id -u):$(id -g)" -v "$repo:/src:ro" -v "$modcache:/go/pkg/mod:ro" -v "$results:/out"
		--tmpfs /tmp:rw,exec,size=2g -e HOME=/tmp -e GOCACHE=/tmp/gocache -e GOTOOLCHAIN=local -e GOFLAGS=-buildvcs=false
		-e QR_TLSLAB_OUT=/out -w /src "$GO_IMAGE")
	if [ "$1" = gotest ]; then
		"${gorun[@]}" go test -tags tlslab -count=1 -run TestLab -v ./internal/scanner/
	else
		shift
		"${gorun[@]}" go run ./cmd/qrscan -lab "$@"
	fi
	;;
*)
	sed -n '2,12p' "$0"
	exit 2
	;;
esac
