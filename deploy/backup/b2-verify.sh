#!/usr/bin/env bash
# Empirical check of the B2 design on a TEST bucket, with a key that has the
# same capabilities as the production one (no deleteFiles). Run from WSL:
#   make b2-verify
# Reads ~/.config/quantumready/b2-test.env (0600), never printed:
#   B2_KEY_ID=…  B2_APP_KEY=…  B2_BUCKET=fizsar-qr-backups-test
# Checks:
#   1. restic init, two backups, forget --prune and unlock work, leaving no locks
#   2. that key cannot delete a file version for good (native API and S3 API)
#   3. what restic deleted is only hidden, and the old versions are still
#      downloadable; the lifecycle rule keeps them N days and never hides live files
# Writes under a new prefix per run (restic-verify-<time>/); without
# deleteFiles it cannot clean up: the bucket's lifecycle rule removes it.
# shellcheck disable=SC2015 # ok and bad never fail
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=deploy/backup/common.sh
. "$here/common.sh"
conf=${B2_TEST_ENV:-$HOME/.config/quantumready/b2-test.env}
[ -f "$conf" ] || { echo "missing $conf (see the header of this script)"; exit 2; }
[ "$(stat -c %a "$conf")" = 600 ] || { echo "$conf must be chmod 600"; exit 2; }
set -a
# shellcheck source=/dev/null
. "$conf"
set +a
: "${B2_KEY_ID:?} ${B2_APP_KEY:?} ${B2_BUCKET:?}"
# B2 native API version (since 10/2026 b2_authorize_account rejects v1–v3).
B2_API=${B2_API:-v4}

fails=0
ok() { echo "ok   $*"; }
bad() {
	echo "FAIL $*"
	fails=$((fails + 1))
}
json() { python3 -Ic "import json,sys; d=json.load(sys.stdin); print($1)"; }
work=$(mktemp -d "${XDG_CACHE_HOME:-$HOME/.cache}/qr-b2-verify.XXXXXX")
trap 'rm -rf "$work"' EXIT
chmod 700 "$work"

# --- B2 native API: who are we, what can this key do
auth=$(curl -fsS -u "$B2_KEY_ID:$B2_APP_KEY" "https://api.backblazeb2.com/b2api/$B2_API/b2_authorize_account")
token=$(json 'd["authorizationToken"]' <<<"$auth")
account=$(json 'd["accountId"]' <<<"$auth")
api=$(json 'd["apiInfo"]["storageApi"]["apiUrl"]' <<<"$auth")
dl=$(json 'd["apiInfo"]["storageApi"]["downloadUrl"]' <<<"$auth")
s3=$(json 'd["apiInfo"]["storageApi"]["s3ApiUrl"]' <<<"$auth")
caps=$(json '" ".join(sorted(d["apiInfo"]["storageApi"]["allowed"]["capabilities"]))' <<<"$auth")
bucket_id=$(json '",".join(b["id"] for b in d["apiInfo"]["storageApi"]["allowed"]["buckets"] or [])' <<<"$auth")
bucket_names=$(json '",".join(str(b["name"]) for b in d["apiInfo"]["storageApi"]["allowed"]["buckets"] or [])' <<<"$auth")
region=${s3#https://s3.}
region=${region%%.*}
echo "key capabilities: $caps"
echo "key restricted to bucket(s): ${bucket_names:-ALL}   s3: $s3"
[ "$bucket_names" = "$B2_BUCKET" ] && ok "key restricted to $B2_BUCKET" || bad "key must be restricted to $B2_BUCKET only"
case " $caps " in *" deleteFiles "*) bad "key has deleteFiles (it must not)" ;; *) ok "key has no deleteFiles" ;; esac
# B2 answers 503 when busy: clients must retry.
download() { curl -fsS --retry 5 --retry-all-errors -H "Authorization: $token" "$dl/b2api/$B2_API/b2_download_file_by_id?fileId=$1" | sha1sum | cut -d" " -f1; }
b2() { curl -sS --retry 5 -o "$work/out" -w '%{http_code}' -H "Authorization: $token" -d "$2" "$api/b2api/$B2_API/$1"; }

code=$(b2 b2_list_buckets "{\"accountId\":\"$account\",\"bucketId\":\"$bucket_id\"}")
[ "$code" = 200 ] || { echo "b2_list_buckets: HTTP $code $(cat "$work/out")"; exit 1; }
rules=$(json 'd["buckets"][0].get("lifecycleRules", "not visible to this key: check it in the web UI")' <"$work/out")
echo "lifecycle rules: $rules"
python3 -Ic 'import json,sys
r = json.load(sys.stdin)["buckets"][0].get("lifecycleRules") or []
whole = [x for x in r if x.get("fileNamePrefix", "") == ""]
ok = len(whole) == 1 and (whole[0].get("daysFromHidingToDeleting") or 0) >= 7 and whole[0].get("daysFromUploadingToHiding") is None
sys.exit(0 if ok else 1)' <"$work/out" &&
	ok "lifecycle: hidden versions kept ≥ 7 days, live files never hidden" ||
	bad "lifecycle rule must be: whole bucket, daysFromHidingToDeleting ≥ 7, daysFromUploadingToHiding empty"

# --- 1. restic with this key
prefix=restic-verify-$(date -u +%Y%m%dT%H%M%SZ)
export RESTIC_REPOSITORY="s3:$s3/$B2_BUCKET/$prefix" AWS_ACCESS_KEY_ID=$B2_KEY_ID \
	AWS_SECRET_ACCESS_KEY=$B2_APP_KEY AWS_DEFAULT_REGION=$region
RESTIC_PASSWORD=$(head -c 24 /dev/urandom | base64) # throwaway repository
export RESTIC_PASSWORD
mkdir "$work/data"
RESTIC_MOUNTS=(-v "$work/data:/data:ro")
step() { # step DESCRIPTION restic-args…
	local d=$1
	shift
	if restic_run "$@" >"$work/log" 2>&1; then ok "$d"; else
		bad "$d"
		sed 's/^/     /' "$work/log" | tail -20
	fi
}
locks() {
	local l
	l=$(restic_run list locks 2>"$work/log" | wc -l)
	[ "$l" = 0 ] && ok "no locks left ($1)" || bad "$l lock(s) left ($1)"
}
echo "--- 1. restic ($prefix)"
step "init" init
head -c 3000000 /dev/urandom >"$work/data/a.bin"
step "backup 1" backup --host verify /data
locks "after backup 1"
rm "$work/data/a.bin"
head -c 3000000 /dev/urandom >"$work/data/b.bin" # a.bin's data becomes unused: prune must delete a pack
step "backup 2" backup --host verify /data
locks "after backup 2"
step "forget --keep-last 1 --prune" forget --host verify --keep-last 1 --prune --max-unused 0
locks "after forget --prune"
step "unlock" unlock
locks "after unlock"
step "check --read-data" check --read-data
[ "$(restic_run snapshots --json | json 'len(d)')" = 1 ] && ok "1 snapshot left" || bad "expected 1 snapshot"

# --- 2 and 3. what is in the bucket now
echo "--- 2/3. bucket contents under $prefix/"
code=$(b2 b2_list_file_versions "{\"bucketId\":\"$bucket_id\",\"prefix\":\"$prefix/\",\"maxFileCount\":10000}")
[ "$code" = 200 ] || { echo "b2_list_file_versions: HTTP $code $(cat "$work/out")"; exit 1; }
cp "$work/out" "$work/versions.json"
# Hidden files: the newest version is a "hide" marker. For each, its newest upload.
python3 -I - "$work/versions.json" >"$work/hidden" <<'EOF'
import json, sys
files = json.load(open(sys.argv[1]))["files"]
latest, upload = {}, {}
for f in files:  # sorted by name, newest first
    latest.setdefault(f["fileName"], f["action"])
    if f["action"] == "upload":
        upload.setdefault(f["fileName"], f)
for name, action in latest.items():
    if action == "hide" and name in upload:
        u = upload[name]
        kind = name.split("/")[1]
        print(kind, u["fileId"], u["contentSha1"], name)
EOF
echo "hidden files (deleted by restic): $(cut -d' ' -f1 "$work/hidden" | sort | uniq -c | tr -s ' ' | tr '\n' ',')"
grep -q '^locks ' "$work/hidden" && ok "lock files were hidden, not left behind" || bad "no hidden lock files"
grep -q '^data ' "$work/hidden" && ok "prune hid at least one data pack" || bad "prune hid no data pack"

# 3. Every hidden version is still downloadable and intact.
n=0 good=0
while read -r _ id sha _; do
	n=$((n + 1))
	got=$(download "$id") || got=failed
	[ "$got" = "$sha" ] && good=$((good + 1))
done <"$work/hidden"
[ "$n" -gt 0 ] && [ "$good" = "$n" ] && ok "all $n hidden versions downloadable, SHA-1 matches" || bad "$good of $n hidden versions downloadable"

# 2. Definitive delete of one of those versions: must be refused.
read -r _ id _ name < <(grep '^data ' "$work/hidden" | head -1)
code=$(b2 b2_delete_file_version "{\"fileName\":\"$name\",\"fileId\":\"$id\"}")
echo "b2_delete_file_version → HTTP $code $(json 'd.get("code")' <"$work/out" 2>/dev/null || true)"
[ "$code" = 401 ] && ok "native API: definitive delete refused" || bad "native API: definitive delete NOT refused (HTTP $code)"
code=$(curl -sS -o "$work/out" -w '%{http_code}' --aws-sigv4 "aws:amz:$region:s3" -u "$B2_KEY_ID:$B2_APP_KEY" \
	-X DELETE "$s3/$B2_BUCKET/$name?versionId=$id")
echo "S3 DeleteObject?versionId → HTTP $code $(grep -o '<Code>[^<]*' "$work/out" | cut -c7- || true)"
[ "$code" = 403 ] && ok "S3 API: definitive delete refused" || bad "S3 API: definitive delete NOT refused (HTTP $code)"
got=$(download "$id") || got=failed
grep -q "$id $got " "$work/hidden" && ok "that version is still there afterwards" || bad "that version is gone"

echo
if [ "$fails" = 0 ]; then echo "b2-verify: all checks passed"; else
	echo "b2-verify: $fails check(s) FAILED"
	exit 1
fi
