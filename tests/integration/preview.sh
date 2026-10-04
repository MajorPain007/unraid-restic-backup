#!/bin/bash

source "$(dirname "$0")/lib.sh"

API="php $ROOT/tests/integration/api.php"
get() { php -r '$v = json_decode($argv[1], true); foreach (explode(".", $argv[2]) as $k) { $v = is_array($v) ? ($v[$k] ?? null) : null; } echo is_scalar($v) ? (is_bool($v) ? ($v ? "true" : "false") : $v) : json_encode($v, JSON_UNESCAPED_SLASHES);' "$1" "$2"; }
has() { get "$r" preview | grep -qF "\"$S/$1\""; }
rule() { php -r 'foreach (json_decode($argv[1], true)["rules"] as $r) { if ($r["label"] === $argv[2]) { echo $r[$argv[3]]; } }' "$r" "$1" "$2"; }

S="$M/pv"
mkdir -p "$S/docs" "$S/cache" "$S/sub/cache" "$S/keep" "$S/Plex Media Server/Cache" "$S/Plex Media Server/Library" \
         "$S/\$RECYCLE.BIN" "$S/.BIN" "$S/[brackets]" "$S/space dir" "$S/deep/a/b/c" "$S/deep2/a" "$S/cached/inner" \
         "$S/empty" "$S/logs" "$S/Photos"
for f in docs/a.txt docs/b.tmp docs/c.log docs/important.log Photos/d.JPG Photos/e.jpg Photos/f.png cache/x sub/cache/y \
         keep/cache.txt "Plex Media Server/Cache/z" "Plex Media Server/Library/l" "\$RECYCLE.BIN/r" .BIN/k "[brackets]/f" \
         "space dir/f g.txt" deep/a/b/c/d.txt deep2/a/x.txt cached/data.bin cached/inner/i.txt logs/app.log; do
    echo "$f" > "$S/$f"
done
printf 'Signature: 8a477f597d28d172789f06886806bc55\n# a cache\n' > "$S/cached/CACHEDIR.TAG"
head -c 2097152 /dev/urandom > "$S/big.bin"
head -c 1000 /dev/urandom > "$S/small.bin"
ln -s docs/a.txt "$S/link"
restic init -r "$M/pvrepo" >/dev/null
$CFG settings "{\"data_dir\":\"$M/data\"}"

JOB=$(php -r 'echo json_encode(array("id" => "j00000009", "name" => "Preview", "repo" => "r00000001", "sources" => array($argv[1]),
    "excludes" => array("*.tmp", "**/cache", "deep/a", "**/Plex Media Server/Cache/**", "**/\$RECYCLE.BIN", "*.log",
                        "!important.log", $argv[1] . "/space dir", "nothing-matches-this"),
    "iexcludes" => array("*.JPG"), "exclude_caches" => true, "exclude_larger_than" => "1M"), JSON_UNESCAPED_SLASHES);' "$S")

group "The preview counts what restic would back up"
r=$(php "$ROOT/tests/integration/preview_compare.php" "$JOB" "$M/pvrepo")
check "restic's dry run worked" '[ "$(get "$r" restic_exit)" = 0 ] && [ "$(get "$r" restic)" != "[]" ]' "$r"
check "the same files, one by one" '[ "$(get "$r" preview)" = "$(get "$r" restic)" ]' \
    "$(diff <(get "$r" preview | tr , "\n") <(get "$r" restic | tr , "\n") | grep "^[<>]")"
check "the same size and number of files as restic's own summary" \
    '[ "$(get "$r" preview_bytes)" = "$(get "$r" restic_bytes)" ] && [ "$(get "$r" preview_count)" = "$(get "$r" restic_count)" ]' \
    "bytes $(get "$r" preview_bytes) / $(get "$r" restic_bytes), files $(get "$r" preview_count) / $(get "$r" restic_count)"
check "...and the rules did their part" \
    'has docs/important.log && ! has docs/c.log && has .BIN/k && ! has "\$RECYCLE.BIN/r" && has keep/cache.txt && ! has sub/cache/y &&
     has deep2/a/x.txt && ! has deep/a/b/c/d.txt && has small.bin && ! has big.bin && has cached/CACHEDIR.TAG && ! has cached/data.bin &&
     has Photos/f.png && ! has Photos/d.JPG && has "[brackets]/f" && ! has "space dir/f g.txt"' "$(get "$r" preview)"
check "a pattern that matches nothing is listed as such" '[ "$(rule nothing-matches-this files)" = 0 ] && [ "$(rule nothing-matches-this dirs)" = 0 ]'
check "what a rule leaves out is measured, folders included" \
    '[ "$(rule "**/Plex Media Server/Cache/**" dirs)" = 1 ] && [ "$(rule "**/Plex Media Server/Cache/**" files)" = 1 ]'

group "A preview from the page"
cmp=$r
r=$($API job_preview "{\"job\":$JOB}")
id=$(get "$r" id)
check "it starts in the background" '[ -n "$id" ]' "$r"
for _ in $(seq 1 100); do
    s=$($API job_preview_status "{\"id\":\"$id\"}")
    [ "$(get "$s" status)" = done ] && break
    sleep 0.2
done
check "it finishes, with the same numbers" \
    '[ "$(get "$s" status)" = done ] && [ "$(get "$s" result.total.files)" = "$(get "$cmp" restic | tr , "\n" | grep -c .)" ]' "$(get "$s" result.total)"
check "its folder tree has the source with the largest folder first" \
    '[ "$(get "$s" result.sources.0.tree.name)" = pv ] && [ "$(get "$s" result.sources.0.tree.children.0.name)" = Photos ] || [ "$(get "$s" result.sources.0.tree.children.0.bytes)" -ge "$(get "$s" result.sources.0.tree.children.1.bytes)" ]' "$(get "$s" result.sources)"
r=$($API job_preview '{"job":{"name":"x","sources":[]}}')
check "a job without folders gets no preview" '[ "$(get "$r" ok)" = false ]' "$r"

group "Folder by folder, the same verdicts"
tree=$(php -r '
require $argv[1];
$job = $argv[2];
$walk = function ($path) use (&$walk, $job) {
    $r = rb_api_dispatch("job_tree", array("job" => $job, "path" => $path));
    foreach ($r["entries"] ?? array() as $e) {
        if ($e["open"]) {
            $walk($e["path"]);
        } elseif ($e["type"] === "file" && $e["out"] === null) {
            echo $e["path"], "\n";
        }
    }
};
$walk("");' "$ROOT/src/include/lib/api.php" "$JOB" | sort)
want=$(php -r 'foreach (json_decode($argv[1], true) as $f) { echo $f, "\n"; }' "$(get "$cmp" restic)" | sort)
check "the files marked as backed up are the files restic reads" '[ -n "$tree" ] && [ "$tree" = "$want" ]' \
    "$(diff <(echo "$want") <(echo "$tree") | grep "^[<>]")"
r=$($API job_tree "{\"job\":$JOB,\"path\":\"$S/docs\"}")
check "each entry says which rule leaves it out" \
    '[ "$(php -r '"'"'foreach (json_decode($argv[1], true)["entries"] as $e) { if ($e["name"] === "c.log") echo $e["out"]["label"]; }'"'"' "$r")" = "*.log" ]' "$r"
r=$($API job_tree "{\"job\":$JOB,\"path\":\"$S/sub/cache\"}")
check "...and inside a folder left out, which folder it is" \
    '[ "$(get "$r" entries.0.out.via)" = "$S/sub/cache" ]' "$r"
r=$($API job_tree "{\"job\":$JOB,\"path\":\"$S/../../../etc\"}")
check "folders outside the job are not shown" '[ "$(get "$r" ok)" = false ]' "$r"
mkdir -p "$S/.zfs/snapshot/x"
r=$($API job_tree "{\"job\":$JOB,\"path\":\"$S/.zfs/snapshot\"}")
check "nor ZFS snapshot folders" '[ "$(get "$r" ok)" = false ] && get "$r" error | grep -q ZFS' "$r"
rm -rf "$S/.zfs"

printf '\n\033[1mResult:\033[0m %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ]
