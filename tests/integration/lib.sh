set -u
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
PASS=0
FAIL=0
ok()  { printf '  \033[32mPASS\033[0m %s\n' "$1"; PASS=$((PASS + 1)); }
bad() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; [ -n "${2:-}" ] && printf '       %s\n' "$2"; FAIL=$((FAIL + 1)); }
group() { printf '\n\033[1m%s\033[0m\n' "$1"; }
check() { if eval "$2"; then ok "$1"; else bad "$1" "${3:-}"; fi; }

[ "$(id -u)" = 0 ] || { echo "run as root"; exit 2; }
for t in restic rest-server sshd php ssh-keygen; do
    command -v "$t" >/dev/null || [ -x "/usr/sbin/$t" ] || { echo "missing $t"; exit 2; }
done

T=/var/tmp/rbtest
M=/mnt/rbtest
export RB_CONFIG_DIR=$T/config RB_RUN_DIR=$T/run RB_RESTIC=$(command -v restic) RB_VAR_INI=$T/var.ini
export RB_NOTIFY=$T/notify
REST_PID=

cleanup() {
    [ -n "$REST_PID" ] && kill "$REST_PID" 2>/dev/null
    pkill -f "$ROOT/src/scripts/run.php" 2>/dev/null
    pkill -9 -f "src/scripts/browse[.]php" 2>/dev/null
    pkill -9 -f "restic.* mount --no-lock $T" 2>/dev/null
    for m in "$T"/run/browse/*; do
        mountpoint -q "$m" 2>/dev/null && umount -l "$m"
    done
    rm -rf "$T" "$M"
    userdel -r rbtarget >/dev/null 2>&1
}
trap cleanup EXIT
cleanup
mkdir -p "$T/config" "$T/run" "$M/src/docs" "$M/src/sub" "$M/empty"

cat > "$T/var.ini" <<'EOF'
NAME="tower"
mdState="STARTED"
fsState="Started"
startMode="Normal"
mdResync="0"
mdResyncPos="0"
EOF
cat > "$RB_NOTIFY" <<EOF
#!/bin/bash
echo "\$*" >> $T/notifications
EOF
chmod +x "$RB_NOTIFY"

echo hello > "$M/src/docs/a.txt"
head -c 20000000 /dev/urandom > "$M/src/sub/big.bin"
echo junk > "$M/src/sub/x.tmp"

CFG="php $ROOT/tests/integration/cfg.php"
RUN="php $ROOT/src/scripts/run.php"
SCHED="php $ROOT/src/scripts/scheduler.php"
export RESTIC_PASSWORD=test-password

last_op()   { ls -t "$T/run/ops/"*.json 2>/dev/null | head -1; }
op_field()  { php -r '$o = json_decode(file_get_contents($argv[1]), true); $v = $o; foreach (explode(".", $argv[2]) as $k) { $v = $v[$k] ?? null; } echo is_scalar($v) ? $v : json_encode($v);' "$(last_op)" "$1"; }
snapcount() { restic -r "$1" snapshots --json 2>/dev/null | php -r 'echo count(json_decode(stream_get_contents(STDIN), true) ?: []);'; }
wait_ops()  { for _ in $(seq 1 600); do pgrep -f "$ROOT/src/scripts/run.php" >/dev/null || return 0; sleep 0.1; done; return 1; }
mark()       { touch "$T/mark"; sleep 0.05; }
new_op()     { find "$T/run/ops" -name '*.json' -newer "$T/mark" 2>/dev/null | head -1; }
wait_phase() {
    local f
    for _ in $(seq 1 400); do
        f=$(new_op)
        [ -n "$f" ] && [ "$(php -r 'echo json_decode(file_get_contents($argv[1]), true)["phase"] ?? "";' "$f" 2>/dev/null)" = "$1" ] && return 0
        sleep 0.1
    done
    return 1
}
