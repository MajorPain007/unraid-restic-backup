#!/bin/bash
set -u
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
T=/var/tmp/rbdemo
M=/mnt/rbdemo
export RB_CONFIG_DIR=$T/config RB_RUN_DIR=$T/run RB_RESTIC=$(command -v restic) RB_VAR_INI=$T/var.ini RB_NOTIFY=/bin/true
CFG="php $ROOT/tests/integration/cfg.php"

pkill -f "router.php" 2>/dev/null
pkill -f "rest-server --path $T/rest" 2>/dev/null
if [ "${1:-}" = stop ]; then
    rm -rf "$T" "$M"
    exit 0
fi
rm -rf "$T" "$M"
mkdir -p "$T/config" "$T/run" "$M/user/appdata/plex/Library" "$M/user/appdata/nextcloud/config" "$M/user/documents/Taxes 2025" \
         "$M/user/photos/2026/Summer in Italy" "$M/disks/backup"
cat > "$T/var.ini" <<'INI'
NAME="tower"
mdState="STARTED"
fsState="Started"
startMode="Normal"
INI
echo "<?php \$CONFIG = ['dbtype' => 'mysql'];" > "$M/user/appdata/nextcloud/config/config.php"
head -c 3000000 /dev/urandom > "$M/user/appdata/plex/Library/com.plexapp.plugins.library.db"
for i in 1 2 3; do echo "Invoice $i" > "$M/user/documents/Taxes 2025/invoice-$i.pdf"; done
echo "notes" > "$M/user/documents/notes [draft].md"
for i in $(seq 1 12); do head -c 400000 /dev/urandom > "$M/user/photos/2026/Summer in Italy/IMG_$((4000 + i)).jpg"; done

export RESTIC_PASSWORD=demo-password
$CFG settings "{\"data_dir\":\"$M/user/appdata/restic.backup\",\"start_delay\":0}"
$CFG repo "{\"id\":\"r00000001\",\"name\":\"Backup disk\",\"type\":\"local\",\"local\":{\"path\":\"$M/disks/backup/restic\"}}"
$CFG secret r00000001 password demo-password
restic init -r "$M/disks/backup/restic" >/dev/null

id rbtarget >/dev/null 2>&1 || useradd -m -s /bin/bash rbtarget
install -d -m 0700 -o rbtarget -g rbtarget /home/rbtarget/.ssh
rm -rf /home/rbtarget/offsite
$CFG repo '{"id":"r00000002","name":"Offsite (Storage Box)","type":"sftp","sftp":{"user":"rbtarget","host":"127.0.0.1","port":22,"path":"offsite/tower"}}'
$CFG secret r00000002 password demo-password
mkdir -p "$T/config/ssh"
ssh-keygen -q -t ed25519 -N "" -C "restic.backup@tower" -f "$T/config/ssh/r00000002"
install -m 0600 -o rbtarget -g rbtarget "$T/config/ssh/r00000002.pub" /home/rbtarget/.ssh/authorized_keys
ssh-keyscan -T 5 -t ed25519 127.0.0.1 2>/dev/null > "$T/config/ssh/known_hosts"
restic -r sftp:rbtarget@127.0.0.1:offsite/tower -o sftp.args="-i $T/config/ssh/r00000002 -o BatchMode=yes -o UserKnownHostsFile=$T/config/ssh/known_hosts" init >/dev/null

mkdir -p "$T/rest"
(rest-server --path "$T/rest" --listen 127.0.0.1:18001 --no-auth >/dev/null 2>&1 &)
sleep 0.5
$CFG repo '{"id":"r00000003","name":"Family NAS","type":"rest","rest":{"url":"http://127.0.0.1:18001/tower"},"append_only":true}'
$CFG secret r00000003 password demo-password
restic -r rest:http://127.0.0.1:18001/tower init >/dev/null

$CFG job "{\"id\":\"j00000001\",\"name\":\"Appdata\",\"repo\":\"r00000001\",\"sources\":[\"$M/user/appdata\"],\"excludes\":[\"**/Plex Media Server/Cache\"],\"schedule\":{\"mode\":\"cron\",\"cron\":\"0 3 * * *\"}}"
$CFG job "{\"id\":\"j00000002\",\"name\":\"Documents to offsite\",\"repo\":\"r00000002\",\"sources\":[\"$M/user/documents\"],\"schedule\":{\"mode\":\"cron\",\"cron\":\"0 * * * *\"}}"
$CFG job "{\"id\":\"j00000003\",\"name\":\"Photos\",\"repo\":\"r00000003\",\"sources\":[\"$M/user/photos\"],\"schedule\":{\"mode\":\"cron\",\"cron\":\"30 4 * * 0\"}}"
$CFG job "{\"id\":\"j00000004\",\"name\":\"Flash drive\",\"repo\":\"r00000001\",\"enabled\":false,\"sources\":[\"$M/user/documents\"],\"schedule\":{\"mode\":\"off\"}}"

for d in 13 11 9 7 5 3 2 1; do
    when=$(date -d "-$d days 03:00" '+%Y-%m-%d %H:%M:%S')
    echo "change $d" >> "$M/user/appdata/nextcloud/config/config.php"
    restic -r "$M/disks/backup/restic" backup --host tower --tag job:j00000001 --time "$when" "$M/user/appdata" >/dev/null
done
php "$ROOT/src/scripts/run.php" backup j00000001 manual
php "$ROOT/src/scripts/run.php" backup j00000002 manual
php "$ROOT/src/scripts/run.php" backup j00000003 manual
echo "two" > "$M/user/documents/new file.txt"
php "$ROOT/src/scripts/run.php" backup j00000002 manual
php "$ROOT/src/scripts/run.php" check r00000001 manual
kill $(pgrep -f "rest-server --path $T/rest") 2>/dev/null; sleep 0.3
php "$ROOT/src/scripts/run.php" backup j00000003 manual
(rest-server --path "$T/rest" --listen 127.0.0.1:18001 --no-auth >/dev/null 2>&1 &)

php -r '
$f = $argv[1];
$now = time();
$lines = array();
foreach (array("j00000001" => array("Appdata", "r00000001", 86400), "j00000002" => array("Documents to offsite", "r00000002", 3600)) as $id => $j) {
    for ($i = 30; $i >= 1; $i--) {
        $start = $now - $i * $j[2] - 600;
        $status = ($i % 11 === 0) ? "warning" : (($i % 17 === 0) ? "error" : "success");
        $lines[] = json_encode(array("id" => date("Ymd-His", $start) . "-" . substr(md5($id . $i), 0, 4), "kind" => "backup",
            "job" => $id, "job_name" => $j[0], "repo" => $j[1], "repo_name" => "", "trigger" => "schedule",
            "started" => $start, "ended" => $start + 60 + $i * 7, "status" => $status,
            "error" => $status === "error" ? "Cannot reach the server: Connection refused" : ($status === "warning" ? "Some files could not be read" : ""),
            "summary" => array("data_added_packed" => 1048576 * ($i % 7 + 1), "files_new" => $i, "files_changed" => 3), "log" => ""));
    }
}
$old = file_get_contents($f);
file_put_contents($f, implode("\n", $lines) . "\n" . $old);
' "$M/user/appdata/restic.backup/history.jsonl"

cd "$ROOT"
nohup env RB_CONFIG_DIR=$RB_CONFIG_DIR RB_RUN_DIR=$RB_RUN_DIR RB_RESTIC=$RB_RESTIC RB_VAR_INI=$RB_VAR_INI RB_NOTIFY=/bin/true \
    php -S 127.0.0.1:8088 tests/harness/router.php > "$T/server.log" 2>&1 &
sleep 0.5
echo "harness on http://127.0.0.1:8088"
