<?php

if (PHP_SAPI !== 'cli') {
    exit(1);
}

require_once dirname(__DIR__) . '/include/lib/ops.php';
require_once dirname(__DIR__) . '/include/lib/browse.php';

$wait = max(5, (int)($argv[1] ?? 60));
$running = rb_ops_running();
foreach ($running as $op) {
    rb_op_request_cancel($op['id']);
}
foreach (rb_browse_repos() as $id) {
    rb_browse_stop($id);
}
$deadline = time() + $wait;
while (time() < $deadline) {
    $left = array_filter($running, function ($op) {
        return rb_pid_alive($op['runner_pid']);
    });
    $browsing = array_filter(rb_browse_repos(), 'rb_browse_running');
    if (!$left && !$browsing) {
        rb_browse_reap(true);
        exit(0);
    }
    sleep(1);
}
rb_browse_reap(true);
foreach ($running as $op) {
    foreach (array((int)($op['pid'] ?? 0), (int)$op['runner_pid']) as $pid) {
        if ($pid > 0 && rb_pid_alive($pid)) {
            if (function_exists('posix_kill')) {
                posix_kill($pid, 9);
            } else {
                exec('kill -9 ' . $pid);
            }
        }
    }
}
exit(0);
