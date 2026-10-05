#!/bin/bash

source "$(dirname "$0")/lib.sh"

for t in zfs zpool docker unshare; do
    command -v "$t" >/dev/null || { echo "missing $t"; exit 2; }
done
docker image inspect busybox >/dev/null 2>&1 || docker pull -q busybox >/dev/null || { echo "cannot pull busybox"; exit 2; }

POOL=rbtpool
API="php $ROOT/tests/integration/api.php"
get() { php -r '$v = json_decode($argv[1], true); foreach (explode(".", $argv[2]) as $k) { $v = is_array($v) ? ($v[$k] ?? null) : null; } echo is_scalar($v) ? (is_bool($v) ? ($v ? "true" : "false") : $v) : json_encode($v, JSON_UNESCAPED_SLASHES);' "$1" "$2"; }
running() { [ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" = true ]; }

zcleanup() {
    docker rm -f rbt-app rbt-other rbt-files >/dev/null 2>&1
    umount /mnt/user/rbshare 2>/dev/null
    rmdir /mnt/user/rbshare 2>/dev/null
    zpool destroy -f $POOL 2>/dev/null
    rm -f /var/tmp/rbtpool.img
    rmdir /mnt/rbtpool/rbshare /mnt/rbtpool 2>/dev/null
    cleanup
}
trap zcleanup EXIT
zpool destroy -f $POOL 2>/dev/null
truncate -s 1G /var/tmp/rbtpool.img
zpool create -f -m /mnt/rbtpool $POOL /var/tmp/rbtpool.img
zfs create $POOL/rbshare
zfs create $POOL/rbshare/db
echo "A" > "/mnt/rbtpool/rbshare/marker.txt"
echo "child" > "/mnt/rbtpool/rbshare/db/data.db"
head -c 300000000 /dev/urandom > "/mnt/rbtpool/rbshare/big.bin"
mkdir -p /mnt/user/rbshare
mount --bind "/mnt/rbtpool/rbshare" /mnt/user/rbshare

$CFG settings "{\"data_dir\":\"$M/data\",\"start_delay\":0}"
$CFG repo "{\"id\":\"r00000001\",\"name\":\"Disk\",\"type\":\"local\",\"local\":{\"path\":\"$M/repo\"},\"options\":{\"limit_upload\":30000}}"
$CFG secret r00000001 password test-password
restic init -r "$M/repo" >/dev/null

group "Detecting what uses the data"
docker run -d --name rbt-app -v /mnt/user/rbshare/db:/config busybox sleep 100000 >/dev/null
docker run -d --name rbt-other -v $M/elsewhere:/data busybox sleep 100000 >/dev/null
docker run -d --name rbt-files -v /mnt:/mnt busybox sleep 100000 >/dev/null
r=$($API consistency_detect '{"sources":["/mnt/user/rbshare"]}')
check "a container with the share's folder mounted is found, through /mnt/user" \
    '[ "$(get "$r" containers.0.name)" = rbt-app ] && [ "$(get "$r" containers | grep -c rbt-other)" = 0 ]' "$r"
check "a container with all of /mnt mounted is listed apart, not as a user of the data" \
    'get "$r" broad | grep -q rbt-files && ! get "$r" containers | grep -q rbt-files' "$r"
check "ZFS can cover the share: the dataset and its child" \
    '[ "$(get "$r" zfs.ok)" = true ] && get "$r" zfs.datasets | grep -q "rbtpool/rbshare/db"' "$r"
r=$($API consistency_detect "{\"sources\":[\"$M/src\"]}")
check "a folder not on ZFS says so" '[ "$(get "$r" zfs.ok)" = false ] && get "$r" zfs.error | grep -q "not on a ZFS dataset"' "$r"

group "Backing up from a ZFS snapshot"
$CFG job '{"id":"j00000001","name":"Share","repo":"r00000001","sources":["/mnt/user/rbshare"],"consistency":{"mode":"zfs"},"retention":{"enabled":false}}'
mark
$RUN backup j00000001 manual &
wait_phase "Backing up"
sleep 1
echo "B" > "/mnt/rbtpool/rbshare/marker.txt"
wait
check "the backup succeeds" '[ "$(op_field status)" = success ]' "$(op_field error) $(tail -5 $(op_field log))"
check "the snapshot has the share's own path, not a .zfs path" \
    'restic -r $M/repo snapshots --json | grep -q "\"paths\":\[\"/mnt/user/rbshare\"\]" && ! restic -r $M/repo ls latest | grep -q "\.zfs"'
check "it holds the state at the start: the change made during the backup is not in it" \
    '[ "$(restic -r $M/repo dump latest /mnt/user/rbshare/marker.txt)" = A ]'
check "the child dataset is in it" '[ "$(restic -r $M/repo dump latest /mnt/user/rbshare/db/data.db)" = child ]'
check "the ZFS snapshots are gone afterwards" '[ -z "$(zfs list -H -t snapshot -o name -r $POOL | grep restic-backup)" ]'
check "nothing of it is left mounted" '! grep -q "@restic-backup" /proc/self/mountinfo'
$RUN backup j00000001 manual
check "the next run finds its parent: unchanged files are not read again" '[ "$(op_field summary.files_unmodified)" -ge 2 ]' \
    "$(op_field summary)"

group "The data folder inside the snapshotted dataset"
mkdir -p /mnt/rbtpool/rbshare/rbdata
$CFG settings "{\"data_dir\":\"/mnt/rbtpool/rbshare/rbdata\",\"start_delay\":0}"
$CFG job '{"id":"j00000007","name":"Own data","repo":"r00000001","sources":["/mnt/rbtpool/rbshare"],"consistency":{"mode":"zfs"},"retention":{"enabled":false}}'
$RUN backup j00000007 manual
check "restic can still write its cache: the backup succeeds" '[ "$(op_field status)" = success ]' "$(op_field error) $(tail -5 "$(op_field log)")"
check "the cache was written to the live folder, not the snapshot" '[ -n "$(ls /mnt/rbtpool/rbshare/rbdata/cache 2>/dev/null)" ]'
check "the cache is not in the backup" '! restic -r $M/repo ls latest | grep -q "^/mnt/rbtpool/rbshare/rbdata/cache/"'
check "nothing of it is left mounted" '! grep -q "@restic-backup" /proc/self/mountinfo && [ -z "$(zfs list -H -t snapshot -o name -r $POOL | grep restic-backup)" ]'
$CFG settings "{\"data_dir\":\"$M/data\",\"start_delay\":0}"
rm -rf /mnt/rbtpool/rbshare/rbdata

group "ZFS where it can, the rest as it is"
$CFG job "{\"id\":\"j00000006\",\"name\":\"Mixed\",\"repo\":\"r00000001\",\"sources\":[\"/mnt/user/rbshare\",\"$M/src\"],\"consistency\":{\"mode\":\"zfs\"},\"retention\":{\"enabled\":false}}"
r=$($API consistency_detect "{\"sources\":[\"/mnt/user/rbshare\",\"$M/src\"]}")
check "a folder not on ZFS is listed as read as it is, with where it is" \
    '[ "$(get "$r" zfs.ok)" = true ] && [ "$(get "$r" zfs.live.0.path)" = "$M/src" ] && get "$r" zfs.live.0.why | grep -q ext4' "$r"
echo "M1" > /mnt/rbtpool/rbshare/marker.txt
head -c 300000000 /dev/urandom > /mnt/rbtpool/rbshare/big.bin
mark
$RUN backup j00000006 manual &
wait_phase "Backing up"
sleep 1
echo "M2" > /mnt/rbtpool/rbshare/marker.txt
wait
check "the backup succeeds" '[ "$(op_field status)" = success ]' "$(op_field error) $(tail -5 "$(op_field log)")"
check "the share on ZFS comes from the snapshot" '[ "$(restic -r $M/repo dump latest /mnt/user/rbshare/marker.txt)" = M1 ]'
check "...and the other folder is in the snapshot too" 'restic -r $M/repo ls latest | grep -q "^$M/src/docs/a.txt$"'
check "the log says which folders were read as they are" 'grep -q "Not on ZFS, read as they are: $M/src (" "$(op_field log)"'

group "Snapshots no container can hold on to"
snapcount_pool() { zfs list -H -t snapshot -o name -r $POOL | grep -c restic-backup; }
recreate_app() { docker rm -f rbt-app >/dev/null; docker run -d --name rbt-app -v /mnt/rbtpool/rbshare:/data busybox sleep 100000 >/dev/null; }
app_copies() { grep -c "@restic-backup" /proc/$(docker inspect -f '{{.State.Pid}}' rbt-app)/mountinfo; }
share_snap() { zfs list -H -t snapshot -o name -r $POOL/rbshare | grep -m1 "^$POOL/rbshare@restic-backup" | cut -d@ -f2; }

head -c 300000000 /dev/urandom > /mnt/rbtpool/rbshare/big.bin
mark
$RUN backup j00000001 manual &
wait_phase "Backing up"
sleep 1
host_mounts=$(grep -c "@restic-backup" /proc/self/mountinfo)
recreate_app
held=$(app_copies)
wait
check "while restic reads, the snapshot is mounted for restic alone, not for the server" '[ "$host_mounts" = 0 ]'
check "a container recreated meanwhile has no copy of it" '[ "$held" = 0 ]'
check "...and the snapshot is gone afterwards" '[ "$(op_field status)" = success ] && [ "$(snapcount_pool)" = 0 ]' "$(op_field error)"

head -c 300000000 /dev/urandom > /mnt/rbtpool/rbshare/big.bin
mark
$RUN backup j00000001 manual &
RPID=$!
wait_phase "Backing up"
kill -9 $RPID; pkill -9 -f "restic.*backup.*job:j00000001"; wait 2>/dev/null
ls "/mnt/rbtpool/rbshare/.zfs/snapshot/$(share_snap)/" >/dev/null
recreate_app
held=$(app_copies)
$SCHED
check "a container that got a copy of a snapshot holds it" '[ "$held" -ge 1 ]'
check "...the copy is unmounted in the container, and the snapshot goes" \
    '[ "$(snapcount_pool)" = 0 ] && grep -q "Unmounted the copy" "$(op_field log)"' "$(tail -3 "$(op_field log)")"
check "...while the container keeps running, its folder in place" 'running rbt-app && docker exec rbt-app ls /data/marker.txt >/dev/null'

head -c 300000000 /dev/urandom > /mnt/rbtpool/rbshare/big.bin
mark
$RUN backup j00000001 manual &
RUNPID=$!
wait_phase "Backing up"
held_snap="$POOL/rbshare@$(share_snap)"
zfs hold rbtest "$held_snap"
wait $RUNPID
check "a snapshot still busy at the end is reported, not forgotten" \
    '[ "$(op_field status)" = warning ] && op_field error | grep -q "tried again every minute"' "$(op_field status) $(op_field error)"
check "...and left in the journal for the scheduler" \
    '[ "$(php -r '"'"'foreach (glob($argv[1] . "/*.json") as $f) { if (!empty(json_decode(file_get_contents($f), true)["ended"])) echo "yes"; }'"'"' $T/run/journal)" = yes ]'
$SCHED
check "the scheduler leaves it while it is busy" '[ "$(snapcount_pool)" = 1 ]'
zfs release rbtest "$held_snap"
$SCHED
check "once it is free, the next tick removes it and the journal" '[ "$(snapcount_pool)" = 0 ] && [ -z "$(ls $T/run/journal/ 2>/dev/null)" ]'
check "...noted in the run's log, without a notification" \
    'grep -q "Removed the ZFS snapshot that was still busy" "$(op_field log)" && ! grep -q "left behind" $T/notifications 2>/dev/null'
docker rm -f rbt-app >/dev/null
docker run -d --name rbt-app -v /mnt/user/rbshare/db:/config busybox sleep 100000 >/dev/null

group "Stopping containers"
$CFG job '{"id":"j00000002","name":"Stop","repo":"r00000001","sources":["/mnt/user/rbshare"],"consistency":{"mode":"stop","containers":["rbt-app"]},"retention":{"enabled":false}}'
echo "C" > "/mnt/rbtpool/rbshare/marker.txt"
mark
$RUN backup j00000002 manual &
stopped_during=no
wait_phase "Backing up" && { running rbt-app || stopped_during=yes; }
wait
check "the container is stopped while the backup runs" '[ $stopped_during = yes ]'
check "and running again afterwards" 'running rbt-app && [ "$(op_field status)" = success ]' "$(op_field error)"
check "a container that was not in the list kept running" 'running rbt-other && running rbt-files'
check "...and the file manager with /mnt mounted gave no warning" '[ "$(op_field status)" = success ]'

$CFG job '{"id":"j00000005","name":"Stop, then forget","repo":"r00000001","sources":["/mnt/user/rbshare"],"consistency":{"mode":"stop","containers":["rbt-app"]},"retention":{"enabled":true,"last":10}}'
$RUN backup j00000005 manual
check "they start again as soon as restic is done, before the retention policy" \
    '[ "$(grep -n "Starting container rbt-app" "$(op_field log)" | cut -d: -f1)" -lt "$(grep -n "Applying the retention policy" "$(op_field log)" | cut -d: -f1)" ]' \
    "$(grep -E "Starting container|retention policy" "$(op_field log)")"

$CFG job '{"id":"j00000003","name":"Brief","repo":"r00000001","sources":["/mnt/user/rbshare"],"consistency":{"mode":"zfs-stop","containers":["rbt-app"]},"retention":{"enabled":false}}'
echo "D" > "/mnt/rbtpool/rbshare/marker.txt"
mark
$RUN backup j00000003 manual &
up_during=no
wait_phase "Backing up" && { sleep 0.5; running rbt-app && up_during=yes; }
wait
check "with a ZFS snapshot the container is back before restic reads anything" '[ $up_during = yes ]'
check "...and the log shows it was stopped for the snapshot" \
    'grep -q "Stopping containers: rbt-app" $(op_field log) && grep -q "Snapshots taken" $(op_field log)'

$CFG job '{"id":"j00000004","name":"Unlisted","repo":"r00000001","sources":["/mnt/user/rbshare"],"consistency":{"mode":"stop","containers":[]},"retention":{"enabled":false}}'
$RUN backup j00000004 manual
check "a container that uses the data but is not listed makes a warning, and keeps running" \
    '[ "$(op_field status)" = warning ] && op_field error | grep -q "rbt-app" && running rbt-app' "$(op_field status) $(op_field error)"
$CFG job '{"id":"j00000004","name":"Unlisted","repo":"r00000001","sources":["/mnt/user/rbshare"],"consistency":{"mode":"stop","containers":[],"skip":["rbt-app"]},"retention":{"enabled":false}}'
$RUN backup j00000004 manual
check "...unless it was left out on purpose" '[ "$(op_field status)" = success ]' "$(op_field error)"

group "A runner that dies"
mark
$RUN backup j00000002 manual &
RPID=$!
wait_phase "Backing up"
kill -9 $RPID; pkill -9 -f "restic.*backup.*job:j00000002"; wait 2>/dev/null
check "killed mid-backup, it leaves the container stopped" '! running rbt-app'
$SCHED
check "the next scheduler tick starts it again" 'running rbt-app'
check "and says so" 'grep -q "Backup interrupted" $T/notifications'
check "the journal is gone" '[ -z "$(ls $T/run/journal/ 2>/dev/null)" ]'

head -c 300000000 /dev/urandom > /mnt/rbtpool/rbshare/big.bin
mark
$RUN backup j00000001 manual &
RPID=$!
wait_phase "Backing up"
kill -9 $RPID; pkill -9 -f "restic.*backup.*job:j00000001"; wait 2>/dev/null
check "killed with a ZFS snapshot taken, the snapshot stays" '[ -n "$(zfs list -H -t snapshot -o name -r $POOL | grep restic-backup)" ]'
$SCHED
check "and the next tick removes it" '[ -z "$(zfs list -H -t snapshot -o name -r $POOL | grep restic-backup)" ]'

printf '\n\033[1mResult:\033[0m %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ]
