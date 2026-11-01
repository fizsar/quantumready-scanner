#!/usr/bin/env python3
"""Delete old image versions from GHCR (the api and web images).

    deploy/ghcr_cleanup.py [--apply] [--keep sha-xxxxxxx ...]

Run by .github/workflows/ghcr-cleanup.yml (by hand). Without --apply it only
lists what it would delete and how much it would free.

Always kept, in both images:
- the tag in production and the rollback tag: the last two lines of
  deploy/deployments.txt (deploy.sh appends a line after each deploy);
- every tag deployed in the last --days days (30), from the same file;
- the --per-branch (5) most recent builds of each branch (main, showcase),
  in first-parent order;
- the version tagged latest, any --keep tag, and every untagged version
  (they may be parts of a multi-platform image; nothing here deletes them).

It refuses to run if the file names no deployment, or if the production or
rollback tag is missing from an image: the file would be wrong.

Needs GH_TOKEN with packages: read (packages: write for --apply) and the git
history of the branches (origin/main, origin/showcase).
"""
import argparse
import base64
import datetime as dt
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request

OWNER = "fizsar"
PACKAGES = ["quantumready-saas-api", "quantumready-saas-web"]
LINE = re.compile(r"^(\d{4}-\d{2}-\d{2})(?:T(\d{2}:\d{2})Z)? +(sha-[0-9a-f]{7,40})$")


class PlanError(Exception):
    pass


def read_deployments(text):
    """[(datetime UTC, tag)] in file order; comments and blank lines skipped."""
    out = []
    for n, raw in enumerate(text.splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        m = LINE.match(line)
        if not m:
            raise PlanError(f"deployments line {n} not understood: {raw!r}")
        when = dt.datetime.fromisoformat(f"{m[1]}T{m[2] or '00:00'}+00:00")
        out.append((when, m[3]))
    return out


def plan(versions, deployments, branches, now, days=30, per_branch=5, extra=()):
    """Decide which versions to keep.

    versions: [{"id", "tags": [..]}]; deployments: read_deployments();
    branches: {name: [full commit sha, newest first (first parent)]}.
    Returns (keep: {id: [reasons]}, delete: [version]).
    """
    if not deployments:
        raise PlanError("no deployment in the file: it must name at least the one in production")
    by_tag = {t: v for v in versions for t in v["tags"]}
    keep = {}

    def hold(tag, why, required=False):
        v = by_tag.get(tag)
        if v is None:
            if required:
                raise PlanError(f"{tag} ({why}) is not in the image: is the deployments file right?")
            return
        keep.setdefault(v["id"], []).append(why)

    hold(deployments[-1][1], "production", required=True)
    if len(deployments) > 1:
        hold(deployments[-2][1], "rollback", required=True)
    since = now - dt.timedelta(days=days)
    for when, tag in deployments:
        if when >= since:
            hold(tag, f"deployed {when:%Y-%m-%d}")
    for name, commits in branches.items():
        found = 0
        for c in commits:
            tag = next((t for t in by_tag if t.startswith("sha-") and c.startswith(t[4:])), None)
            if tag is None:
                continue
            hold(tag, f"{name} #{found + 1}")
            found += 1
            if found == per_branch:
                break
    hold("latest", "latest")
    for tag in extra:
        hold(tag, "--keep")
    for v in versions:
        if not v["tags"]:
            keep.setdefault(v["id"], []).append("untagged")
    delete = [v for v in versions if v["id"] not in keep]
    return keep, delete


# --- GitHub and the registry -------------------------------------------------

def api(token, method, path):
    req = urllib.request.Request(
        "https://api.github.com" + path, method=method,
        headers={"Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json",
                 "X-GitHub-Api-Version": "2022-11-28"})
    with urllib.request.urlopen(req) as r:
        body = r.read()
        return json.loads(body) if body else None


def list_versions(token, pkg):
    out, page = [], 1
    while True:
        got = api(token, "GET", f"/users/{OWNER}/packages/container/{pkg}/versions?per_page=100&page={page}")
        out += [{"id": v["id"], "name": v["name"], "created": v["created_at"],
                 "tags": v["metadata"]["container"]["tags"]} for v in got]
        if len(got) < 100:
            return out
        page += 1


ACCEPT = ", ".join(["application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.manifest.v1+json",
                    "application/vnd.docker.distribution.manifest.list.v2+json",
                    "application/vnd.docker.distribution.manifest.v2+json"])


def blob_sizes(token, pkg, versions):
    """{version id: {blob digest: size}} from the manifests (compressed sizes)."""
    basic = base64.b64encode(f"{OWNER}:{token}".encode()).decode()
    req = urllib.request.Request(f"https://ghcr.io/token?scope=repository:{OWNER}/{pkg}:pull",
                                 headers={"Authorization": "Basic " + basic})
    with urllib.request.urlopen(req) as r:
        bearer = json.load(r)["token"]

    def manifest(ref):
        req = urllib.request.Request(f"https://ghcr.io/v2/{OWNER}/{pkg}/manifests/{ref}",
                                     headers={"Authorization": "Bearer " + bearer, "Accept": ACCEPT})
        with urllib.request.urlopen(req) as r:
            return json.load(r)

    out = {}
    for v in versions:
        m = manifest(v["name"])
        parts = [m] if "layers" in m else [manifest(x["digest"]) for x in m.get("manifests", [])]
        out[v["id"]] = {b["digest"]: b["size"] for p in parts for b in p.get("layers", []) + [p.get("config") or {}] if b}
    return out


def first_parent(branch):
    return subprocess.run(["git", "log", "--first-parent", "--format=%H", "-n", "500", f"origin/{branch}"],
                          capture_output=True, text=True, check=True).stdout.split()


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--apply", action="store_true", help="delete (default: dry run, list only)")
    ap.add_argument("--keep", nargs="*", default=[], help="extra tags to keep")
    ap.add_argument("--deployments", default="deploy/deployments.txt")
    ap.add_argument("--days", type=int, default=30)
    ap.add_argument("--per-branch", type=int, default=5)
    ap.add_argument("--branches", default="main,showcase")
    args = ap.parse_args()
    token = os.environ.get("GH_TOKEN") or sys.exit("GH_TOKEN is not set")
    summary = os.environ.get("GITHUB_STEP_SUMMARY")

    with open(args.deployments, encoding="utf-8") as f:
        deployments = read_deployments(f.read())
    branches = {b: first_parent(b) for b in args.branches.split(",")}
    now = dt.datetime.now(dt.timezone.utc)
    mode = "DELETE" if args.apply else "dry run"
    report = [f"## GHCR cleanup ({mode})", "",
              f"Production {deployments[-1][1] if deployments else '-'}, rollback "
              f"{deployments[-2][1] if len(deployments) > 1 else '-'}; deployed since "
              f"{now - dt.timedelta(days=args.days):%Y-%m-%d}; {args.per_branch} per branch of {args.branches}.", ""]
    plans = []
    for pkg in PACKAGES:  # plan both images before deleting anything
        versions = list_versions(token, pkg)
        keep, delete = plan(versions, deployments, branches, now, args.days, args.per_branch, args.keep)
        plans.append((pkg, versions, keep, delete))
    total = 0
    for pkg, versions, keep, delete in plans:
        sizes = blob_sizes(token, pkg, versions)
        kept_blobs = {d for v in versions if v["id"] in keep for d in sizes[v["id"]]}
        freed = {d: s for v in delete for d, s in sizes[v["id"]].items() if d not in kept_blobs}
        all_blobs = {d: s for v in versions for d, s in sizes[v["id"]].items()}
        total += sum(freed.values())
        report += [f"### {pkg}: keep {len(keep)} of {len(versions)}, delete {len(delete)}, "
                   f"frees {sum(freed.values()) / 1e6:.0f} MB of {sum(all_blobs.values()) / 1e6:.0f} MB", "",
                   "Kept:", ""]
        for v in versions:
            if v["id"] in keep:
                report.append(f"- {', '.join(v['tags']) or v['name'][:19]} ({v['created'][:10]}): {'; '.join(keep[v['id']])}")
        report += ["", f"{'Deleted' if args.apply else 'Would delete'}:", ""]
        report += [f"- {', '.join(v['tags'])} ({v['created'][:10]})" for v in delete] or ["- nothing"]
        report.append("")
    report.append(f"**Total {'freed' if args.apply else 'to free'}: {total / 1e6:.0f} MB** (compressed layer sizes)")
    if args.apply:
        for pkg, _, _, delete in plans:
            for v in delete:
                try:
                    api(token, "DELETE", f"/users/{OWNER}/packages/container/{pkg}/versions/{v['id']}")
                except urllib.error.HTTPError as e:
                    sys.exit(f"delete {pkg} {v['tags']}: HTTP {e.code} {e.read()[:200]!r}; stopped")
    text = "\n".join(report) + "\n"
    print(text)
    if summary:
        with open(summary, "a", encoding="utf-8") as f:
            f.write(text)


if __name__ == "__main__":
    try:
        main()
    except PlanError as e:
        sys.exit(f"refusing: {e}")
