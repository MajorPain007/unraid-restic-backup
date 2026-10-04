#!/bin/bash

source "$(dirname "$0")/lib.sh"

API="php $ROOT/tests/integration/api.php"
export RB_BROWSE=off
call() { $API "$@"; }
get() { php -r '$v = json_decode($argv[1], true); foreach (explode(".", $argv[2]) as $k) { $v = is_array($v) ? ($v[$k] ?? null) : null; } echo is_scalar($v) ? (is_bool($v) ? ($v ? "true" : "false") : $v) : json_encode($v, JSON_UNESCAPED_SLASHES);' "$1" "$2"; }
okay() { [ "$(get "$1" ok)" = true ]; }
count() { php -r '$v = json_decode($argv[1], true); foreach (explode(".", $argv[2]) as $k) { $v = $v[$k] ?? null; } echo is_array($v) ? count($v) : 0;' "$1" "$2"; }

mkdir -p "$M/src2"
echo "first" > "$M/src2/notes.txt"
echo "odd" > "$M/src2/odd [1]*.txt"
echo "even" > "$M/src2/odd 11.txt"
mkdir -p "$M/src2/photos"; head -c 300000 /dev/urandom > "$M/src2/photos/p1.jpg"

group "Settings and schedules"
r=$(call settings_save "{\"settings\":{\"data_dir\":\"$M/data\",\"start_delay\":0}}")
check "settings are saved" 'okay "$r"' "$r"
r=$(call settings_save '{"settings":{"data_dir":"/tmp/x"}}')
check "a data folder in RAM is refused" '! okay "$r" && get "$r" errors | grep -q "Data folder"' "$r"
r=$(call schedule_preview '{"schedule":{"mode":"cron","cron":"0 3 * * *"}}')
check "a schedule is previewed with its next runs" '[ "$(get "$r" description)" = "Daily at 03:00" ] && [ "$(count "$r" next)" = 5 ]' "$r"
r=$(call schedule_preview '{"schedule":{"mode":"cron","cron":"61 * * * *"}}')
check "a broken schedule says what is wrong" '[ "$(get "$r" valid)" = false ] && get "$r" error | grep -q "minute: 61"' "$r"

group "A local repository"
r=$(call repo_save "{\"repo\":{\"name\":\"Backup disk\",\"type\":\"local\",\"local\":{\"path\":\"$M/disk/restic\"}},\"password_mode\":\"generate\"}")
check "it is created with a generated password, shown once" 'okay "$r" && [ "$(get "$r" generated_password | wc -c)" = 32 ]' "$r"
RID=$(get "$r" repo.id)
export RESTIC_PASSWORD=$(get "$r" generated_password)
mkdir -p "$M/disk"; echo keep > "$M/disk/other-file"
r=$(call repo_test "{\"id\":\"$RID\"}")
check "an empty location is reported as such, ready to initialize" '[ "$(get "$r" status)" = empty ]' "$r"
r=$(call repo_init "{\"id\":\"$RID\"}")
check "it is initialized" 'okay "$r" && [ -f $M/disk/restic/config ]' "$r"
r=$(call repo_test "{\"id\":\"$RID\"}")
check "and then opens" '[ "$(get "$r" status)" = ok ] && [ -n "$(get "$r" repository_id)" ]' "$r"
r=$(call repo_init "{\"id\":\"$RID\"}")
check "initializing it again is refused" '! okay "$r" && get "$r" error | grep -q "already"' "$r"
r=$(call repo_save "{\"repo\":{\"name\":\"Same place\",\"type\":\"local\",\"local\":{\"path\":\"$M/disk/restic\"}},\"password_mode\":\"generate\"}")
check "a second entry for the same location is refused" '! okay "$r" && get "$r" errors | grep -q "already uses"' "$r"
r=$(call repo_password "{\"id\":\"$RID\"}")
check "the password can be shown again, to store it" '[ "$(get "$r" password)" = "$RESTIC_PASSWORD" ]'
r=$(call overview '{}')
check "the browser never gets the password with the configuration" '! echo "$r" | grep -q "$(get "$(call repo_password "{\"id\":\"$RID\"}")" password)"'

group "A job"
r=$(call job_save "{\"job\":{\"name\":\"Nothing\",\"repo\":\"$RID\",\"sources\":[]}}")
check "a job without sources is refused, field by field" '! okay "$r" && get "$r" errors | grep -q "Sources"' "$r"
r=$(call job_save "{\"job\":{\"name\":\"Into itself\",\"repo\":\"$RID\",\"sources\":[\"$M/disk\"]}}")
check "a job backing up its own repository is refused" '! okay "$r" && get "$r" errors | grep -q "contains the repository"' "$r"
r=$(call job_save "{\"job\":{\"name\":\"Files\",\"repo\":\"$RID\",\"sources\":[\"$M/src2\"],\"schedule\":{\"mode\":\"off\"}}}")
check "a job is saved" 'okay "$r"' "$r"
JID=$(get "$r" job.id)
r=$(call job_run "{\"id\":\"$JID\"}")
check "Run now starts it" '[ "$(get "$r" started)" = true ]' "$r"
sleep 0.3
r=$(call job_run "{\"id\":\"$JID\"}")
check "Run now on a running job says so" '! okay "$r" && get "$r" error | grep -q "running already"' "$r"
wait_ops
r=$(call overview '{}')
check "the overview shows the result" '[ "$(get "$r" jobs.0.last.status)" = success ] && [ "$(get "$r" recent.0.status)" = success ]' "$(get "$r" jobs.0.last)"
check "...and the schedule in words" '[ "$(get "$r" jobs.0.schedule)" = "Manual only" ]'

group "Snapshots"
echo "second" > "$M/src2/notes.txt"
call job_run "{\"id\":\"$JID\"}" >/dev/null; wait_ops
r=$(call snapshots "{\"repo\":\"$RID\"}")
check "both snapshots are listed, newest first, with their job" \
    '[ "$(get "$r" snapshots.1.job)" = "$JID" ] && [ "$(get "$r" snapshots.0.time)" -ge "$(get "$r" snapshots.1.time)" ]' "$r"
NEW=$(get "$r" snapshots.0.id); OLD=$(get "$r" snapshots.1.id)
r=$(call ls "{\"repo\":\"$RID\",\"snapshot\":\"$NEW\",\"path\":\"$M/src2\"}")
check "a folder is listed: folders first, without itself" \
    '[ "$(get "$r" entries.0.name)" = photos ] && ! get "$r" entries | grep -q "\"path\":\"$M/src2\""' "$r"
check "...and the listing is cached" '[ -n "$(ls $T/run/cache/ls/$NEW/ 2>/dev/null)" ]'
r=$(call diff "{\"repo\":\"$RID\",\"from\":\"$OLD\",\"to\":\"$NEW\"}")
check "two snapshots are compared" 'echo "$r" | grep -q "notes.txt" && [ "$(get "$r" changes.0.modifier)" = M ]' "$r"
r=$(call find "{\"repo\":\"$RID\",\"pattern\":\"*.JPG\"}")
check "find searches every snapshot, ignoring case" '[ "$(get "$r" results | grep -o p1.jpg | wc -l)" -ge 2 ]' "$r"
r=$(call retention_preview "{\"job\":{\"id\":\"$JID\",\"retention\":{\"last\":1,\"daily\":0,\"weekly\":0,\"monthly\":0}}}")
check "a retention preview shows what would go, and removes nothing" \
    '[ "$(get "$r" remove.0.id)" = "$OLD" ] && [ "$(get "$r" keep.0.id)" = "$NEW" ] && [ "$(snapcount $M/disk/restic)" = 2 ]' "$r"

group "Restore"
r=$(call restore_start "{\"repo\":\"$RID\",\"snapshot\":\"$OLD\",\"items\":[\"$M/src2/notes.txt\",\"$M/src2/photos\"],\"target\":\"path\",\"target_path\":\"$M/restored\"}")
wait_ops
check "items restore straight into the chosen folder" \
    '[ "$(cat $M/restored/notes.txt)" = first ] && [ -f $M/restored/photos/p1.jpg ] && [ ! -e $M/restored/mnt ]' "$r $(op_field error)"
r=$(call restore_start "{\"repo\":\"$RID\",\"snapshot\":\"$NEW\",\"items\":[\"$M/src2/odd [1]*.txt\"],\"target\":\"path\",\"target_path\":\"$M/odd\"}")
wait_ops
check "a name with [ ] * restores just that file" '[ -f "$M/odd/odd [1]*.txt" ] && [ ! -e "$M/odd/odd 11.txt" ]' "$(ls $M/odd 2>&1)"
echo "changed here" > "$M/src2/notes.txt"
call restore_start "{\"repo\":\"$RID\",\"snapshot\":\"$OLD\",\"items\":[\"$M/src2/notes.txt\"],\"target\":\"original\",\"overwrite\":\"never\"}" >/dev/null
wait_ops
check "restoring in place with \"never\" keeps the file that is there" '[ "$(cat $M/src2/notes.txt)" = "changed here" ]'
call restore_start "{\"repo\":\"$RID\",\"snapshot\":\"$OLD\",\"items\":[\"$M/src2/notes.txt\"],\"target\":\"original\",\"overwrite\":\"always\"}" >/dev/null
wait_ops
check "...and with \"always\" puts the snapshot's version back" '[ "$(cat $M/src2/notes.txt)" = first ]'
check "the result counts what was restored" '[ "$(op_field summary.files_restored)" -ge 1 ] && [ "$(op_field status)" = success ]'
r=$(call restore_start "{\"repo\":\"$RID\",\"snapshot\":\"$OLD\",\"items\":[\"$M/src2/notes.txt\"],\"target\":\"path\",\"target_path\":\"/tmp/x\"}")
check "a restore into RAM is refused" '! okay "$r"' "$r"

group "Downloads"
php -r '$_SERVER["REQUEST_METHOD"] = "POST"; $_POST = array("action" => "download", "repo" => $argv[1], "snapshot" => $argv[2], "path" => $argv[3]); include $argv[4];' \
    "$RID" "$NEW" "$M/src2/photos/p1.jpg" "$ROOT/src/include/api.php" > "$T/dl.bin" 2>/dev/null
check "a file downloads as it is" 'cmp -s "$T/dl.bin" "$M/src2/photos/p1.jpg"'
php -r '$_SERVER["REQUEST_METHOD"] = "POST"; $_POST = array("action" => "download", "repo" => $argv[1], "snapshot" => $argv[2], "path" => $argv[3]); include $argv[4];' \
    "$RID" "$NEW" "$M/src2/photos" "$ROOT/src/include/api.php" > "$T/dl.zip" 2>/dev/null
check "a folder downloads as a zip" 'python3 -c "import sys, zipfile; sys.exit(0 if any(n.endswith(\"p1.jpg\") for n in zipfile.ZipFile(sys.argv[1]).namelist()) else 1)" "$T/dl.zip"'

group "Repository care"
r=$(call snapshot_forget "{\"repo\":\"$RID\",\"ids\":[\"$OLD\"]}")
check "a chosen snapshot is removed" 'okay "$r" && [ "$(snapcount $M/disk/restic)" = 1 ]' "$r"
r=$(call snapshots "{\"repo\":\"$RID\"}")
check "...and the list shows it at once" '[ "$(get "$r" snapshots | grep -o "\"id\"" | wc -l)" = 1 ]'
r=$(call repo_stats "{\"id\":\"$RID\"}")
check "stats measure the repository" '[ "$(get "$r" stats.total_size)" -gt 0 ]' "$r"
r=$(call size_history "{\"repo\":\"$RID\"}")
check "the size history has the measurement, and the job's runs with what they backed up" \
    '[ "$(count "$r" repo.points)" = 1 ] && [ "$(get "$r" repo.points.0.1)" -gt 0 ] && [ "$(get "$r" jobs.0.id)" = "$JID" ] &&
     [ "$(count "$r" jobs.0.points)" -ge 2 ] && [ "$(get "$r" jobs.0.points.0.1)" -gt 0 ]' "$r"
call repo_stats "{\"id\":\"$RID\"}" >/dev/null
r=$(call size_history "{\"job\":\"$JID\"}")
check "...one point a day for the repository, however often it is measured" '[ "$(count "$r" repo.points)" = 1 ] && [ "$(count "$r" jobs)" = 1 ]' "$r"
r=$(call repo_keys "{\"id\":\"$RID\"}")
check "its keys are listed" '[ "$(get "$r" keys.0.current)" = true ]' "$r"
r=$(call repo_unlock "{\"id\":\"$RID\"}")
check "unlock runs" 'okay "$r"' "$r"
r=$(call browse "{\"path\":\"$M\"}")
check "folders can be browsed under /mnt" 'echo "$r" | grep -q "\"name\": \"src2\""' "$r"
r=$(call browse '{"path":"/etc"}')
check "...but not elsewhere" '! okay "$r"'
mkdir -p "$M/share"; chown 99:100 "$M/share"; chmod 0777 "$M/share"
r=$(call mkdir "{\"path\":\"$M/share\",\"name\":\"New repo\"}")
check "a new folder can be made" 'okay "$r" && [ -d "$M/share/New repo" ] && [ "$(get "$r" path)" = "$M/share/New repo" ]' "$r"
check "...owned and permitted like its parent" '[ "$(stat -c %u:%g:%a "$M/share/New repo")" = 99:100:777 ]' "$(stat -c %u:%g:%a "$M/share/New repo")"
r=$(call mkdir "{\"path\":\"$M/share\",\"name\":\"New repo\"}")
check "...once" '! okay "$r" && get "$r" error | grep -q "exists already"' "$r"
r=$(call mkdir "{\"path\":\"$M/share\",\"name\":\"a/b\"}")
check "a name with a slash is refused" '! okay "$r" && [ ! -e "$M/share/a" ]' "$r"
r=$(call mkdir '{"path":"/mnt","name":"x"}')
check "nothing is made in /mnt itself" '! okay "$r" && [ ! -e /mnt/x ]' "$r"
r=$(call mkdir '{"path":"/etc","name":"x"}')
check "...nor outside /mnt" '! okay "$r" && [ ! -e /etc/x ]' "$r"
mkdir -p "$M/ram" && mount -t tmpfs -o size=1m tmpfs "$M/ram"
r=$(call mkdir "{\"path\":\"$M/ram\",\"name\":\"x\"}")
check "...nor in a folder that lives in memory" '! okay "$r" && get "$r" error | grep -q "in memory"' "$r"
umount "$M/ram"
r=$(call op_log "{\"id\":\"$(basename "$(last_op)" .json)\"}")
check "an operation's log can be read" 'get "$r" text | grep -q "Finished"' "$r"

group "Stop"
head -c 300000000 /dev/urandom > "$M/src2/huge.bin"
call repo_save "{\"repo\":{\"id\":\"$RID\",\"name\":\"Backup disk\",\"type\":\"local\",\"local\":{\"path\":\"$M/disk/restic\"},\"options\":{\"limit_upload\":20000}},\"password_mode\":\"keep\"}" >/dev/null
mark
call job_run "{\"id\":\"$JID\"}" >/dev/null
wait_phase "Backing up"
sleep 1
r=$(call op_stop "{\"id\":\"$(basename "$(last_op)" .json)\"}")
wait_ops
check "Stop from the page ends a backup" 'okay "$r" && [ "$(op_field status)" = cancelled ]' "$r $(op_field status)"
rm -f "$M/src2/huge.bin"

group "Deleting"
r=$(call repo_delete "{\"id\":\"$RID\"}")
check "a repository a job uses cannot be deleted" '! okay "$r" && get "$r" error | grep -q "Files"' "$r"
call job_delete "{\"id\":\"$JID\"}" >/dev/null
r=$(call repo_delete "{\"id\":\"$RID\"}")
check "without the job it can, and its password goes with it" \
    'okay "$r" && [ ! -e $T/config/secrets/$RID.password ]' "$r"
check "...but the backup data stays where it is" '[ -f $M/disk/restic/config ] && [ -f $M/disk/other-file ]'

group "An SFTP repository, set up as a user would"
useradd -m -s /bin/bash rbtarget
install -d -m 0700 -o rbtarget -g rbtarget /home/rbtarget/.ssh
r=$(call repo_save '{"repo":{"name":"Offsite","type":"sftp","sftp":{"user":"rbtarget","host":"127.0.0.1","port":22,"path":"backups/tower"}},"password_mode":"generate"}')
SID=$(get "$r" repo.id)
check "saving it creates the SSH key and shows its public half" 'get "$r" repo.ssh_public_key | grep -q "^ssh-ed25519 "' "$r"
check "...kept out of Unraid Connect's flash backup" '[ "$(cat $T/config/ssh/.gitignore)" = "*" ] && [ "$(cat $T/config/secrets/.gitignore)" = "*" ]'
get "$r" repo.ssh_public_key > /home/rbtarget/.ssh/authorized_keys
chown rbtarget:rbtarget /home/rbtarget/.ssh/authorized_keys; chmod 600 /home/rbtarget/.ssh/authorized_keys
r=$(call repo_test "{\"id\":\"$SID\"}")
check "the first test asks to confirm the host key, with its fingerprint" \
    '[ "$(get "$r" status)" = hostkey ] && [ "$(get "$r" hostkey.status)" = unknown ] && get "$r" hostkey.scanned | grep -q "SHA256:"' "$r"
FP=$(get "$r" hostkey.scanned | php -r 'echo json_encode(array_column(json_decode(stream_get_contents(STDIN), true), "fingerprint"));')
r=$(call repo_hostkey_trust "{\"id\":\"$SID\",\"fingerprints\":$FP}")
check "confirming stores it" 'okay "$r" && grep -q "^127.0.0.1 ssh-" $T/config/ssh/known_hosts' "$r"
r=$(call repo_hostkey_trust "{\"id\":\"$SID\",\"fingerprints\":[\"SHA256:notthisone\"]}")
check "a fingerprint that does not match is not stored" '! okay "$r"' "$r"
r=$(call repo_test "{\"id\":\"$SID\"}")
check "then the test reaches the server: nothing there yet" '[ "$(get "$r" status)" = empty ]' "$r"
r=$(call repo_init "{\"id\":\"$SID\"}")
check "it initializes on the server" 'okay "$r" && [ -f /home/rbtarget/backups/tower/config ]' "$r"
r=$(call job_save "{\"job\":{\"name\":\"Offsite files\",\"repo\":\"$SID\",\"sources\":[\"$M/src2\"],\"schedule\":{\"mode\":\"off\"}}}")
call job_run "{\"id\":\"$(get "$r" job.id)\"}" >/dev/null; wait_ops
check "and a backup goes there" '[ "$(op_field status)" = success ] && [ "$(op_field repo)" = "$SID" ]' "$(op_field error)"
ssh-keygen -q -t ed25519 -N "" -f "$T/otherhost" && sed -i "s#^127.0.0.1 ssh-ed25519 .*#127.0.0.1 $(cut -d' ' -f1,2 "$T/otherhost.pub")#" "$T/config/ssh/known_hosts"
r=$(call repo_test "{\"id\":\"$SID\"}")
check "a host key that changed is reported as changed, not trusted" '[ "$(get "$r" hostkey.status)" = changed ]' "$r"

printf '\n\033[1mResult:\033[0m %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ]
