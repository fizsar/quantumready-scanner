#!/usr/bin/env python3
"""No executable binaries and no files over 1 MB in the repository.

Usage: python3 deploy/check_files.py [ROOT]   (default: the repository)

Looks at every file git tracks (the index: what a commit would hold) and
fails on:
- an executable binary, by its content (ELF, Windows PE, Mach-O), whatever
  its mode: a build of cmd/worker went into main this way (3cf9ce8);
- a file with the executable bit that is not a script (no #! line);
- a file over 1 MB.
Only the paths in the lists below are allowed to break a rule. Adding one is
a deliberate change, reviewed in the same commit.

Run by CI (`make check-files`). Exits 1 listing every problem.
"""
import fnmatch
import pathlib
import subprocess
import sys

MAX_BYTES = 1024 * 1024

# Allowed over 1 MB: documentation images, the social card, test fixtures
# and embedded reference data.
LARGE_OK = (
    "docs/screenshots/*",
    "web/public/og.png",
    "*/testdata/*",
    "test/tlslab/reference/*",
    "internal/domains/public_suffix_list.dat",
)
# Allowed executable binaries: none.
BINARY_OK: tuple[str, ...] = ()

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else pathlib.Path(__file__).resolve().parent.parent)


def allowed(path, patterns):
    return any(fnmatch.fnmatch(path, p) for p in patterns)


def binary_kind(head):
    """The kind of executable binary these first bytes start, or None."""
    if head.startswith(b"\x7fELF"):
        return "ELF"
    if head[:4] in (b"\xfe\xed\xfa\xce", b"\xfe\xed\xfa\xcf", b"\xce\xfa\xed\xfe", b"\xcf\xfa\xed\xfe", b"\xca\xfe\xba\xbe"):
        return "Mach-O"
    if head.startswith(b"MZ") and len(head) >= 64:
        pe = int.from_bytes(head[0x3C:0x40], "little")
        if head[pe : pe + 4] == b"PE\0\0":
            return "PE"
    return None


out = subprocess.run(["git", "-C", str(root), "ls-files", "-s", "-z"], check=True, capture_output=True).stdout
errors = []
for entry in out.split(b"\0"):
    if not entry:
        continue
    meta, path = entry.decode().split("\t", 1)
    mode = meta.split()[0]
    if mode == "120000" or mode == "160000":  # symlinks, submodules
        continue
    f = root / path
    if not f.is_file():
        continue
    size = f.stat().st_size
    with f.open("rb") as fh:
        head = fh.read(4096)
    kind = binary_kind(head)
    if kind and not allowed(path, BINARY_OK):
        errors.append(f"{path}: executable binary ({kind}); build output never goes into the repository")
    elif mode == "100755" and not head.startswith(b"#!") and not allowed(path, BINARY_OK):
        errors.append(f"{path}: executable bit on a file that is not a script (no #! line)")
    if size > MAX_BYTES and not allowed(path, LARGE_OK):
        errors.append(f"{path}: {size / MAX_BYTES:.1f} MB, over the 1 MB limit (exceptions: LARGE_OK in deploy/check_files.py)")

for e in errors:
    print(e, file=sys.stderr)
if errors:
    sys.exit(1)
print("no executable binaries; no files over 1 MB outside the exceptions")
