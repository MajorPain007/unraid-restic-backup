<?php

require_once __DIR__ . '/config.php';

function rb_repo_url(array $repo)
{
    switch ($repo['type']) {
        case 'local':
            return $repo['local']['path'];
        case 'rest':
            return 'rest:' . $repo['rest']['url'];
        default:
            $s = $repo['sftp'];
            $host = strpos($s['host'], ':') !== false ? '[' . $s['host'] . ']' : $s['host'];
            return 'sftp:' . $s['user'] . '@' . $host . ':' . $s['path'];
    }
}

function rb_ssh_args(array $repo)
{
    return array(
        '-i', rb_ssh_key_file($repo['id']),
        '-p', (string)$repo['sftp']['port'],
        '-o', 'IdentitiesOnly=yes',
        '-o', 'BatchMode=yes',
        '-o', 'StrictHostKeyChecking=yes',
        '-o', 'UserKnownHostsFile=' . rb_known_hosts_file(),
        '-o', 'ConnectTimeout=20',
        '-o', 'ServerAliveInterval=30',
        '-o', 'ServerAliveCountMax=6',
    );
}

function rb_restic_env(array $repo, array $settings)
{
    $env = array(
        'PATH'                 => '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',
        'HOME'                 => '/root',
        'LANG'                 => 'C.UTF-8',
        'RESTIC_REPOSITORY'    => rb_repo_url($repo),
        'RESTIC_PASSWORD_FILE' => rb_secret_file($repo['id'], 'password'),
        'RESTIC_CACHE_DIR'     => $settings['data_dir'] . '/cache',
        'TMPDIR'               => RB_RUN_DIR . '/tmp',
        'RESTIC_PROGRESS_FPS'  => '1',
    );
    if ($repo['type'] === 'rest') {
        if ($repo['rest']['user'] !== '') {
            $env['RESTIC_REST_USERNAME'] = $repo['rest']['user'];
            $env['RESTIC_REST_PASSWORD'] = rb_secret_get($repo['id'], 'rest');
        }
        if ($repo['rest']['cacert'] !== '') {
            $env['RESTIC_CACERT'] = $repo['rest']['cacert'];
        }
    }
    return $env;
}

function rb_restic_global_args(array $repo)
{
    $args = array();
    if ($repo['type'] === 'sftp') {
        $args[] = '-o';
        $args[] = 'sftp.args=' . implode(' ', rb_ssh_args($repo));
    }
    $o = $repo['options'];
    if ($o['connections'] > 0) {
        $args[] = '-o';
        $args[] = $repo['type'] . '.connections=' . $o['connections'];
    }
    if ($o['limit_upload'] > 0) {
        $args[] = '--limit-upload';
        $args[] = (string)$o['limit_upload'];
    }
    if ($o['limit_download'] > 0) {
        $args[] = '--limit-download';
        $args[] = (string)$o['limit_download'];
    }
    if ($o['compression'] !== 'auto') {
        $args[] = '--compression';
        $args[] = $o['compression'];
    }
    foreach ($o['extra_flags'] as $flag) {
        $args[] = $flag;
    }
    return $args;
}

function rb_restic_cmd(array $repo, array $settings, array $args, $lowPriority = true)
{
    $cmd = array();
    if ($lowPriority) {
        if ($settings['nice'] > 0) {
            array_push($cmd, 'nice', '-n', (string)$settings['nice']);
        }
        if ($settings['io_priority'] === 'idle') {
            array_push($cmd, 'ionice', '-c', '3');
        } elseif ($settings['io_priority'] === 'low') {
            array_push($cmd, 'ionice', '-c', '2', '-n', '7');
        }
    }
    $cmd[] = RB_RESTIC;
    return array_merge($cmd, rb_restic_global_args($repo), $args);
}

function rb_proc_stream(array $cmd, array $env, $onLine, $tick = null, $onStart = null, $stdin = null)
{
    $spec = array(0 => $stdin === null ? array('file', '/dev/null', 'r') : array('pipe', 'r'),
                  1 => array('pipe', 'w'), 2 => array('pipe', 'w'));
    $proc = @proc_open($cmd, $spec, $pipes, '/', $env);
    if (!is_resource($proc)) {
        return -1;
    }
    if ($stdin !== null) {
        fwrite($pipes[0], $stdin);
        fclose($pipes[0]);
    }
    $status = proc_get_status($proc);
    if ($onStart) {
        $onStart($status['pid']);
    }
    stream_set_blocking($pipes[1], false);
    stream_set_blocking($pipes[2], false);
    $buf = array(1 => '', 2 => '');
    $open = array(1 => true, 2 => true);
    $interrupted = false;
    while ($open[1] || $open[2]) {
        $read = array();
        foreach (array(1, 2) as $i) {
            if ($open[$i]) {
                $read[] = $pipes[$i];
            }
        }
        $w = $e = null;
        $n = @stream_select($read, $w, $e, 0, 200000);
        if ($n === false) {
            usleep(200000);
        }
        foreach (array(1, 2) as $i) {
            if (!$open[$i]) {
                continue;
            }
            $chunk = fread($pipes[$i], 65536);
            if ($chunk !== false && $chunk !== '') {
                $buf[$i] .= $chunk;
                while (($pos = strpos($buf[$i], "\n")) !== false) {
                    $onLine($i === 1 ? 'out' : 'err', rtrim(substr($buf[$i], 0, $pos), "\r"));
                    $buf[$i] = substr($buf[$i], $pos + 1);
                }
            } elseif (feof($pipes[$i])) {
                if ($buf[$i] !== '') {
                    $onLine($i === 1 ? 'out' : 'err', rtrim($buf[$i], "\r"));
                    $buf[$i] = '';
                }
                $open[$i] = false;
            }
        }
        if ($tick && !$interrupted && $tick()) {
            $interrupted = true;
            proc_terminate($proc, 2);
        }
    }
    fclose($pipes[1]);
    fclose($pipes[2]);
    $code = -1;
    for ($i = 0; $i < 600; $i++) {
        $status = proc_get_status($proc);
        if (!$status['running']) {
            $code = $status['exitcode'];
            break;
        }
        usleep(100000);
    }
    $closed = proc_close($proc);
    return $code !== -1 ? $code : $closed;
}

function rb_restic_capture(array $repo, array $settings, array $args, $limitLines = 200000)
{
    $out = array();
    $err = array();
    $code = rb_proc_stream(
        rb_restic_cmd($repo, $settings, $args, false),
        rb_restic_env($repo, $settings),
        function ($stream, $line) use (&$out, &$err, $limitLines) {
            if ($stream === 'out') {
                if (count($out) < $limitLines) {
                    $out[] = $line;
                }
            } elseif (count($err) < 200) {
                $err[] = $line;
            }
        }
    );
    return array($code, $out, $err);
}

function rb_restic_error(array $errLines, $code)
{
    $plain = array();
    $fatal = '';
    foreach ($errLines as $line) {
        $j = json_decode($line, true);
        if (is_array($j) && ($j['message_type'] ?? '') === 'exit_error') {
            $fatal = (string)($j['message'] ?? '');
        } elseif (is_array($j) && ($j['message_type'] ?? '') === 'error') {
            $plain[] = (string)($j['error']['message'] ?? '');
        } elseif (trim($line) !== '') {
            $plain[] = trim($line);
        }
    }
    $ssh = array();
    foreach ($plain as $line) {
        if (strpos($line, 'subprocess ssh:') === 0) {
            $ssh[] = trim(substr($line, 15));
        }
    }
    foreach ($ssh as $line) {
        $explained = rb_explain_ssh($line);
        if (strpos($explained, 'SSH: ') !== 0) {
            return $explained;
        }
    }
    foreach ($ssh as $line) {
        if (preg_match('/[A-Za-z]{3}/', $line)) {
            return rb_explain_ssh($line);
        }
    }
    if ($fatal !== '') {
        return rb_explain_restic($fatal, $code);
    }
    if ($plain) {
        return end($plain);
    }
    return $code === -1 ? 'restic could not be started' : "restic exited with code $code";
}

function rb_explain_restic($msg, $code)
{
    $first = trim(strtok(str_replace('Fatal: ', '', $msg), "\n"));
    switch ($code) {
        case 10:
            return 'There is no repository at this location. Initialize it first, or check the path.';
        case 11:
            return 'The repository is locked by another operation. ' . $first;
        case 12:
            return 'Wrong password for this repository.';
        case 130:
            return 'Stopped.';
    }
    return $first;
}

function rb_explain_ssh($msg)
{
    if (stripos($msg, 'IDENTIFICATION HAS CHANGED') !== false) {
        return 'The server\'s host key has changed since it was confirmed. That happens when the server is '
             . 'reinstalled - or when someone is in between. Check, then confirm the new key with "Test connection".';
    }
    if (stripos($msg, 'Host key verification failed') !== false) {
        return 'The server\'s host key is not confirmed yet, or it has changed. Use "Test connection" to check and confirm it.';
    }
    if (stripos($msg, 'Permission denied') !== false) {
        return 'The server refused the key (' . $msg . '). Add the public key shown in the repository settings to the server.';
    }
    if (stripos($msg, 'Connection refused') !== false || stripos($msg, 'timed out') !== false
        || stripos($msg, 'No route to host') !== false || stripos($msg, 'Could not resolve') !== false) {
        return 'Cannot reach the server: ' . $msg;
    }
    return 'SSH: ' . $msg;
}

function rb_repo_endpoint(array $repo)
{
    if ($repo['type'] === 'sftp') {
        return array($repo['sftp']['host'], (int)$repo['sftp']['port']);
    }
    if ($repo['type'] === 'rest') {
        $u = parse_url($repo['rest']['url']);
        $port = isset($u['port']) ? (int)$u['port'] : (($u['scheme'] ?? '') === 'https' ? 443 : 80);
        return array($u['host'] ?? '', $port);
    }
    return null;
}

function rb_repo_reachable(array $repo, $attempts = 3, $wait = 20, $cancelled = null, $log = null)
{
    $ep = rb_repo_endpoint($repo);
    if ($ep === null) {
        return '';
    }
    $host = trim($ep[0], '[]');
    $why = '';
    for ($i = 1; $i <= $attempts; $i++) {
        $errno = 0;
        $errstr = '';
        $fh = @fsockopen(strpos($host, ':') !== false ? "[$host]" : $host, $ep[1], $errno, $errstr, 10);
        if ($fh) {
            fclose($fh);
            return '';
        }
        $why = "Cannot reach {$ep[0]} on port {$ep[1]}" . ($errstr !== '' ? ": $errstr" : '');
        if ($log) {
            $log("Attempt $i of $attempts: $why" . ($i < $attempts ? " - trying again in {$wait}s" : ''));
        }
        for ($s = 0; $i < $attempts && $s < $wait; $s++) {
            if ($cancelled && $cancelled()) {
                return $why;
            }
            sleep(1);
        }
    }
    return $why . '.';
}

function rb_json_lines(array $lines)
{
    $out = array();
    foreach ($lines as $line) {
        $j = json_decode($line, true);
        if (is_array($j)) {
            $out[] = $j;
        }
    }
    return $out;
}
