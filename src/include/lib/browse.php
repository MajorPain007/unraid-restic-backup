<?php

require_once __DIR__ . '/ops.php';
require_once __DIR__ . '/restic.php';
require_once __DIR__ . '/consistency.php';

define('RB_BROWSE_DIR', RB_RUN_DIR . '/browse');
define('RB_BROWSE_IDLE', (int)(getenv('RB_BROWSE_IDLE') ?: 600));
define('RB_BROWSE_RETRY', 600);

function rb_browse_files($repoId)
{
    $base = RB_BROWSE_DIR . '/' . $repoId;
    return array('mnt' => $base, 'status' => "$base.json", 'used' => "$base.used", 'stop' => "$base.stop",
                 'lock' => "$base.lock");
}

function rb_browse_available()
{
    return getenv('RB_BROWSE') !== 'off' && file_exists('/dev/fuse');
}

function rb_browse_status($repoId)
{
    $s = rb_json_read(rb_browse_files($repoId)['status']);
    if (!is_array($s)) {
        return null;
    }
    if ($s['status'] === 'error') {
        return time() - (int)($s['ended'] ?? 0) < RB_BROWSE_RETRY ? $s : null;
    }
    if ((int)$s['pid'] === 0) {
        return time() - (int)$s['started'] < 30 ? $s : null;
    }
    return rb_pid_alive($s['pid']) ? $s : null;
}

function rb_browse_running($repoId)
{
    $s = rb_browse_status($repoId);
    return $s !== null && $s['status'] !== 'error';
}

function rb_browse_start(array $repo)
{
    $s = rb_browse_status($repo['id']);
    if ($s !== null) {
        return $s;
    }
    @mkdir(RB_BROWSE_DIR, 0755, true);
    $s = array('status' => 'starting', 'pid' => 0, 'restic' => 0, 'started' => time());
    rb_json_write(rb_browse_files($repo['id'])['status'], $s);
    rb_spawn(array($repo['id']), 'browse.php');
    return $s;
}

function rb_browse_stop($repoId, $wait = 0)
{
    if (!rb_browse_running($repoId)) {
        return;
    }
    @touch(rb_browse_files($repoId)['stop']);
    for ($i = 0; $i < $wait * 5 && rb_browse_running($repoId); $i++) {
        usleep(200000);
    }
}

function rb_browse_repos()
{
    return array_map(function ($f) {
        return basename($f, '.json');
    }, glob(RB_BROWSE_DIR . '/*.json') ?: array());
}

function rb_browse_reap($all = false)
{
    foreach (rb_browse_repos() as $id) {
        $f = rb_browse_files($id);
        $s = rb_json_read($f['status']);
        if (!is_array($s)) {
            continue;
        }
        $alive = rb_pid_alive($s['pid']);
        if (!$all && ($alive || ((int)$s['pid'] === 0 && time() - (int)$s['started'] < 30))) {
            continue;
        }
        foreach (array((int)($s['restic'] ?? 0), $alive ? (int)$s['pid'] : 0) as $pid) {
            if ($pid > 0 && rb_pid_alive($pid)) {
                exec('kill -9 ' . $pid . ' 2>/dev/null');
            }
        }
        rb_browse_unmount($f['mnt']);
        if ($s['status'] !== 'error' || $all || time() - (int)($s['ended'] ?? 0) >= RB_BROWSE_RETRY) {
            @unlink($f['status']);
        }
        @unlink($f['stop']);
    }
}

function rb_browse_mounted($dir)
{
    $want = str_replace(array('\\', ' ', "\t", "\n"), array('\134', '\040', '\011', '\012'), $dir);
    foreach (@file('/proc/self/mountinfo') ?: array() as $line) {
        $f = explode(' ', $line);
        if (($f[4] ?? '') === $want) {
            return true;
        }
    }
    return false;
}

function rb_browse_unmount($dir)
{
    for ($i = 0; $i < 3 && rb_browse_mounted($dir); $i++) {
        rb_cmd(array('umount', '-l', $dir), 15);
    }
}

function rb_browse_list($repoId, $snap, $path)
{
    $f = rb_browse_files($repoId);
    @touch($f['used']);
    $base = realpath($f['mnt'] . '/ids/' . substr($snap, 0, 8));
    if ($base === false || !is_dir($base)) {
        return null;
    }
    $want = $path === '/' ? $base : $base . $path;
    $real = realpath($want);
    if ($real !== $want || !is_dir($real)) {
        return array();
    }
    $names = @scandir($real);
    if ($names === false) {
        return null;
    }
    $entries = array();
    $prefix = $path === '/' ? '' : $path;
    foreach ($names as $name) {
        if ($name === '.' || $name === '..') {
            continue;
        }
        $st = @lstat("$real/$name");
        $mode = $st ? $st['mode'] : 0;
        $type = rb_browse_type($mode);
        $entries[] = array(
            'name'  => $name,
            'type'  => $type,
            'path'  => "$prefix/$name",
            'size'  => $type === 'file' && $st ? $st['size'] : null,
            'mtime' => $st ? $st['mtime'] : null,
            'perm'  => rb_browse_perm($mode),
        );
    }
    return $entries;
}

function rb_browse_type($mode)
{
    $types = array(0040000 => 'dir', 0100000 => 'file', 0120000 => 'symlink', 0060000 => 'dev',
                   0020000 => 'chardev', 0010000 => 'fifo', 0140000 => 'socket');
    return $types[$mode & 0170000] ?? 'irregular';
}

function rb_browse_perm($mode)
{
    $t = $mode & 0170000;
    $s = ($t === 0040000 ? 'd' : '') . ($t === 0120000 ? 'L' : '') . ($t === 0060000 || $t === 0020000 ? 'D' : '')
       . ($t === 0010000 ? 'p' : '') . ($t === 0140000 ? 'S' : '') . ($mode & 04000 ? 'u' : '')
       . ($mode & 02000 ? 'g' : '') . ($t === 0020000 ? 'c' : '') . ($mode & 01000 ? 't' : '');
    $s = $s === '' ? '-' : $s;
    foreach (array(0400, 0200, 0100, 040, 020, 010, 04, 02, 01) as $i => $bit) {
        $s .= $mode & $bit ? 'rwx'[$i % 3] : '-';
    }
    return $s;
}
