#!/bin/bash

source "$(dirname "$0")/lib.sh"

API="php $ROOT/tests/integration/api.php"
call() { $API "$@"; }
get() { php -r '$v = json_decode($argv[1], true); foreach (explode(".", $argv[2]) as $k) { $v = is_array($v) ? ($v[$k] ?? null) : null; } echo is_scalar($v) ? (is_bool($v) ? ($v ? "true" : "false") : $v) : json_encode($v, JSON_UNESCAPED_SLASHES);' "$1" "$2"; }
LIB="$ROOT/src/include/lib/browse.php"
mounted() { php -r 'require $argv[1]; exit(rb_browse_mounted($argv[2]) ? 0 : 1);' "$LIB" "$T/run/browse/$RID"; }
running() { php -r 'require $argv[1]; exit(rb_browse_running($argv[2]) ? 0 : 1);' "$LIB" "$RID"; }
status()  { get "$(cat "$T/run/browse/$RID.json" 2>/dev/null || echo '{}')" "$1"; }
until_()  { for _ in $(seq 1 150); do eval "$1" && return 0; sleep 0.2; done; return 1; }
list() {
    local r
    for _ in $(seq 1 150); do
        r=$(call ls "{\"repo\":\"$RID\",\"snapshot\":\"$1\",\"path\":\"$2\"}")
        [ "$(get "$r" pending)" = true ] || { echo "$r"; return; }
        sleep 0.2
    done
    echo "$r"
}
entries() { get "$(list "$1" "$2")" entries; }
nocache() { rm -rf "$T/run/cache/ls"; }

S="$M/br"
mkdir -p "$S/dir/sub" "$S/empty" "$S/sticky" "$S/odd name ü"
echo a > "$S/dir/a.txt"
head -c 5000 /dev/urandom > "$S/dir/b.bin"
echo x > "$S/odd name ü/f [1].txt"
echo deep > "$S/dir/sub/deep.txt"
ln -s /etc "$S/etclink"
ln -s dir "$S/rellink"
ln -s dir/a.txt "$S/filelink"
mkfifo "$S/fifo"
echo s > "$S/suid"
chmod 4755 "$S/suid"
chmod 1777 "$S/sticky"
chmod 0640 "$S/dir/a.txt"
touch -d '2020-01-02 03:04:05' "$S/dir/a.txt"

$CFG settings "{\"data_dir\":\"$M/data\"}"
r=$(call repo_save "{\"repo\":{\"name\":\"Disk\",\"type\":\"local\",\"local\":{\"path\":\"$M/repo\"}},\"password_mode\":\"generate\"}")
RID=$(get "$r" repo.id)
export RESTIC_PASSWORD=$(get "$r" generated_password)
call repo_init "{\"id\":\"$RID\"}" >/dev/null
r=$(call job_save "{\"job\":{\"name\":\"Browse\",\"repo\":\"$RID\",\"sources\":[\"$S\"],\"schedule\":{\"mode\":\"off\"}}}")
JID=$(get "$r" job.id)
$RUN backup "$JID" manual >/dev/null 2>&1
SNAP=$(get "$(call snapshots "{\"repo\":\"$RID\",\"refresh\":true}")" snapshots.0.id)
PATHS=("/" "$M" "$S" "$S/dir" "$S/dir/sub" "$S/odd name ü" "$S/sticky" "$S/empty" "$S/etclink" "$S/etclink/ssh" "$S/rellink" "$S/dir/a.txt" "$S/nothing")

group "Folders read from the mount are the folders restic ls gives"
declare -A OLD
for p in "${PATHS[@]}"; do
    OLD[$p]=$(RB_BROWSE=off entries "$SNAP" "$p")
done
check "restic ls listed the test folders" '[ -n "$SNAP" ] && echo "${OLD[$S]}" | grep -q fifo && echo "${OLD[$S/dir]}" | grep -q a.txt' "${OLD[$S]}"
nocache
r=$(call ls "{\"repo\":\"$RID\",\"snapshot\":\"$SNAP\",\"path\":\"$S\"}")
check "the first folder waits for the mount to start" '[ "$(get "$r" pending)" = true ]' "$r"
check "...which then is ready" 'until_ "[ \"\$(status status)\" = ready ]" && mounted' "$(cat "$T/run/browse/$RID.json" 2>&1)"
same=1
diffs=""
for p in "${PATHS[@]}"; do
    new=$(RB_RESTIC=/nonexistent entries "$SNAP" "$p")
    if [ "$new" != "${OLD[$p]}" ]; then
        same=0
        diffs+="$p: restic ${OLD[$p]} - mount $new"$'\n'
    fi
done
check "every folder the same: names, types, sizes, times, permissions" '[ $same = 1 ]' "$diffs"
check "links are not followed: the server's /etc does not show" \
    '! echo "$(entries "$SNAP" "$S/etclink")$(entries "$SNAP" "$S/etclink/ssh")" | grep -q passwd'
nocache
r=$(call ls "{\"repo\":\"$RID\",\"snapshot\":\"$SNAP\",\"path\":\"$S/../../../etc\"}")
check "nor is .. on the way" '[ "$(get "$r" entries)" = "[]" ]' "$r"
nocache
start=$(date +%s%N)
r=$(RB_RESTIC=/nonexistent call ls "{\"repo\":\"$RID\",\"snapshot\":\"$SNAP\",\"path\":\"$S/dir/sub\"}")
ms=$(( ($(date +%s%N) - start) / 1000000 ))
check "a folder not read before comes at once" '[ "$(get "$r" entries.0.name)" = deep.txt ] && [ $ms -lt 1500 ]' "$ms ms: $r"

group "The mount ends when it should"
pid=$(status pid)
$RUN backup "$JID" manual >/dev/null 2>&1
check "a run on the repository ends it, and its mount" 'until_ "! running" && ! mounted && ! kill -0 $pid 2>/dev/null' "$(cat "$T/run/browse/$RID.json" 2>&1)"
NEWSNAP=$(get "$(call snapshots "{\"repo\":\"$RID\",\"refresh\":true}")" snapshots.0.id)
check "the next folder starts it again, with the new snapshot in it" \
    '[ "$NEWSNAP" != "$SNAP" ] && [ "$(entries "$NEWSNAP" "$S/dir/sub" | grep -c deep.txt)" = 1 ] && [ "$(status status)" = ready ]'

php "$ROOT/src/scripts/stop_all.php" 10
check "stopping everything ends it" '! running && ! mounted && [ ! -e "$T/run/browse/$RID.json" ]'

nocache
entries "$SNAP" "$S" >/dev/null
restic_pid=$(status restic)
kill -9 "$(status pid)"
check "a killed server leaves restic and the mount behind..." 'kill -0 $restic_pid 2>/dev/null && mounted'
php -r 'require $argv[1]; rb_browse_reap();' "$LIB"
check "...which the scheduler cleans up" 'until_ "! kill -0 $restic_pid 2>/dev/null" && ! mounted && [ ! -e "$T/run/browse/$RID.json" ]'

nocache
RB_BROWSE_IDLE=2 entries "$SNAP" "$S" >/dev/null
check "unused, it ends by itself" 'running && until_ "! running" && ! mounted'

group "Where the mount fails, folders come from restic ls"
nocache
rm -rf "$T/run/browse/$RID"
echo "not a folder" > "$T/run/browse/$RID"
new=$(entries "$SNAP" "$S")
check "the listing still comes" '[ "$new" = "${OLD[$S]}" ]' "$new"
check "and the failure is remembered, not tried with every folder" \
    '[ "$(status status)" = error ] && [ -n "$(status error)" ]' "$(cat "$T/run/browse/$RID.json")"
rm -f "$T/run/browse/$RID"

printf '\n\033[1mResult:\033[0m %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ]
