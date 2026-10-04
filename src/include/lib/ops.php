<?php

require_once __DIR__ . '/config.php';

define('RB_OPS_DIR', RB_RUN_DIR . '/ops');
define('RB_LOCK_DIR', RB_RUN_DIR . '/locks');

function rb_prepare_data_dir(array $settings)
{
    foreach (array('', '/cache', '/tmp', '/logs') as $sub) {
        $dir = $settings['data_dir'] . $sub;
        if (!is_dir($dir) && !@mkdir($dir, 0755, true)) {
            return "Cannot create $dir";
        }
    }
    return '';
}

function rb_new_op_id()
{
    return date('Ymd-His') . '-' . bin2hex(random_bytes(2));
}

function rb_op_file($id)
{
    return RB_OPS_DIR . '/' . $id . '.json';
}

function rb_op_write(array $op)
{
    $op['updated'] = time();
    return rb_json_write(rb_op_file($op['id']), $op);
}

function rb_op_read($id)
{
    return preg_match('/^\d{8}-\d{6}-[0-9a-f]{4}$/', $id) ? rb_json_read(rb_op_file($id)) : null;
}

function rb_pid_alive($pid)
{
    $pid = (int)$pid;
    if ($pid <= 0) {
        return false;
    }
    if (function_exists('posix_kill')) {
        return posix_kill($pid, 0);
    }
    return file_exists("/proc/$pid");
}

function rb_ops_list($limit = 50)
{
    $files = glob(RB_OPS_DIR . '/*.json') ?: array();
    rsort($files);
    $out = array();
    foreach (array_slice($files, 0, $limit) as $f) {
        $op = rb_json_read($f);
        if (!$op) {
            continue;
        }
        if (($op['status'] ?? '') === 'running' && !rb_pid_alive($op['runner_pid'] ?? 0)) {
            $op['status'] = 'error';
            $op['error'] = 'The operation ended unexpectedly.';
        }
        $out[] = $op;
    }
    return $out;
}

function rb_op_running_on($repoId, array $kinds)
{
    foreach (rb_ops_running() as $op) {
        if ($op['repo'] === $repoId && in_array($op['kind'], $kinds, true)) {
            return $op;
        }
    }
    return null;
}

function rb_ops_running()
{
    return array_values(array_filter(rb_ops_list(200), function ($op) {
        return $op['status'] === 'running';
    }));
}

function rb_ops_prune($keep = 100)
{
    $files = glob(RB_OPS_DIR . '/*.json') ?: array();
    rsort($files);
    foreach (array_slice($files, $keep) as $f) {
        $op = rb_json_read($f);
        if (!$op || ($op['status'] ?? '') !== 'running' || !rb_pid_alive($op['runner_pid'] ?? 0)) {
            @unlink($f);
            @unlink(substr($f, 0, -5) . '.cancel');
        }
    }
}

function rb_op_cancel_file($id)
{
    return RB_OPS_DIR . '/' . $id . '.cancel';
}

function rb_op_cancel_requested($id)
{
    clearstatcache(true, rb_op_cancel_file($id));
    return file_exists(rb_op_cancel_file($id));
}

function rb_op_request_cancel($id)
{
    $op = rb_op_read($id);
    if (!$op || $op['status'] !== 'running') {
        return false;
    }
    @touch(rb_op_cancel_file($id));
    return true;
}

function rb_repo_lock($repoId)
{
    if (!is_dir(RB_LOCK_DIR)) {
        @mkdir(RB_LOCK_DIR, 0755, true);
    }
    $fh = @fopen(RB_LOCK_DIR . '/' . $repoId . '.lock', 'c');
    if (!$fh) {
        return null;
    }
    if (!flock($fh, LOCK_EX | LOCK_NB)) {
        fclose($fh);
        return null;
    }
    return $fh;
}

function rb_repo_busy($repoId)
{
    $fh = rb_repo_lock($repoId);
    if ($fh === null) {
        return true;
    }
    flock($fh, LOCK_UN);
    fclose($fh);
    return false;
}

function rb_history_file(array $settings)
{
    return $settings['data_dir'] . '/history.jsonl';
}

function rb_history_append(array $settings, array $record)
{
    $file = rb_history_file($settings);
    if (!is_dir(dirname($file)) && !@mkdir(dirname($file), 0755, true)) {
        return false;
    }
    $ok = @file_put_contents($file, json_encode($record, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE) . "\n",
                             FILE_APPEND | LOCK_EX) !== false;
    clearstatcache(true, $file);
    if (@filesize($file) > 4 * 1048576) {
        rb_history_compact($settings);
    }
    return $ok;
}

function rb_history_compact(array $settings)
{
    $file = rb_history_file($settings);
    $fh = @fopen($file, 'r+');
    if (!$fh || !flock($fh, LOCK_EX)) {
        return;
    }
    $cut = time() - $settings['history_days'] * 86400;
    $keep = array();
    while (($line = fgets($fh)) !== false) {
        $r = json_decode($line, true);
        if (is_array($r) && ($r['ended'] ?? 0) >= $cut) {
            $keep[] = rtrim($line, "\n");
        }
    }
    $keep = array_slice($keep, -20000);
    ftruncate($fh, 0);
    rewind($fh);
    fwrite($fh, $keep ? implode("\n", $keep) . "\n" : '');
    flock($fh, LOCK_UN);
    fclose($fh);
}

function rb_history_read(array $settings, array $filter = array(), $limit = 200)
{
    $lines = @file(rb_history_file($settings), FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES);
    if (!$lines) {
        return array();
    }
    $out = array();
    for ($i = count($lines) - 1; $i >= 0 && count($out) < $limit; $i--) {
        $r = json_decode($lines[$i], true);
        if (!is_array($r)) {
            continue;
        }
        foreach ($filter as $k => $v) {
            if ($v !== '' && $v !== null && ($r[$k] ?? null) !== $v) {
                continue 2;
            }
        }
        $out[] = $r;
    }
    return $out;
}

define('RB_SIZES_KEEP', 1100);

function rb_size_point(array $op)
{
    $s = $op['summary'];
    return array((int)$op['ended'], (int)($s['total_bytes_processed'] ?? 0),
                 (int)($s['data_added_packed'] ?? $s['data_added'] ?? 0), (int)($s['total_files_processed'] ?? 0));
}

function rb_job_sizes_from_history(array $settings, $jobId)
{
    $points = array();
    foreach (rb_history_read($settings, array('kind' => 'backup', 'job' => $jobId), RB_SIZES_KEEP) as $op) {
        if (in_array($op['status'] ?? '', array('success', 'warning'), true) && is_array($op['summary'] ?? null)) {
            $points[] = rb_size_point($op);
        }
    }
    return array_reverse($points);
}

function rb_state_add_repo_size(array &$state, $repoId, array $stats)
{
    $sizes = $state['repos'][$repoId]['sizes'] ?? array();
    $point = array((int)$stats['at'], (int)($stats['total_size'] ?? 0), (int)($stats['snapshots_count'] ?? 0));
    $last = end($sizes);
    if ($last && date('Y-m-d', $last[0]) === date('Y-m-d', $point[0])) {
        array_pop($sizes);
    }
    $sizes[] = $point;
    $state['repos'][$repoId]['sizes'] = array_slice($sizes, -RB_SIZES_KEEP);
}

function rb_state_file(array $settings)
{
    return $settings['data_dir'] . '/state.json';
}

function rb_state_read(array $settings)
{
    $s = rb_json_read(rb_state_file($settings), array());
    return array(
        'jobs'  => is_array($s['jobs'] ?? null) ? $s['jobs'] : array(),
        'repos' => is_array($s['repos'] ?? null) ? $s['repos'] : array(),
    );
}

function rb_state_update(array $settings, $fn)
{
    if (!is_dir(RB_LOCK_DIR)) {
        @mkdir(RB_LOCK_DIR, 0755, true);
    }
    $lock = @fopen(RB_LOCK_DIR . '/state.lock', 'c');
    if ($lock) {
        flock($lock, LOCK_EX);
    }
    $state = rb_state_read($settings);
    $fn($state);
    $ok = is_dir($settings['data_dir']) && rb_json_write(rb_state_file($settings), $state);
    if ($lock) {
        flock($lock, LOCK_UN);
        fclose($lock);
    }
    return $ok;
}

function rb_spawn(array $args, $script = 'run.php')
{
    $cmd = 'nohup setsid ' . escapeshellarg(RB_PHP) . ' ' . escapeshellarg(dirname(__DIR__, 2) . '/scripts/' . $script);
    foreach ($args as $a) {
        $cmd .= ' ' . escapeshellarg($a);
    }
    exec($cmd . ' >/dev/null 2>&1 &');
}
