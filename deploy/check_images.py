#!/usr/bin/env python3
"""Every container image the repository uses is pinned by digest.

Usage: python3 deploy/check_images.py [ROOT]   (default: the repository)

Looks at Dockerfiles (FROM), compose files and workflows (image:) and the
*_IMAGE defaults of scripts. Allowed without a digest: FROM scratch, the
lab's own local images (qr-tlslab/*), our own GHCR images (immutable
sha-<commit> tags) and build stages named in the same Dockerfile.

Also: PostgreSQL is the same digest everywhere it appears (production, the
restore drill, dev, CI, the script tests), so a Dependabot PR that bumps
deploy/compose.prod.yaml fails CI until the others follow.

Run by CI (`make check-compose`). Exits 1 listing every problem.
"""
import pathlib
import re
import sys

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else pathlib.Path(__file__).resolve().parent.parent)
DIGEST = re.compile(r"@sha256:[0-9a-f]{64}\b")
ALLOWED = (re.compile(r"^scratch$"), re.compile(r"^qr-tlslab/"), re.compile(r"^ghcr\.io/fizsar/quantumready-saas-"))
# test-check-images.sh holds unpinned images on purpose (its mutations).
SKIP = ("node_modules", "/dist/", "/.git/", "test-check-images.sh")

errors, postgres = [], {}


def files(*globs):
    for g in globs:
        for p in sorted(root.glob(g)):
            if p.is_file() and not any(s in str(p) for s in SKIP):
                yield p


def check(path, line_no, ref, stages=()):
    ref = ref.strip().strip("'\"")
    if not ref or "$" in ref.split("@")[0] or ref in stages or any(a.search(ref) for a in ALLOWED):
        return
    where = f"{path.relative_to(root)}:{line_no}"
    if not DIGEST.search(ref):
        errors.append(f"{where}: {ref} is not pinned by digest")
    if ref.startswith("postgres:"):
        postgres.setdefault(ref, []).append(where)


for p in files("**/Dockerfile", "**/*.Dockerfile"):
    stages = set()
    for n, line in enumerate(p.read_text().splitlines(), 1):
        m = re.match(r"\s*FROM\s+(?:--\S+\s+)*(\S+)(?:\s+AS\s+(\S+))?", line, re.I)
        if m:
            check(p, n, m.group(1), stages)
            if m.group(2):
                stages.add(m.group(2))

for p in files("**/compose*.yaml", "**/compose*.yml", ".github/workflows/*.yml", "deploy/**/*.sh", "deploy/*.sh", "web/scripts/*.sh", "web/og/*.sh", "test/**/*.sh"):
    for n, line in enumerate(p.read_text().splitlines(), 1):
        code = line.split(" #")[0]
        m = re.match(r"\s*(?:-\s*)?image:\s*(\S+)", code)
        # Not images: YAML aliases (*api-image, our own GHCR image, resolved by
        # check_compose.py) and workflow matrix entries (image: api).
        workflow_name = ".github" in str(p) and not re.search(r"[:/]", m.group(1)) if m else False
        if m and not m.group(1).startswith("*") and not workflow_name:
            check(p, n, m.group(1))
        # VAR_IMAGE=name:tag@... and any default ${..._IMAGE:-name:tag@...}
        # (CHROME=${CHROME_IMAGE:-zenika/alpine-chrome:124@sha256:...}).
        for m in re.finditer(r"\b[A-Z_]*IMAGE=([a-z0-9][^\s}\"'$]*)", code):
            check(p, n, m.group(1))
        for m in re.finditer(r"\$\{[A-Z_]*IMAGE:-([a-z0-9][^\s}\"']*)", code):
            check(p, n, m.group(1))
        # docker run ... koalaman/shellcheck:v0.10.0@sha256:... (tools run by scripts)
        for m in re.finditer(r"docker run [^|;]*?\s((?:[a-z0-9.-]+/)+[a-z0-9._-]+:[A-Za-z0-9._-]+(?:@sha256:[0-9a-f]{64})?)", code):
            check(p, n, m.group(1))

if len(postgres) > 1:
    errors.append("PostgreSQL is pinned to different images: " + "; ".join(f"{r} at {', '.join(w)}" for r, w in postgres.items()))
if not postgres:
    errors.append("no PostgreSQL image found (the checker lost track of it)")

if errors:
    print("image pinning problems:\n  " + "\n  ".join(errors))
    sys.exit(1)
print(f"images pinned by digest; PostgreSQL in {sum(len(w) for w in postgres.values())} places, one digest")
