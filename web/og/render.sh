#!/usr/bin/env bash
# Renders og.html to ../public/og.png (1200×630) with headless Chromium in a
# throwaway container (no network, read-only source). Run it after editing
# og.html and commit both files.
set -euo pipefail
cd "$(dirname "$0")"
CHROME=${CHROME_IMAGE:-zenika/alpine-chrome:124@sha256:eb3378c1ed0079f94db054a5fe1aaa790a254ec0d6bbc67eda052420d86a179d}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/fonts"
cp og.html "$work/"
cp ../node_modules/@fontsource/jetbrains-mono/files/jetbrains-mono-latin-{400,700}-normal.woff2 "$work/fonts/"
chmod -R a+rX "$work"
chmod a+w "$work"
docker run --rm --network none -v "$work:/og" "$CHROME" \
	--no-sandbox --headless --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
	--window-size=1200,630 --screenshot=/og/og.png file:///og/og.html >/dev/null 2>&1
cp "$work/og.png" ../public/og.png
echo "public/og.png: $(file -b ../public/og.png 2>/dev/null || stat -c %s ../public/og.png)"
