#!/usr/bin/env python3
"""Checks .trivyignore.yaml before the release gate uses it.

Standard library only (runs on the runner as is), so it accepts a strict
subset of YAML: a top-level "vulnerabilities:" that is either "[]" or a list
of entries, each preceded by a comment line and holding id, statement and
expired_at (plus optional paths). Anything else is an error.

Usage: trivyignore_check.py FILE
"""
import datetime
import re
import sys

KEYS = {"id", "statement", "expired_at", "paths"}
REQUIRED = {"id", "statement", "expired_at"}


def check(text):
    errors, entries = [], []
    lines = text.splitlines()
    top = [i for i, l in enumerate(lines) if l.startswith("vulnerabilities:")]
    others = [l for l in lines if l and not l.startswith((" ", "#", "vulnerabilities:"))]
    if len(top) != 1 or others:
        return ["want exactly one top-level key, vulnerabilities"], []
    start = top[0]
    if lines[start].split(":", 1)[1].strip() == "[]":
        rest = [l for l in lines[start + 1:] if l.strip() and not l.lstrip().startswith("#")]
        return (["content after vulnerabilities: []"] if rest else []), []
    cur, comment = None, False
    for n, line in enumerate(lines[start + 1:], start + 2):
        s = line.strip()
        if not s:
            continue
        if s.startswith("#"):
            comment = True
            continue
        m = re.match(r"^  - (\w+):\s*(.*)$", line) or re.match(r"^    (\w+):\s*(.*)$", line)
        if not m:
            errors.append(f"line {n}: unexpected {line!r}")
            continue
        if line.startswith("  - "):
            cur = {"_line": n, "_comment": comment}
            entries.append(cur)
        elif cur is None:
            errors.append(f"line {n}: key outside an entry")
            continue
        comment = False
        key, val = m.group(1), m.group(2).strip().strip('"').strip("'")
        if key not in KEYS:
            errors.append(f"line {n}: unknown key {key}")
        cur[key] = val
    for e in entries:
        where = f"entry at line {e['_line']} ({e.get('id', '?')})"
        missing = REQUIRED - e.keys()
        if missing:
            errors.append(f"{where}: missing {', '.join(sorted(missing))}")
        if not e["_comment"]:
            errors.append(f"{where}: needs a comment line above it with the justification")
        if not e.get("statement"):
            errors.append(f"{where}: empty statement")
        if "expired_at" in e:
            try:
                datetime.date.fromisoformat(e["expired_at"])
            except ValueError:
                errors.append(f"{where}: expired_at must be YYYY-MM-DD")
    return errors, entries


def main():
    with open(sys.argv[1], encoding="utf-8") as f:
        errors, entries = check(f.read())
    for e in errors:
        print(f"{sys.argv[1]}: {e}", file=sys.stderr)
    if errors:
        sys.exit(1)
    today = datetime.date.today()
    for e in entries:
        state = "EXPIRED (blocks again)" if datetime.date.fromisoformat(e["expired_at"]) < today else "until " + e["expired_at"]
        print(f"exception {e['id']}: {state} — {e['statement']}")
    print(f"{len(entries)} exception(s)")


if __name__ == "__main__":
    main()
