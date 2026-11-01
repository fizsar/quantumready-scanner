# shellcheck shell=bash
# Pins the images of a deploy by digest in the next .env (security review of
# 01/11/2026). deploy.sh sends this file to the VPS ahead of its own script;
# deploy/test-pin-images.sh tests it with a fake docker.
#
# The Release (release.yml) builds each image, gates it with Trivy, pushes
# it and records the pushed digest as a status of the commit (release/api,
# release/web). deploy.sh reads those and passes them here as API_REF and
# WEB_REF (ghcr.io/fizsar/quantumready-saas-<name>@sha256:...). compose then
# runs exactly that image (QR_API_IMAGE, QR_WEB_IMAGE), whatever the tag
# points to later; a tag that no longer resolves to the recorded digest
# (re-pushed after the Release) stops the deploy.
#
# Releases from before this change have no recorded digest: with
# ALLOW_UNRECORDED=1 (QR_DEPLOY_PIN_UNRECORDED=1, e.g. a rollback to an older
# tag) the tag is resolved once here and that digest is pinned.

DOCKER=${DOCKER:-docker}
OWN_IMAGE=ghcr.io/fizsar/quantumready-saas-

# tag_digest REPO TAG: the digest REPO:TAG resolves to in the registry.
tag_digest() {
	"$DOCKER" pull --quiet "$1:$2" </dev/null >/dev/null || return 1
	"$DOCKER" image inspect -f '{{range .RepoDigests}}{{println .}}{{end}}' "$1:$2" </dev/null |
		sed -n "s|^$1@\(sha256:[0-9a-f]\{64\}\)$|\1|p" | head -1
}

# pin_images ENV_FILE TAG API_REF WEB_REF ALLOW_UNRECORDED: sets QR_API_IMAGE
# and QR_WEB_IMAGE in ENV_FILE, or fails without touching it.
pin_images() {
	local env=$1 tag=$2 allow=$5 name ref repo want got
	local -A refs=([api]=$3 [web]=$4) pinned=()
	for name in api web; do
		ref=${refs[$name]} repo=$OWN_IMAGE$name
		if [ -z "$ref" ]; then
			if [ "$allow" != 1 ]; then
				echo "refusing to deploy $tag: the Release recorded no digest for $name." >&2
				echo "Older releases have none: QR_DEPLOY_PIN_UNRECORDED=1 resolves the tag once and pins it." >&2
				return 1
			fi
			got=$(tag_digest "$repo" "$tag") || got=
			if [ -z "$got" ]; then
				echo "refusing to deploy $tag: $repo:$tag does not resolve to a digest" >&2
				return 1
			fi
			echo "==> $name: $tag not recorded by the Release; pinned as resolved now: $got"
			pinned[$name]=$repo@$got
			continue
		fi
		if [[ ! $ref =~ ^${repo//./\\.}@sha256:[0-9a-f]{64}$ ]]; then
			echo "refusing to deploy $tag: $name reference $ref is not $repo@sha256:<digest>" >&2
			return 1
		fi
		want=${ref#*@}
		got=$(tag_digest "$repo" "$tag") || got=
		if [ "$got" != "$want" ]; then
			echo "refusing to deploy $tag: $repo:$tag resolves to ${got:-nothing}, not $want, the digest the Release scanned and recorded." >&2
			echo "The tag was pushed again after the Release: find out why before deploying." >&2
			return 1
		fi
		echo "==> $name: $ref (recorded by the Release)"
		pinned[$name]=$ref
	done
	for name in api web; do
		local key
		key=QR_${name^^}_IMAGE
		if grep -q "^$key=" "$env"; then
			sed -i "s|^$key=.*|$key=${pinned[$name]}|" "$env"
		else
			echo "$key=${pinned[$name]}" >>"$env"
		fi
	done
}
