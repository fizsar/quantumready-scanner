#!/usr/bin/env python3
"""Checks the security policy of deploy/compose.prod.yaml.

Usage: docker compose -f deploy/compose.prod.yaml config --format json | python3 deploy/check_compose.py

Run by CI (and `make check-compose`). Exits 1 listing every violation.
"""
import json
import re
import sys

cfg = json.load(sys.stdin)
services, networks = cfg["services"], cfg["networks"]
errors = []


def check(ok, msg):
    if not ok:
        errors.append(msg)


# Networks: backend has no route outside; web is Caddy's (external).
check(networks.get("backend", {}).get("internal") is True, "network backend must be internal")
check(networks.get("web", {}).get("external") is True, "network web must be the external Caddy network")

expected_networks = {
    "db": {"backend"},
    "migrate": {"backend"},
    "api": {"backend", "web"},
    "worker": {"backend", "egress"},  # never web: scan targets must not reach Caddy/API through it
    "mailer": {"backend", "mail-egress"},  # its own way out, not the worker's; never web
    "web": {"web"},
}
for name, want in expected_networks.items():
    check(name in services, f"service {name} missing")
    if name in services:
        got = set(services[name].get("networks", {}))
        check(got == want, f"{name}: networks {sorted(got)}, want {sorted(want)}")

for name, svc in services.items():
    check(svc.get("read_only") is True, f"{name}: read_only must be true")
    check("no-new-privileges:true" in svc.get("security_opt", []), f"{name}: no-new-privileges missing")
    check(svc.get("cap_drop") == ["ALL"], f"{name}: cap_drop must be [ALL]")
    limits = svc.get("deploy", {}).get("resources", {}).get("limits", {})
    check("memory" in limits and "cpus" in limits, f"{name}: CPU and memory limits required")
    check(str(svc.get("memswap_limit")) == str(limits.get("memory")), f"{name}: memswap_limit must equal the memory limit")
    for p in svc.get("ports", []):
        check(p.get("host_ip") == "127.0.0.1", f"{name}: port {p.get('published')} must bind 127.0.0.1")
    check("healthcheck" in svc or name == "migrate", f"{name}: healthcheck required")

for name in ("db", "worker", "mailer", "migrate"):
    check(not services.get(name, {}).get("ports"), f"{name}: must not publish ports")

for name in ("worker", "mailer"):
    if name in services:
        test = services[name]["healthcheck"]["test"]
        check(test[:1] == ["CMD"], f"{name}: healthcheck must not use a shell (CMD, not CMD-SHELL)")

if "worker" in services:
    env = services["worker"].get("environment", {})
    check("sslmode=disable" in env.get("DATABASE_URL", ""), "worker: DATABASE_URL must set sslmode=disable")

# Least privilege (Apertura step 4): the API and the worker log in as their
# own users, never as the owner (a superuser skips row-level security).
for name, role in (("api", "qr_app"), ("worker", "qr_worker")):
    if name in services:
        url = services[name].get("environment", {}).get("DATABASE_URL", "")
        check(url.startswith(f"postgres://{role}:"), f"{name}: DATABASE_URL must log in as {role}")

if "mailer" in services:
    env = services["mailer"].get("environment", {})
    # Credentials only over TLS (the mailer refuses them anyway).
    check(env.get("SMTP_SECURITY") in ("starttls", "tls"), "mailer: SMTP_SECURITY must default to starttls or tls")
    check(services["mailer"].get("profiles") == ["mail"], "mailer: must be behind the mail profile until configured")
# Images: pinned by digest, so a deploy never pulls something nobody chose
# (on 23/10/2026 the moving postgres:17-alpine tag recreated the database
# with 17.11). Ours too (security review of 01/11/2026): a GHCR tag can be
# pushed again, so the VPS runs the digest the Release recorded after Trivy
# (QR_API_IMAGE, QR_WEB_IMAGE, set by deploy.sh).
OWN = "ghcr.io/fizsar/quantumready-saas-"
for name, svc in services.items():
    image = svc.get("image", "")
    pinned = re.search(r"@sha256:[0-9a-f]{64}$", image) is not None
    check(pinned, f"{name}: image {image!r} must be pinned by digest (name[:tag]@sha256:...)")
    if name != "db":
        own = OWN + ("web" if name == "web" else "api") + "@"
        check(image.startswith(own), f"{name}: image {image!r} must be {own}sha256:...")

if errors:
    print("compose policy violations:\n  " + "\n  ".join(errors))
    sys.exit(1)
print(f"compose policy ok ({len(services)} services)")
