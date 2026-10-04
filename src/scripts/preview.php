<?php

if (PHP_SAPI !== 'cli') {
    exit(1);
}

require_once dirname(__DIR__) . '/include/lib/preview.php';

rb_use_local_timezone();

$id = $argv[1] ?? '';
$req = preg_match('/^[0-9a-z-]{6,40}$/', $id) ? rb_json_read(rb_preview_file_path($id, '.request.json')) : null;
if (!is_array($req)) {
    exit(2);
}
$settings = $req['settings'];
if ($settings['nice'] > 0 && function_exists('proc_nice')) {
    @proc_nice((int)$settings['nice']);
}
$io = array('idle' => '-c 3', 'low' => '-c 2 -n 7')[$settings['io_priority']] ?? '';
if ($io !== '') {
    @exec('ionice ' . $io . ' -p ' . getmypid() . ' 2>/dev/null');
}

$status = array('id' => $id, 'status' => 'running', 'started' => time(), 'pid' => getmypid(), 'progress' => null);
$write = function () use (&$status, $id) {
    rb_json_write(rb_preview_file_path($id), $status);
};
$write();
register_shutdown_function(function () use (&$status, $write) {
    if ($status['status'] === 'running') {
        $e = error_get_last();
        $status['status'] = 'error';
        $status['error'] = 'The preview stopped unexpectedly' . ($e ? ': ' . $e['message'] : '.');
        $write();
    }
});

$result = rb_preview_run($req['job'], $req['repo'], $settings,
    function ($p) use (&$status, $write) {
        $status['progress'] = $p;
        $write();
    },
    function () use ($id) {
        return file_exists(rb_preview_file_path($id, '.cancel'));
    });
$status['status'] = $result['cancelled'] ? 'cancelled' : 'done';
$status['ended'] = time();
$status['progress'] = null;
$status['result'] = $result;
$write();
@unlink(rb_preview_file_path($id, '.request.json'));
@unlink(rb_preview_file_path($id, '.cancel'));
