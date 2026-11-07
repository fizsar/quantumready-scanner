#!/usr/bin/env python3
"""Fails when a Dockerfile's package pins over its base image are no longer needed.

web/Dockerfile installs fixed versions of packages that its pinned base image
(nginx-unprivileged, by digest) still ships vulnerable:

    RUN apk add --no-cache 'musl>=1.2.6-r3' 'zlib>=1.3.2-r1'   # base-pins

This reads the FROM image of the final stage and every 'pkg>=version' of the
line marked `# base-pins`, asks the base image itself which versions it carries
(apk info -v) and compares them with apk's own ordering (apk version -t). If the
base already carries every minimum, the pins are dead weight: it fails and says
to drop them. Since the base is pinned by digest, this happens exactly in the PR
that moves the digest (Dependabot), not at random.

    python3 deploy/check_base_pins.py web/Dockerfile

Exit 0: no pins, or some pin still needed. Exit 1: every pin is already in the
base (drop the block). Exit 2: the Dockerfile or docker did not answer as
expected.
"""
import re
import subprocess
import sys

MARK = "# base-pins"


def parse(dockerfile: str):
    """The final stage's base image and the {package: minimum} of the marked line."""
    base = None
    pins = {}
    for line in open(dockerfile, encoding="utf-8"):
        m = re.match(r"^FROM\s+(\S+)", line)
        if m:
            base = m.group(1)
            pins = {}  # pins belong to the final stage only
        if MARK in line and line.lstrip().startswith("RUN"):
            for pkg, ver in re.findall(r"'([a-z0-9][a-z0-9.+_-]*)>=([0-9][^']*)'", line):
                pins[pkg] = ver
    return base, pins


def run(args):
    return subprocess.run(args, capture_output=True, text=True, check=True).stdout


def installed(base: str, docker=run):
    """Versions of the packages in the base image, from apk itself."""
    out = docker(["docker", "run", "--rm", "--entrypoint", "apk", base, "info", "-v"])
    versions = {}
    for item in out.split():
        m = re.match(r"^(.+?)-(\d[^-]*-r\d+)$", item)
        if m:
            versions[m.group(1)] = m.group(2)
    return versions


def at_least(base: str, have: str, want: str, docker=run) -> bool:
    """have >= want in apk's ordering, asked to the base image's apk."""
    out = docker(["docker", "run", "--rm", "--entrypoint", "apk", base, "version", "-t", have, want]).strip()
    if out not in ("<", "=", ">"):
        raise RuntimeError(f"apk version -t {have} {want}: unexpected answer {out!r}")
    return out in ("=", ">")


def check(dockerfile: str, docker=run):
    base, pins = parse(dockerfile)
    if not pins:
        return 0, [f"{dockerfile}: no base pins"]
    if not base or "@sha256:" not in base:
        return 2, [f"{dockerfile}: the final stage's base is not pinned by digest: {base}"]
    have = installed(base, docker)
    lines, needed = [], 0
    for pkg, want in sorted(pins.items()):
        cur = have.get(pkg)
        if cur is None:
            lines.append(f"{pkg}: not in the base image (pin needed: it installs it)")
            needed += 1
        elif at_least(base, cur, want, docker):
            lines.append(f"{pkg}: the base already has {cur} >= {want}")
        else:
            lines.append(f"{pkg}: the base has {cur} < {want}: pin still needed")
            needed += 1
    if needed:
        return 0, lines
    lines.append(f"every pin is already in the base image: drop the '{MARK}' block from {dockerfile}")
    return 1, lines


def main():
    if len(sys.argv) != 2:
        print(__doc__.strip().splitlines()[0])
        return 2
    try:
        code, lines = check(sys.argv[1])
    except (OSError, subprocess.CalledProcessError, RuntimeError) as e:
        print(f"check_base_pins: {e}", file=sys.stderr)
        return 2
    for line in lines:
        print(line)
    return code


if __name__ == "__main__":
    sys.exit(main())
