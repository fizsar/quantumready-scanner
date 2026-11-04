#!/usr/bin/env bash
# No text over the 3D stage unless it sits on its own opaque panel: the lab
# pages scrolled from top to bottom in a real Chromium, at a phone width and
# at an iPhone's (393×852, DPR 3). Same throwaway setup as overflow-check.sh
# (production nginx config, internal Docker network, no Internet). Chromium
# runs without a GPU here, so the stage shows the still images: the same
# element, the same visibility rules as the 3D canvas.
#
#   web/scripts/overlap-check.sh [SIZES] [PAGE...]
#   SIZES: WxHxDPR,... (default 375x667x2,375x812x1,393x852x3,1440x900x1)
#
# Also checks that the panels that stay on screen never scroll inside or cut
# an equation, and that on phones the scene keeps at least 40 % of the height.
set -euo pipefail
SIZES=${1:-375x667x2,375x812x1,393x852x3,1440x900x1}
shift || true
[ $# -gt 0 ] || set -- '/lab/historia?lang=es' '/lab/historia?lang=en'
CHECK=overlap-check.mjs exec bash "$(dirname "$0")/overflow-check.sh" "$SIZES" "$@"
