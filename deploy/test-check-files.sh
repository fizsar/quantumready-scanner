#!/usr/bin/env bash
# Tests deploy/check_files.py: the repository passes, and a throwaway git
# repository fails with each kind of file it must reject, one at a time.
# Needs git and python3.
# shellcheck disable=SC2015 # ok/FAIL reporting never fails
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
check=$PWD/deploy/check_files.py
tmp=$(mktemp -d)
elf=$(type -P true) # any ELF executable of the system
trap 'rm -rf "$tmp"' EXIT

fails=0
expect() { # expect pass|fail DESCRIPTION ROOT [GREP]
	local want=$1 d=$2 dir=$3 pattern=${4:-} got
	if python3 "$check" "$dir" >"$tmp/out" 2>&1; then got=pass; else got=fail; fi
	if [[ $got == "$want" ]] && { [[ -z $pattern ]] || grep -qE -- "$pattern" "$tmp/out"; }; then echo "ok   $d"; else
		echo "FAIL $d (want $want, got $got${pattern:+, output must match $pattern})"
		cat "$tmp/out"
		fails=1
	fi
}
repo() { # a fresh repository with one ordinary file and one script
	rm -rf "$tmp/r" && mkdir -p "$tmp/r/deploy" && git -C "$tmp/r" init -q
	echo "hello" >"$tmp/r/README.md"
	printf '#!/usr/bin/env bash\necho hi\n' >"$tmp/r/deploy/run.sh" && chmod 755 "$tmp/r/deploy/run.sh"
	git -C "$tmp/r" add -A
}
add() { # add PATH [MODE]: stage a file written beforehand
	git -C "$tmp/r" add -f "$1"
	[[ -z ${2:-} ]] || git -C "$tmp/r" update-index --chmod="$2" "$1"
}
big() { mkdir -p "$(dirname "$tmp/r/$1")" && head -c $((1024 * 1024 + 1)) /dev/zero >"$tmp/r/$1"; }
pe() { # a minimal Windows PE header: MZ, e_lfanew at 0x3c, PE\0\0 there
	python3 -c 'import sys; h = bytearray(128); h[:2] = b"MZ"; h[0x3c] = 64; h[64:68] = b"PE\0\0"; open(sys.argv[1], "wb").write(h)' "$1"
}

expect pass "the repository: no binaries, nothing over 1 MB outside the exceptions" .

repo
expect pass "a text file and an executable script" "$tmp/r"

repo && cp "$elf" "$tmp/r/worker" && add worker
expect fail "an ELF binary (like the worker of 3cf9ce8)" "$tmp/r" "worker: executable binary \(ELF\)"

repo && cp "$elf" "$tmp/r/data.bin" && chmod 644 "$tmp/r/data.bin" && add data.bin -x
expect fail "an ELF binary without the executable bit" "$tmp/r" "data.bin: executable binary \(ELF\)"

repo && pe "$tmp/r/tool.exe" && add tool.exe
expect fail "a Windows PE binary" "$tmp/r" "tool.exe: executable binary \(PE\)"

repo && printf '\xcf\xfa\xed\xfe rest' >"$tmp/r/mac" && add mac
expect fail "a Mach-O binary" "$tmp/r" "mac: executable binary \(Mach-O\)"

repo && printf 'MZ is just text here\n' >"$tmp/r/notes.txt" && add notes.txt
expect pass "a text file that starts with MZ is not a PE binary" "$tmp/r"

repo && echo "no shebang" >"$tmp/r/run" && add run +x
expect fail "the executable bit on a file without #!" "$tmp/r" "run: executable bit"

repo && big web/src/huge.json && add web/src/huge.json
expect fail "a file over 1 MB" "$tmp/r" "web/src/huge.json: 1.0 MB, over the 1 MB limit"

repo && head -c $((1024 * 1024)) /dev/zero >"$tmp/r/exact" && add exact
expect pass "exactly 1 MB is allowed" "$tmp/r"

for ok in docs/screenshots/S9/home-es.png web/public/og.png internal/x/testdata/big.json test/tlslab/reference/tls12.json internal/domains/public_suffix_list.dat; do
	repo && big "$ok" && add "$ok"
	expect pass "over 1 MB in an exception: $ok" "$tmp/r"
done

repo && big web/public/og-new.png && add web/public/og-new.png
expect fail "an exception is a path, not a prefix (web/public/og-new.png)" "$tmp/r" "og-new.png"

repo && cp "$elf" "$tmp/r/a.out" && big docs/screenshots/a.png && big other.bin && add a.out && add docs/screenshots/a.png && add other.bin
expect fail "every problem is listed at once (a.out sorts first)" "$tmp/r" "a.out: executable"
grep -q "other.bin: 1.0 MB" "$tmp/out" && ! grep -q "screenshots" "$tmp/out" && echo "ok   ... the large file too, and not the exception" || {
	echo "FAIL the listing misses other.bin or names the exception"
	fails=1
}

repo && cp "$elf" "$tmp/r/worker"
expect pass "an untracked binary is not the repository's business (.gitignore)" "$tmp/r"

exit $fails
