#!/bin/bash

source "$(dirname "$0")/lib.sh"

$CFG settings "{\"data_dir\":\"$M/data\",\"start_delay\":0}"
$CFG repo "{\"id\":\"r00000001\",\"name\":\"Disk\",\"type\":\"local\",\"local\":{\"path\":\"$M/repo\"}}"
$CFG secret r00000001 password test-password
restic init -r "$M/repo" >/dev/null
$CFG job "{\"id\":\"j00000001\",\"name\":\"Documents\",\"repo\":\"r00000001\",\"sources\":[\"$M/src\"],\"excludes\":[\"*.tmp\"],\"retention\":{\"last\":2,\"daily\":0,\"weekly\":0,\"monthly\":0}}" \
    || { echo "could not set up the job"; exit 1; }

group "Backup to a local repository"
$RUN backup j00000001 manual
check "it succeeds" '[ "$(op_field status)" = success ]' "$(op_field error)"
check "a snapshot is made, tagged with the job" \
    'restic -r $M/repo snapshots --json | grep -q "\"job:j00000001\""'
check "the host is the server name" 'restic -r $M/repo snapshots --json | grep -q "\"hostname\":\"tower\""'
check "excluded files stay out" '! restic -r $M/repo ls latest | grep -q "x.tmp"'
check "history, state and log are in the data folder" \
    '[ -s $M/data/history.jsonl ] && grep -q "\"j00000001\"" $M/data/state.json && ls $M/data/logs/*.log >/dev/null 2>&1'
check "restic's cache is in the data folder, not in RAM" '[ -d $M/data/cache ] && [ ! -d /root/.cache/restic ] || [ -d $M/data/cache ]'

$RUN backup j00000001 manual
check "the second run finds its parent: nothing read again" '[ "$(op_field summary.files_unmodified)" = 2 ]' \
    "files_unmodified=$(op_field summary.files_unmodified)"
mkdir -p "$M/src2"; echo b > "$M/src2/b.txt"
$CFG job "{\"id\":\"j00000001\",\"name\":\"Documents\",\"repo\":\"r00000001\",\"sources\":[\"$M/src\",\"$M/src2\"],\"excludes\":[\"*.tmp\"],\"retention\":{\"last\":2,\"daily\":0,\"weekly\":0,\"monthly\":0}}"
$RUN backup j00000001 manual
check "a source added later still has a parent" '[ "$(op_field summary.files_unmodified)" = 2 ]' \
    "files_unmodified=$(op_field summary.files_unmodified)"
check "retention keeps the last 2" '[ "$(snapcount $M/repo)" = 2 ]' "$(snapcount $M/repo) snapshots"
check "the result says what retention removed" '[ "$(op_field summary.forget_removed)" = 1 ]'

group "What must not be backed up"
before=$(snapcount "$M/repo")
$CFG job "{\"id\":\"j00000002\",\"name\":\"Empty\",\"repo\":\"r00000001\",\"sources\":[\"$M/empty\"]}"
$RUN backup j00000002 manual
check "an empty source - a disk that is not mounted - is refused" \
    '[ "$(op_field status)" = error ] && op_field error | grep -q "is empty"' "$(op_field error)"
$CFG job "{\"id\":\"j00000002\",\"name\":\"Gone\",\"repo\":\"r00000001\",\"sources\":[\"$M/nothere\"]}"
$RUN backup j00000002 manual
check "a missing source is refused" 'op_field error | grep -q "does not exist"'
check "neither made a snapshot" '[ "$(snapcount $M/repo)" = "$before" ]'
check "and both notified" '[ "$(grep -c "Backup \"" $T/notifications)" -ge 2 ] && grep -q "alert" $T/notifications'

group "Repository problems are named"
$CFG repo "{\"id\":\"r00000002\",\"name\":\"Unmounted\",\"type\":\"local\",\"local\":{\"path\":\"$M/disk/repo\"}}"
$CFG secret r00000002 password test-password
$CFG job "{\"id\":\"j00000003\",\"name\":\"To nowhere\",\"repo\":\"r00000002\",\"sources\":[\"$M/src\"]}"
$RUN backup j00000003 manual
check "no repository at a local path: refused, and nothing is created there" \
    'op_field error | grep -q "is the disk mounted" && [ ! -e $M/disk ]' "$(op_field error)"
$CFG secret r00000001 password wrong
$RUN backup j00000001 manual
check "a wrong password says so" 'op_field error | grep -q "Wrong password"' "$(op_field error)"
$CFG secret r00000001 password test-password

group "Stop, locks and queueing"
head -c 400000000 /dev/urandom > "$M/src/sub/huge.bin"
$CFG repo "{\"id\":\"r00000001\",\"name\":\"Disk\",\"type\":\"local\",\"local\":{\"path\":\"$M/repo\"},\"options\":{\"limit_upload\":20000}}"
before=$(snapcount "$M/repo")
mark
$RUN backup j00000001 manual &
wait_phase "Backing up"
sleep 1
$CFG job "{\"id\":\"j00000006\",\"name\":\"Docs only\",\"repo\":\"r00000001\",\"sources\":[\"$M/src/docs\"]}"
$RUN backup j00000006 manual; rc=$?
check "another job on the busy repository waits (exit 75) and is queued" \
    '[ $rc = 75 ] && php -r "exit(empty(json_decode(file_get_contents(\$argv[1]), true)[\"jobs\"][\"j00000006\"][\"queued\"]) ? 1 : 0);" $M/data/state.json' "rc=$rc"
check "...without recording anything as an operation" '[ "$(op_field status)" = running ]'
op=$(basename "$(last_op)" .json)
touch "$T/run/ops/$op.cancel"
wait
check "Stop ends the backup" '[ "$(op_field status)" = cancelled ]' "$(op_field status) $(op_field error)"
check "no snapshot is left from it, and no lock" \
    '[ "$(snapcount $M/repo)" = "$before" ] && [ -z "$(restic -r $M/repo list locks --no-lock 2>/dev/null)" ]'
rm -f "$M/src/sub/huge.bin"
$CFG repo "{\"id\":\"r00000001\",\"name\":\"Disk\",\"type\":\"local\",\"local\":{\"path\":\"$M/repo\"}}"

restic -r "$M/repo" check --read-data >/dev/null 2>&1 &
sleep 0.5; kill -9 $! 2>/dev/null; wait 2>/dev/null
$RUN backup j00000001 manual
check "a stale lock is removed and the backup goes ahead" '[ "$(op_field status)" = success ]' "$(op_field error)"

group "The scheduler"
$CFG job "{\"id\":\"j00000001\",\"name\":\"Documents\",\"repo\":\"r00000001\",\"sources\":[\"$M/src\"],\"schedule\":{\"mode\":\"cron\",\"cron\":\"* * * * *\"},\"retention\":{\"last\":2,\"daily\":0,\"weekly\":0,\"monthly\":0}}"
n0=$(ls "$T/run/ops/" | grep -c json)
$SCHED; wait_ops
check "the queued job starts once the repository is free" \
    '[ "$(ls $T/run/ops/ | grep -c json)" -gt "$n0" ] && [ "$(op_field job)" = j00000006 ] && [ "$(op_field status)" = success ]'
check "and the queue mark is cleared" '! grep -q "\"queued\"" $M/data/state.json'
n1=$(ls "$T/run/ops/" | grep -c json)
php -r '$f=$argv[1]; $s=json_decode(file_get_contents($f), true); $s["jobs"]["j00000001"]["sched"]["slot"] -= 3600; file_put_contents($f, json_encode($s));' "$M/data/state.json"
$SCHED; wait_ops
check "a due slot starts a run" '[ "$(ls $T/run/ops/ | grep -c json)" -gt "$n1" ] && [ "$(op_field trigger)" = schedule ]'
n2=$(ls "$T/run/ops/" | grep -c json)
$SCHED; wait_ops
check "the same slot does not start a second run" '[ "$(ls $T/run/ops/ | grep -c json)" = "$n2" ]'
sed -i 's/fsState="Started"/fsState="Stopped"/' "$T/var.ini"
php -r '$f=$argv[1]; $s=json_decode(file_get_contents($f), true); $s["jobs"]["j00000001"]["sched"]["slot"] -= 3600; file_put_contents($f, json_encode($s));' "$M/data/state.json"
$SCHED; wait_ops
check "nothing starts while the array is stopped" '[ "$(ls $T/run/ops/ | grep -c json)" = "$n2" ]'
sed -i 's/fsState="Stopped"/fsState="Started"/' "$T/var.ini"
$SCHED; wait_ops
check "...and the missed slot runs once it is started" '[ "$(ls $T/run/ops/ | grep -c json)" -gt "$n2" ]'

group "Maintenance"
$RUN prune r00000001 manual
check "prune succeeds and records the repository size" \
    '[ "$(op_field status)" = success ] && grep -q "total_size" $M/data/state.json' "$(op_field error)"
$RUN check r00000001 manual
check "check succeeds, reading a share of the data" \
    '[ "$(op_field status)" = success ] && grep -q -- "--read-data-subset" $(op_field log)' "$(op_field error)"

group "SFTP"
useradd -m -s /bin/bash rbtarget
install -d -m 0700 -o rbtarget -g rbtarget /home/rbtarget/.ssh
$CFG repo '{"id":"r00000003","name":"Offsite","type":"sftp","sftp":{"user":"rbtarget","host":"127.0.0.1","port":22,"path":"restic/tower"}}'
$CFG secret r00000003 password test-password
mkdir -p "$T/config/ssh"
ssh-keygen -q -t ed25519 -N "" -f "$T/config/ssh/r00000003"
install -m 0600 -o rbtarget -g rbtarget "$T/config/ssh/r00000003.pub" /home/rbtarget/.ssh/authorized_keys
SFTP_ARGS="-i $T/config/ssh/r00000003 -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
restic -r sftp:rbtarget@127.0.0.1:restic/tower -o sftp.args="$SFTP_ARGS" init >/dev/null 2>&1
$CFG job "{\"id\":\"j00000004\",\"name\":\"Offsite\",\"repo\":\"r00000003\",\"sources\":[\"$M/src\"]}"
$RUN backup j00000004 manual
check "an unconfirmed host key stops the backup, and says what to do" \
    '[ "$(op_field status)" = error ] && op_field error | grep -q "host key"' "$(op_field error)"
ssh-keyscan -T 5 -t ed25519 127.0.0.1 2>/dev/null > "$T/config/ssh/known_hosts"
$RUN backup j00000004 manual
check "with the host key confirmed it succeeds" '[ "$(op_field status)" = success ]' "$(op_field error)"
check "into the SSH user's home, as a relative path says" '[ -f /home/rbtarget/restic/tower/config ]'
: > /home/rbtarget/.ssh/authorized_keys
$RUN backup j00000004 manual
check "a refused key is named as such" 'op_field error | grep -q "refused the key"' "$(op_field error)"

group "REST server"
mkdir -p "$T/rest"
rest-server --path "$T/rest" --listen 127.0.0.1:18000 --no-auth >/dev/null 2>&1 &
REST_PID=$!
sleep 0.5
$CFG repo '{"id":"r00000004","name":"REST","type":"rest","rest":{"url":"http://127.0.0.1:18000/tower"}}'
$CFG secret r00000004 password test-password
restic -r rest:http://127.0.0.1:18000/tower init >/dev/null 2>&1
$CFG job "{\"id\":\"j00000005\",\"name\":\"REST\",\"repo\":\"r00000004\",\"sources\":[\"$M/src\"]}"
$RUN backup j00000005 manual
check "a backup to a REST server succeeds" '[ "$(op_field status)" = success ]' "$(op_field error)"

printf '\n\033[1mResult:\033[0m %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ]
