<?php

if (PHP_SAPI !== 'cli') {
    exit(1);
}

require_once dirname(__DIR__) . '/include/lib/browse.php';

rb_use_local_timezone();

$cfg = rb_config_load();
$repo = rb_repo($cfg, $argv[1] ?? '');
if (!$repo) {
    exit(2);
}
$f = rb_browse_files($repo['id']);
@mkdir(RB_BROWSE_DIR, 0755, true);
$lock = @fopen($f['lock'], 'c');
if (!$lock || !flock($lock, LOCK_EX | LOCK_NB)) {
    exit(0);
}
@unlink($f['stop']);
rb_browse_unmount($f['mnt']);
@mkdir($f['mnt'], 0755);

$status = array('status' => 'starting', 'pid' => getmypid(), 'restic' => 0, 'started' => time());
$write = function () use (&$status, $f) {
    rb_json_write($f['status'], $status);
};
$write();
$restic = 0;
register_shutdown_function(function () use (&$status, &$restic, $write, $f) {
    if ($status['status'] === 'starting' || $status['status'] === 'ready') {
        if ($restic > 0 && rb_pid_alive($restic)) {
            exec('kill -9 ' . $restic . ' 2>/dev/null');
        }
        rb_browse_unmount($f['mnt']);
        $e = error_get_last();
        $status = array('status' => 'error', 'pid' => 0, 'ended' => time(),
                        'error' => 'Browsing stopped unexpectedly' . ($e ? ': ' . $e['message'] : '.'));
        $write();
    }
});

$err = array();
$stopping = 0;
$why = '';
$code = rb_proc_stream(
    rb_restic_cmd($repo, $cfg['settings'], array('mount', '--no-lock', $f['mnt']), false),
    rb_restic_env($repo, $cfg['settings']),
    function ($stream, $line) use (&$err) {
        if ($stream === 'err' && count($err) < 200) {
            $err[] = $line;
        }
    },
    function () use (&$status, &$restic, &$stopping, &$why, $write, $f) {
        clearstatcache();
        if ($stopping) {
            if (time() - $stopping > 15 && $restic > 0 && rb_pid_alive($restic)) {
                exec('kill -9 ' . $restic . ' 2>/dev/null');
            }
            return false;
        }
        if ($status['status'] === 'starting' && rb_browse_mounted($f['mnt'])) {
            $status['status'] = 'ready';
            $status['ready'] = time();
            $write();
            @touch($f['used']);
        }
        if (file_exists($f['stop'])) {
            $why = 'stop';
        } elseif ($status['status'] === 'ready' && time() - (int)@filemtime($f['used']) > RB_BROWSE_IDLE) {
            $why = 'idle';
        } elseif ($status['status'] === 'starting' && time() - $status['started'] > 900) {
            $why = 'Opening the repository took longer than 15 minutes.';
        } else {
            return false;
        }
        $stopping = time();
        return true;
    },
    function ($pid) use (&$status, &$restic, $write) {
        $restic = $pid;
        $status['restic'] = $pid;
        $write();
    });

rb_browse_unmount($f['mnt']);
@unlink($f['stop']);
if ($why === '' || ($why !== 'stop' && $why !== 'idle')) {
    $message = $why !== '' ? $why : ($code === 0 ? '' : rb_restic_error($err, $code));
    if ($message !== '') {
        $status = array('status' => 'error', 'pid' => 0, 'ended' => time(), 'error' => $message);
        $write();
        exit(1);
    }
}
$status['status'] = 'stopped';
@unlink($f['status']);
