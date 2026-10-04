<?php

require_once __DIR__ . '/common.php';

define('RB_NOTIFY', getenv('RB_NOTIFY') ?: '/usr/local/emhttp/webGui/scripts/notify');

function rb_notify($subject, $description, $level = 'normal', $message = '')
{
    if (!is_executable(RB_NOTIFY)) {
        return false;
    }
    $cmd = array(RB_NOTIFY, '-e', 'Restic Backup', '-s', $subject, '-d', $description, '-i', $level);
    if ($message !== '') {
        array_push($cmd, '-m', $message);
    }
    exec('nohup setsid ' . implode(' ', array_map('escapeshellarg', $cmd)) . ' >/dev/null 2>&1 &');
    return true;
}

function rb_healthcheck_ping($url, $event, $body = '')
{
    if ($url === '') {
        return;
    }
    $target = rtrim($url, '/') . ($event === 'start' ? '/start' : ($event === 'fail' ? '/fail' : ''));
    $cmd = array('curl', '-fsS', '-m', '10', '--retry', '2', '-o', '/dev/null');
    if ($body !== '') {
        array_push($cmd, '--data-binary', '@-');
    }
    $cmd[] = $target;
    $proc = @proc_open($cmd, array(0 => array('pipe', 'r'), 1 => array('file', '/dev/null', 'w'),
                                   2 => array('file', '/dev/null', 'w')), $pipes);
    if (is_resource($proc)) {
        fwrite($pipes[0], substr($body, -10000));
        fclose($pipes[0]);
        proc_close($proc);
    }
}

function rb_parity_running()
{
    $ini = rb_var_ini();
    return (int)($ini['mdResyncPos'] ?? 0) > 0 && (int)($ini['mdResync'] ?? 0) > 0;
}

function rb_array_started_at()
{
    $t = @file_get_contents(RB_RUN_DIR . '/array_started');
    return $t === false ? 0 : (int)$t;
}
