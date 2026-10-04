<?php

require_once __DIR__ . '/ops.php';
require_once __DIR__ . '/restic.php';
require_once __DIR__ . '/schedule.php';
require_once __DIR__ . '/unraid.php';
require_once __DIR__ . '/jobs.php';
require_once __DIR__ . '/consistency.php';
require_once __DIR__ . '/preview.php';
require_once __DIR__ . '/browse.php';

class RbApiError extends Exception
{
    public $errors;

    public function __construct($message, array $errors = array())
    {
        parent::__construct($message);
        $this->errors = $errors;
    }
}

function rb_api_dispatch($action, array $p)
{
    rb_use_local_timezone();
    $fn = 'rb_api_' . $action;
    if (!preg_match('/^[a-z_]{2,40}$/', $action) || !function_exists($fn)) {
        return array('ok' => false, 'error' => "Unknown action \"$action\"");
    }
    try {
        $result = $fn($p);
        return array('ok' => true) + $result;
    } catch (RbApiError $e) {
        return array('ok' => false, 'error' => $e->getMessage(), 'errors' => $e->errors);
    } catch (Throwable $e) {
        return array('ok' => false, 'error' => $e->getMessage());
    }
}

function rb_p(array $p, $key, $default = '')
{
    return array_key_exists($key, $p) ? $p[$key] : $default;
}

function rb_pj(array $p, $key)
{
    $v = rb_p($p, $key, null);
    if (is_array($v)) {
        return $v;
    }
    $d = is_string($v) ? json_decode($v, true) : null;
    if (!is_array($d)) {
        throw new RbApiError("Missing or malformed \"$key\"");
    }
    return $d;
}

function rb_need_repo(array $cfg, $id)
{
    $repo = rb_repo($cfg, (string)$id);
    if (!$repo) {
        throw new RbApiError('That repository does not exist (any more).');
    }
    return $repo;
}

function rb_need_job(array $cfg, $id)
{
    $job = rb_job($cfg, (string)$id);
    if (!$job) {
        throw new RbApiError('That job does not exist (any more).');
    }
    return $job;
}

function rb_need_snapshot_id($id)
{
    $id = (string)$id;
    if (!preg_match('/^[0-9a-f]{8,64}$/', $id)) {
        throw new RbApiError('Not a snapshot ID');
    }
    return $id;
}

function rb_need_snapshot_path($path)
{
    $path = (string)$path;
    if ($path === '' || $path[0] !== '/' || strpbrk($path, "\0\n\r") !== false) {
        throw new RbApiError('Not a path inside the snapshot');
    }
    return $path === '/' ? '/' : rtrim($path, '/');
}

function rb_restic_ready(array $cfg, array $repo)
{
    if (!is_executable(RB_RESTIC)) {
        throw new RbApiError('restic is not installed (' . RB_RESTIC . '). Reinstall the plugin.');
    }
    if (!rb_secret_has($repo['id'], 'password')) {
        throw new RbApiError('The repository has no password set.');
    }
    if (rb_prepare_data_dir($cfg['settings']) !== '') {
        throw new RbApiError('The data folder ' . $cfg['settings']['data_dir'] . ' cannot be created. Is the array started?');
    }
}

function rb_api_restic(array $cfg, array $repo, array $args, $limitLines = 200000)
{
    rb_restic_ready($cfg, $repo);
    $why = rb_repo_reachable($repo, 1, 0);
    if ($why !== '') {
        throw new RbApiError($why);
    }
    list($code, $out, $err) = rb_restic_capture($repo, $cfg['settings'], $args, $limitLines);
    if ($code !== 0) {
        throw new RbApiError(rb_restic_error($err, $code));
    }
    return $out;
}

function rb_api_lock(array $repo)
{
    $lock = rb_repo_lock($repo['id']);
    if ($lock === null) {
        throw new RbApiError('An operation on this repository is running. Try again when it has finished.');
    }
    rb_browse_stop($repo['id']);
    return $lock;
}

function rb_cache_dir()
{
    $dir = RB_RUN_DIR . '/cache';
    if (!is_dir($dir)) {
        @mkdir($dir, 0755, true);
    }
    return $dir;
}

function rb_config_change($fn)
{
    $lock = rb_config_lock();
    try {
        $cfg = rb_config_load();
        $result = $fn($cfg);
        if (!rb_config_save($cfg)) {
            throw new RbApiError('The configuration could not be written to ' . RB_CONFIG_FILE . '.');
        }
        return $result;
    } finally {
        if ($lock) {
            flock($lock, LOCK_UN);
            fclose($lock);
        }
    }
}

function rb_api_overview(array $p)
{
    $cfg = rb_config_load();
    $settings = $cfg['settings'];
    $now = time();
    $state = rb_state_read($settings);
    $running = rb_ops_running();
    $byRepo = array();
    foreach ($running as $op) {
        $byRepo[$op['repo']] = $op;
    }
    $jobs = array();
    foreach ($cfg['jobs'] as $job) {
        $js = $state['jobs'][$job['id']] ?? array();
        $op = null;
        foreach ($running as $r) {
            if ($r['job'] === $job['id']) {
                $op = $r;
            }
        }
        $jobs[] = array(
            'id'       => $job['id'],
            'schedule' => rb_sched_describe($job['schedule']),
            'next'     => $job['enabled'] ? rb_sched_next($job['schedule'], $js['sched'] ?? array(), $now) : null,
            'last'     => $js['last'] ?? null,
            'last_success' => $js['last_success'] ?? null,
            'queued'   => !empty($js['queued']),
            'running'  => $op,
            'recent'   => array_map(function ($h) {
                return array('id' => $h['id'], 'status' => $h['status'], 'started' => $h['started'],
                             'ended' => $h['ended'], 'added' => $h['summary']['data_added_packed'] ?? null);
            }, rb_history_read($settings, array('job' => $job['id'], 'kind' => 'backup'), 30)),
        );
    }
    $repos = array();
    foreach ($cfg['repos'] as $repo) {
        $rs = $state['repos'][$repo['id']] ?? array();
        $repos[] = array(
            'id'      => $repo['id'],
            'url'     => rb_repo_url($repo),
            'stats'   => $rs['stats'] ?? null,
            'prune'   => array('last' => $rs['prune']['last'] ?? null,
                               'next' => $repo['maintenance']['prune']['enabled'] && !$repo['append_only']
                                   ? rb_sched_next($repo['maintenance']['prune']['schedule'], $rs['prune']['sched'] ?? array(), $now) : null,
                               'schedule' => rb_sched_describe($repo['maintenance']['prune']['schedule'])),
            'check'   => array('last' => $rs['check']['last'] ?? null,
                               'next' => $repo['maintenance']['check']['enabled']
                                   ? rb_sched_next($repo['maintenance']['check']['schedule'], $rs['check']['sched'] ?? array(), $now) : null,
                               'schedule' => rb_sched_describe($repo['maintenance']['check']['schedule'])),
            'running' => $byRepo[$repo['id']] ?? null,
        );
    }
    return array(
        'now'      => $now,
        'config'   => rb_config_public($cfg),
        'jobs'     => $jobs,
        'repos'    => $repos,
        'running'  => $running,
        'recent'   => rb_history_read($settings, array(), 15),
        'system'   => array(
            'array_started' => rb_array_started(),
            'restic'        => rb_restic_version(),
            'server'        => rb_server_name(),
            'data_dir_ok'   => is_dir($settings['data_dir']),
            'timezone'      => date_default_timezone_get(),
            'scheduler_tick' => (int)@filemtime(RB_RUN_DIR . '/scheduler.tick'),
        ),
    );
}

function rb_api_ops(array $p)
{
    return array('now' => time(), 'running' => rb_ops_running());
}

function rb_restic_version()
{
    $cache = RB_RUN_DIR . '/restic.version';
    $v = @file_get_contents($cache);
    if ($v !== false && @filemtime($cache) >= @filemtime(RB_RESTIC)) {
        return trim($v);
    }
    if (!is_executable(RB_RESTIC)) {
        return '';
    }
    $out = array();
    exec(escapeshellarg(RB_RESTIC) . ' version --json 2>/dev/null', $out);
    $j = json_decode(implode('', $out), true);
    $v = is_array($j) ? (string)($j['version'] ?? '') : '';
    if ($v !== '') {
        @mkdir(RB_RUN_DIR, 0755, true);
        @file_put_contents($cache, $v);
    }
    return $v;
}

function rb_api_history(array $p)
{
    $cfg = rb_config_load();
    $filter = array();
    foreach (array('job', 'repo', 'kind', 'status') as $k) {
        if (rb_p($p, $k) !== '') {
            $filter[$k] = (string)rb_p($p, $k);
        }
    }
    $limit = max(1, min(1000, (int)rb_p($p, 'limit', 200)));
    return array('history' => rb_history_read($cfg['settings'], $filter, $limit));
}

function rb_api_op_log(array $p)
{
    $id = (string)rb_p($p, 'id');
    $op = rb_op_read($id);
    if (!$op) {
        $cfg = rb_config_load();
        foreach (rb_history_read($cfg['settings'], array(), 5000) as $h) {
            if ($h['id'] === $id) {
                $op = $h;
                break;
            }
        }
    }
    if (!$op || empty($op['log'])) {
        throw new RbApiError('No log for this operation.');
    }
    $file = $op['log'];
    if (strpos($file, '/logs/') === false || substr($file, -4) !== '.log') {
        throw new RbApiError('No log for this operation.');
    }
    $size = (int)@filesize($file);
    $offset = (int)rb_p($p, 'offset', -1);
    if ($offset < 0 || $offset > $size) {
        $offset = max(0, $size - 262144);
    }
    $fh = @fopen($file, 'r');
    $text = '';
    if ($fh) {
        fseek($fh, $offset);
        $text = (string)stream_get_contents($fh, 1048576);
        fclose($fh);
    }
    return array('text' => $text, 'offset' => $offset + strlen($text), 'size' => $size,
                 'status' => $op['status'] ?? '', 'op' => $op);
}

function rb_api_op_stop(array $p)
{
    if (!rb_op_request_cancel((string)rb_p($p, 'id'))) {
        throw new RbApiError('That operation is not running.');
    }
    return array();
}

function rb_api_settings_save(array $p)
{
    $in = rb_pj($p, 'settings');
    return rb_config_change(function (&$cfg) use ($in) {
        $errors = array();
        $settings = rb_clean_settings($in, $errors);
        if ($errors) {
            throw new RbApiError('Please correct the marked fields.', $errors);
        }
        $cfg['settings'] = $settings;
        return array('settings' => $settings);
    });
}

function rb_api_schedule_preview(array $p)
{
    $errors = array();
    $s = rb_clean_schedule(rb_pj($p, 'schedule'), 'Schedule', $errors);
    if ($errors) {
        return array('valid' => false, 'error' => preg_replace('/^Schedule: /', '', $errors[0]), 'next' => array());
    }
    $next = array();
    $now = time();
    if ($s['mode'] === 'cron') {
        $next = RbCron::parse($s['cron'])->upcoming($now, 5);
    } elseif ($s['mode'] === 'interval') {
        $step = rb_sched_interval($s);
        for ($i = 1; $i <= 5; $i++) {
            $next[] = $now + $i * $step;
        }
    }
    return array('valid' => true, 'description' => rb_sched_describe($s), 'next' => $next);
}

function rb_api_repo_save(array $p)
{
    $in = rb_pj($p, 'repo');
    $passwordMode = (string)rb_p($p, 'password_mode', 'keep');
    $password = (string)rb_p($p, 'password');
    $restPassword = (string)rb_p($p, 'rest_password');
    $restPasswordMode = (string)rb_p($p, 'rest_password_mode', 'keep');
    return rb_config_change(function (&$cfg) use ($in, $passwordMode, $password, $restPassword, $restPasswordMode) {
        $errors = array();
        $isNew = empty($in['id']);
        if ($isNew) {
            $in['id'] = rb_new_id('r');
        } elseif (rb_find($cfg['repos'], (string)$in['id']) === null) {
            throw new RbApiError('That repository does not exist (any more).');
        }
        $repo = rb_clean_repo($in, $errors);
        if ($repo['id'] === '') {
            throw new RbApiError('Malformed repository ID');
        }
        if ($passwordMode === 'set' && $password === '') {
            $errors[] = 'Password: enter the repository password';
        } elseif ($passwordMode === 'set' && strpbrk($password, "\r\n") !== false) {
            $errors[] = 'Password: a single line';
        }
        if ($passwordMode === 'keep' && !rb_secret_has($repo['id'], 'password')) {
            $errors[] = 'Password: a repository needs one';
        }
        if ($repo['type'] === 'rest' && $restPasswordMode === 'set' && strpbrk($restPassword, "\r\n") !== false) {
            $errors[] = 'REST password: a single line';
        }
        foreach ($cfg['repos'] as $other) {
            if ($other['id'] !== $repo['id'] && rb_repo_url($other) === rb_repo_url($repo)) {
                $errors[] = "Location: \"{$other['name']}\" already uses this repository";
            }
        }
        if ($errors) {
            throw new RbApiError('Please correct the marked fields.', $errors);
        }
        $generated = '';
        if ($passwordMode === 'generate') {
            $generated = rb_generate_password();
            $password = $generated;
        }
        if ($passwordMode !== 'keep' && !rb_secret_set($repo['id'], 'password', $password)) {
            throw new RbApiError('The password could not be stored.');
        }
        if ($repo['type'] === 'rest' && $restPasswordMode === 'set') {
            rb_secret_set($repo['id'], 'rest', $restPassword);
        }
        $i = rb_find($cfg['repos'], $repo['id']);
        if ($i === null) {
            $cfg['repos'][] = $repo;
        } else {
            $cfg['repos'][$i] = $repo;
        }
        if ($repo['type'] === 'sftp' && !is_file(rb_ssh_key_file($repo['id']))) {
            rb_ssh_keygen($repo['id']);
        }
        rb_browse_stop($repo['id']);
        $public = rb_config_public(array('repos' => array($repo)) + $cfg);
        return array('repo' => $public['repos'][0], 'generated_password' => $generated, 'created' => $isNew);
    });
}

function rb_api_repo_delete(array $p)
{
    $id = (string)rb_p($p, 'id');
    rb_browse_stop($id, 20);
    return rb_config_change(function (&$cfg) use ($id) {
        $repo = rb_need_repo($cfg, $id);
        $users = array();
        foreach ($cfg['jobs'] as $job) {
            if ($job['repo'] === $id) {
                $users[] = $job['name'];
            }
        }
        if ($users) {
            throw new RbApiError('Jobs still back up to this repository: ' . implode(', ', $users) . '. Delete them or point them elsewhere first.');
        }
        if (rb_repo_busy($id)) {
            throw new RbApiError('An operation on this repository is running.');
        }
        array_splice($cfg['repos'], rb_find($cfg['repos'], $id), 1);
        foreach (array('password', 'rest') as $kind) {
            @unlink(rb_secret_file($id, $kind));
        }
        @unlink(rb_ssh_key_file($id));
        @unlink(rb_ssh_key_file($id) . '.pub');
        @unlink(rb_cache_dir() . "/snapshots-$id.json");
        return array('deleted' => $repo['name']);
    });
}

function rb_api_repo_password(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    return array('password' => rb_secret_get($repo['id'], 'password'));
}

function rb_ssh_keygen($repoId)
{
    rb_private_dir(RB_SSH_DIR);
    $file = rb_ssh_key_file($repoId);
    @unlink($file);
    @unlink("$file.pub");
    $comment = 'restic.backup@' . rb_server_name();
    $code = rb_proc_stream(array('ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-C', $comment, '-f', $file),
                           array('PATH' => '/usr/bin:/bin:/usr/sbin:/sbin'), function () {
                           });
    @chmod($file, 0600);
    if ($code !== 0 || !is_file("$file.pub")) {
        throw new RbApiError('ssh-keygen could not create a key.');
    }
    return trim((string)file_get_contents("$file.pub"));
}

function rb_api_repo_keygen(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    if ($repo['type'] !== 'sftp') {
        throw new RbApiError('Only SFTP repositories use an SSH key.');
    }
    return array('public_key' => rb_ssh_keygen($repo['id']));
}

function rb_known_hosts_name($host, $port)
{
    return (int)$port === 22 ? $host : "[$host]:$port";
}

function rb_fingerprints(array $lines)
{
    if (!$lines) {
        return array();
    }
    $out = array();
    $fp = array();
    rb_proc_stream(array('ssh-keygen', '-l', '-f', '/dev/stdin'), array('PATH' => '/usr/bin:/bin'),
        function ($stream, $line) use (&$out) {
            if ($stream === 'out' && preg_match('/^(\d+)\s+(\S+)\s+.*\((\w+)\)\s*$/', $line, $m)) {
                $out[] = array('type' => $m[3], 'bits' => (int)$m[1], 'fingerprint' => $m[2]);
            }
        }, null, null, implode("\n", $lines) . "\n");
    return $out;
}

function rb_scan_host_keys(array $repo)
{
    $lines = array();
    $err = array();
    rb_proc_stream(array('ssh-keyscan', '-T', '10', '-p', (string)$repo['sftp']['port'], $repo['sftp']['host']),
        array('PATH' => '/usr/bin:/bin'),
        function ($stream, $line) use (&$lines, &$err) {
            if ($stream === 'out' && $line !== '' && $line[0] !== '#') {
                $lines[] = $line;
            } elseif ($stream === 'err' && $line !== '' && $line[0] !== '#') {
                $err[] = $line;
            }
        });
    if (!$lines) {
        throw new RbApiError('The server did not answer on port ' . $repo['sftp']['port'] .
                             ($err ? ': ' . end($err) : '.') . ' Check the host name and port.');
    }
    return $lines;
}

function rb_known_host_lines(array $repo)
{
    $name = rb_known_hosts_name($repo['sftp']['host'], $repo['sftp']['port']);
    $out = array();
    foreach (@file(rb_known_hosts_file(), FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES) ?: array() as $line) {
        $hosts = explode(',', strtok($line, ' '));
        if (in_array($name, $hosts, true)) {
            $out[] = $line;
        }
    }
    return $out;
}

function rb_api_repo_hostkey(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    if ($repo['type'] !== 'sftp') {
        throw new RbApiError('Only SFTP repositories have a host key.');
    }
    $scanned = rb_fingerprints(rb_scan_host_keys($repo));
    $stored = rb_fingerprints(array_map(function ($l) {
        return substr($l, strpos($l, ' ') + 1);
    }, rb_known_host_lines($repo)));
    $status = $stored ? 'unknown-type' : 'unknown';
    foreach ($scanned as $k) {
        $sameType = array_filter($stored, function ($s) use ($k) {
            return $s['type'] === $k['type'];
        });
        if (!$sameType) {
            continue;
        }
        if (!in_array($k['fingerprint'], array_column($sameType, 'fingerprint'), true)) {
            $status = 'changed';
            break;
        }
        $status = 'trusted';
    }
    if ($status === 'unknown-type') {
        $status = 'unknown';
    }
    return array('status' => $status, 'scanned' => $scanned, 'stored' => $stored);
}

function rb_api_repo_hostkey_trust(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    $confirmed = rb_pj($p, 'fingerprints');
    $lines = rb_scan_host_keys($repo);
    $keep = array();
    foreach ($lines as $line) {
        $fp = rb_fingerprints(array($line));
        if ($fp && in_array($fp[0]['fingerprint'], $confirmed, true)) {
            $keep[] = $line;
        }
    }
    if (!$keep) {
        throw new RbApiError('The server now shows different keys from the ones you confirmed. Check again.');
    }
    $name = rb_known_hosts_name($repo['sftp']['host'], $repo['sftp']['port']);
    $existing = @file(rb_known_hosts_file(), FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES) ?: array();
    $others = array_filter($existing, function ($line) use ($name) {
        return !in_array($name, explode(',', strtok($line, ' ')), true);
    });
    foreach ($keep as &$line) {
        $line = $name . substr($line, strpos($line, ' '));
    }
    unset($line);
    rb_private_dir(RB_SSH_DIR);
    if (!rb_file_write(rb_known_hosts_file(), implode("\n", array_merge(array_values($others), $keep)) . "\n", 0600)) {
        throw new RbApiError('known_hosts could not be written.');
    }
    return array('trusted' => count($keep));
}

function rb_api_repo_test(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    rb_restic_ready($cfg, $repo);
    if ($repo['type'] === 'sftp') {
        if (!is_file(rb_ssh_key_file($repo['id']))) {
            throw new RbApiError('The repository has no SSH key yet.');
        }
        $hk = rb_api_repo_hostkey(array('id' => $repo['id']));
        if ($hk['status'] !== 'trusted') {
            return array('status' => 'hostkey', 'hostkey' => $hk);
        }
    }
    if ($repo['type'] === 'local') {
        $parent = dirname($repo['local']['path']);
        while ($parent !== '/' && !is_dir($parent)) {
            $parent = dirname($parent);
        }
        if (preg_match('#^/mnt/[^/]+$#', $parent) && !rb_dir_has_entries($parent)) {
            throw new RbApiError("$parent is empty - is the disk mounted?");
        }
    }
    $why = rb_repo_reachable($repo, 1, 0);
    if ($why !== '') {
        throw new RbApiError($why);
    }
    list($code, $out, $err) = rb_restic_capture($repo, $cfg['settings'], array('cat', 'config', '--no-lock'));
    if ($code === 10) {
        return array('status' => 'empty');
    }
    if ($code !== 0) {
        throw new RbApiError(rb_restic_error($err, $code));
    }
    $c = json_decode(implode("\n", $out), true);
    return array('status' => 'ok', 'repository_id' => (string)($c['id'] ?? ''), 'version' => (int)($c['version'] ?? 0));
}

function rb_api_repo_init(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    $test = rb_api_repo_test($p);
    if ($test['status'] === 'ok') {
        throw new RbApiError('There already is a repository at this location.');
    }
    if ($test['status'] !== 'empty') {
        throw new RbApiError('Confirm the server\'s host key first.');
    }
    $lock = rb_api_lock($repo);
    $out = rb_api_restic($cfg, $repo, array('init', '--json'));
    flock($lock, LOCK_UN);
    $j = rb_json_lines($out);
    return array('repository_id' => (string)($j[0]['id'] ?? ''));
}

function rb_api_repo_unlock(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    $all = rb_bool(rb_p($p, 'all', false));
    $lock = rb_api_lock($repo);
    rb_api_restic($cfg, $repo, $all ? array('unlock', '--remove-all') : array('unlock'));
    flock($lock, LOCK_UN);
    return array();
}

function rb_api_repo_stats(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    $out = rb_api_restic($cfg, $repo, array('stats', '--json', '--mode', 'raw-data', '--no-lock'));
    $stats = json_decode(implode("\n", $out), true);
    if (!is_array($stats)) {
        throw new RbApiError('restic stats gave no result.');
    }
    $stats['at'] = time();
    rb_state_update($cfg['settings'], function (&$state) use ($repo, $stats) {
        $state['repos'][$repo['id']]['stats'] = $stats;
        rb_state_add_repo_size($state, $repo['id'], $stats);
    });
    return array('stats' => $stats);
}

function rb_api_size_history(array $p)
{
    $cfg = rb_config_load();
    $state = rb_state_read($cfg['settings']);
    if ((string)rb_p($p, 'job') !== '') {
        $jobs = array(rb_need_job($cfg, rb_p($p, 'job')));
        $repo = rb_need_repo($cfg, $jobs[0]['repo']);
    } else {
        $repo = rb_need_repo($cfg, rb_p($p, 'repo'));
        $jobs = array_values(array_filter($cfg['jobs'], function ($j) use ($repo) {
            return $j['repo'] === $repo['id'];
        }));
    }
    return array(
        'repo' => array('id' => $repo['id'], 'name' => $repo['name'], 'points' => $state['repos'][$repo['id']]['sizes'] ?? array()),
        'jobs' => array_map(function ($j) use ($state, $cfg) {
            return array('id' => $j['id'], 'name' => $j['name'],
                         'points' => $state['jobs'][$j['id']]['sizes'] ?? rb_job_sizes_from_history($cfg['settings'], $j['id']));
        }, $jobs),
    );
}

function rb_api_repo_keys(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    $out = rb_api_restic($cfg, $repo, array('key', 'list', '--json', '--no-lock'));
    $keys = json_decode(implode("\n", $out), true);
    return array('keys' => is_array($keys) ? $keys : array());
}

function rb_api_repo_run(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'id'));
    $kind = (string)rb_p($p, 'kind');
    if (!in_array($kind, array('prune', 'check'), true)) {
        throw new RbApiError('Unknown operation');
    }
    rb_restic_ready($cfg, $repo);
    if (rb_repo_busy($repo['id']) || rb_op_running_on($repo['id'], array('restore'))) {
        throw new RbApiError('An operation on this repository is running. Try again when it has finished.');
    }
    rb_spawn(array($kind, $repo['id'], 'manual'));
    return array('started' => true);
}

function rb_api_job_save(array $p)
{
    $in = rb_pj($p, 'job');
    return rb_config_change(function (&$cfg) use ($in) {
        $errors = array();
        if (empty($in['id'])) {
            $in['id'] = rb_new_id('j');
        } elseif (rb_find($cfg['jobs'], (string)$in['id']) === null) {
            throw new RbApiError('That job does not exist (any more).');
        }
        $job = rb_clean_job($in, array_column($cfg['repos'], 'id'), $errors);
        $errors = array_merge($errors, rb_check_job_against_repos($job, $cfg));
        if ($errors) {
            throw new RbApiError('Please correct the marked fields.', $errors);
        }
        $i = rb_find($cfg['jobs'], $job['id']);
        if ($i === null) {
            $cfg['jobs'][] = $job;
        } else {
            $cfg['jobs'][$i] = $job;
        }
        return array('job' => $job);
    });
}

function rb_api_job_preview(array $p)
{
    $cfg = rb_config_load();
    $errors = array();
    $job = rb_clean_job(rb_pj($p, 'job'), array_column($cfg['repos'], 'id'), $errors);
    if (!$job['sources']) {
        throw new RbApiError('Add a folder to back up first.');
    }
    @mkdir(RB_PREVIEW_DIR, 0755, true);
    rb_preview_prune();
    $id = rb_new_op_id();
    rb_json_write(rb_preview_file_path($id, '.request.json'), array(
        'job' => $job, 'repo' => rb_repo($cfg, $job['repo']) ?: array('type' => ''), 'settings' => $cfg['settings'],
    ));
    rb_spawn(array($id), 'preview.php');
    return array('id' => $id);
}

function rb_api_job_tree(array $p)
{
    $cfg = rb_config_load();
    $errors = array();
    $job = rb_clean_job(rb_pj($p, 'job'), array_column($cfg['repos'], 'id'), $errors);
    $repo = rb_repo($cfg, $job['repo']) ?: array('type' => '');
    $path = (string)rb_p($p, 'path', '');
    $src = '';
    if ($path !== '') {
        $path = $path[0] === '/' ? rb_path_clean($path) : '';
        $src = $path !== '' ? rb_preview_source_of($job, $path) : null;
        if ($src === null || $path === '') {
            throw new RbApiError('That folder is not one this job backs up.');
        }
        if (in_array('.zfs', explode('/', substr($path, strlen($src))), true)) {
            throw new RbApiError('ZFS snapshot folders are not opened here.');
        }
    }
    try {
        return rb_preview_folder($job, $repo, $cfg['settings'], $src, $path);
    } catch (RuntimeException $e) {
        throw new RbApiError($e->getMessage());
    }
}

function rb_api_job_preview_status(array $p)
{
    $id = (string)rb_p($p, 'id');
    if (!preg_match('/^[0-9a-z-]{6,40}$/', $id)) {
        throw new RbApiError('There is no such preview.');
    }
    $s = rb_json_read(rb_preview_file_path($id));
    if (!$s) {
        if (file_exists(rb_preview_file_path($id, '.request.json'))) {
            return array('status' => 'starting');
        }
        throw new RbApiError('The preview is gone - start it again.');
    }
    $alive = function_exists('posix_kill') ? posix_kill((int)$s['pid'], 0) : file_exists('/proc/' . (int)$s['pid']);
    if ($s['status'] === 'running' && !$alive) {
        $s = array('status' => 'error', 'error' => 'The preview stopped unexpectedly.');
    }
    return $s;
}

function rb_api_job_preview_stop(array $p)
{
    $id = (string)rb_p($p, 'id');
    if (preg_match('/^[0-9a-z-]{6,40}$/', $id) && file_exists(rb_preview_file_path($id))) {
        @touch(rb_preview_file_path($id, '.cancel'));
    }
    return array();
}

function rb_api_job_delete(array $p)
{
    $id = (string)rb_p($p, 'id');
    $result = rb_config_change(function (&$cfg) use ($id) {
        $job = rb_need_job($cfg, $id);
        array_splice($cfg['jobs'], rb_find($cfg['jobs'], $id), 1);
        return array('deleted' => $job['name'], 'settings' => $cfg['settings']);
    });
    rb_state_update($result['settings'], function (&$state) use ($id) {
        unset($state['jobs'][$id]);
    });
    unset($result['settings']);
    return $result;
}

function rb_api_job_run(array $p)
{
    $cfg = rb_config_load();
    $job = rb_need_job($cfg, rb_p($p, 'id'));
    $repo = rb_need_repo($cfg, $job['repo']);
    rb_restic_ready($cfg, $repo);
    if (!rb_array_started()) {
        throw new RbApiError('The array is not started.');
    }
    foreach (rb_ops_running() as $op) {
        if ($op['job'] === $job['id']) {
            throw new RbApiError('This job is running already.');
        }
    }
    if (rb_repo_busy($repo['id'])) {
        rb_state_update($cfg['settings'], function (&$state) use ($job) {
            $state['jobs'][$job['id']]['queued'] = time();
        });
        return array('queued' => true);
    }
    rb_spawn(array('backup', $job['id'], 'manual'));
    return array('started' => true);
}

function rb_api_job_forget(array $p)
{
    $cfg = rb_config_load();
    $job = rb_need_job($cfg, rb_p($p, 'id'));
    $repo = rb_need_repo($cfg, $job['repo']);
    rb_restic_ready($cfg, $repo);
    if (rb_repo_busy($repo['id'])) {
        throw new RbApiError('An operation on this repository is running. Try again when it has finished.');
    }
    rb_spawn(array('forget', $job['id'], 'manual'));
    return array('started' => true);
}

function rb_api_retention_preview(array $p)
{
    $cfg = rb_config_load();
    $in = rb_pj($p, 'job');
    $job = rb_need_job($cfg, $in['id'] ?? '');
    $errors = array();
    $preview = rb_clean_job(array_merge($job, array('retention' => $in['retention'] ?? array())),
                            array_column($cfg['repos'], 'id'), $errors);
    $repo = rb_need_repo($cfg, $job['repo']);
    $args = rb_forget_args($preview, true);
    $args[] = '--no-lock';
    $out = rb_api_restic($cfg, $repo, $args);
    $groups = json_decode(implode("\n", $out), true);
    $keep = array();
    $remove = array();
    foreach (is_array($groups) ? $groups : array() as $g) {
        $reasons = array();
        foreach ($g['reasons'] ?? array() as $r) {
            $reasons[$r['snapshot']['id'] ?? ''] = $r['matches'] ?? array();
        }
        foreach ($g['keep'] ?? array() as $s) {
            $keep[] = array('id' => $s['id'], 'time' => $s['time'], 'reasons' => $reasons[$s['id']] ?? array());
        }
        foreach ($g['remove'] ?? array() as $s) {
            $remove[] = array('id' => $s['id'], 'time' => $s['time']);
        }
    }
    return array('keep' => $keep, 'remove' => $remove);
}

function rb_api_snapshots(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'repo'));
    $cache = rb_cache_dir() . '/snapshots-' . $repo['id'] . '.json';
    $fresh = !rb_bool(rb_p($p, 'refresh', false)) && @filemtime($cache) > time() - 600;
    $list = $fresh ? rb_json_read($cache) : null;
    if (!is_array($list)) {
        $out = rb_api_restic($cfg, $repo, array('snapshots', '--json', '--no-lock'));
        $raw = json_decode(implode("\n", $out), true);
        $list = array();
        foreach (is_array($raw) ? $raw : array() as $s) {
            $jobId = '';
            foreach ($s['tags'] ?? array() as $t) {
                if (strpos($t, 'job:') === 0) {
                    $jobId = substr($t, 4);
                }
            }
            $list[] = array(
                'id'       => $s['id'],
                'short_id' => $s['short_id'] ?? substr($s['id'], 0, 8),
                'time'     => strtotime($s['time']),
                'hostname' => $s['hostname'] ?? '',
                'paths'    => $s['paths'] ?? array(),
                'tags'     => array_values(array_filter($s['tags'] ?? array(), function ($t) {
                    return strpos($t, 'job:') !== 0;
                })),
                'job'      => $jobId,
                'summary'  => isset($s['summary']) ? array(
                    'added'     => $s['summary']['data_added_packed'] ?? ($s['summary']['data_added'] ?? 0),
                    'size'      => $s['summary']['total_bytes_processed'] ?? 0,
                    'files'     => $s['summary']['total_files_processed'] ?? 0,
                    'new'       => $s['summary']['files_new'] ?? 0,
                    'changed'   => $s['summary']['files_changed'] ?? 0,
                    'duration'  => isset($s['summary']['backup_start'], $s['summary']['backup_end'])
                        ? strtotime($s['summary']['backup_end']) - strtotime($s['summary']['backup_start']) : null,
                ) : null,
            );
        }
        usort($list, function ($a, $b) {
            return $b['time'] - $a['time'];
        });
        rb_json_write($cache, $list);
    }
    $names = array();
    foreach ($cfg['jobs'] as $job) {
        $names[$job['id']] = $job['name'];
    }
    return array('snapshots' => $list, 'job_names' => $names, 'cached_at' => (int)@filemtime($cache));
}

function rb_api_ls(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'repo'));
    $snap = rb_need_snapshot_id(rb_p($p, 'snapshot'));
    $path = rb_need_snapshot_path(rb_p($p, 'path', '/'));
    $dir = rb_cache_dir() . '/ls/' . $snap;
    $cache = $dir . '/' . md5($path) . '.json';
    $entries = rb_json_read($cache);
    if (!is_array($entries)) {
        $entries = null;
        if (rb_browse_available()) {
            $s = rb_browse_status($repo['id']);
            if ($s === null) {
                rb_restic_ready($cfg, $repo);
                $why = rb_repo_reachable($repo, 1, 0);
                if ($why !== '') {
                    throw new RbApiError($why);
                }
                $s = rb_browse_start($repo);
            }
            if ($s['status'] === 'starting') {
                return array('path' => $path, 'pending' => true);
            }
            if ($s['status'] === 'ready') {
                $entries = rb_browse_list($repo['id'], $snap, $path);
                if ($entries === null) {
                    rb_browse_stop($repo['id']);
                }
            }
        }
        if ($entries === null) {
            $entries = rb_api_ls_restic($cfg, $repo, $snap, $path);
        }
        usort($entries, function ($a, $b) {
            $da = $a['type'] === 'dir' ? 0 : 1;
            $db = $b['type'] === 'dir' ? 0 : 1;
            return $da !== $db ? $da - $db : strnatcasecmp($a['name'], $b['name']);
        });
        @mkdir($dir, 0755, true);
        rb_json_write($cache, $entries);
        rb_ls_cache_trim();
    }
    return array('path' => $path, 'entries' => $entries);
}

function rb_api_ls_restic(array $cfg, array $repo, $snap, $path)
{
    $out = rb_api_restic($cfg, $repo, array('ls', '--json', '--no-lock', $snap, $path));
    $entries = array();
    foreach (rb_json_lines($out) as $n) {
        if (($n['message_type'] ?? $n['struct_type'] ?? '') !== 'node' || ($n['path'] ?? '') === $path) {
            continue;
        }
        $entries[] = array(
            'name'  => $n['name'],
            'type'  => $n['type'],
            'path'  => $n['path'],
            'size'  => $n['size'] ?? null,
            'mtime' => isset($n['mtime']) ? strtotime($n['mtime']) : null,
            'perm'  => $n['permissions'] ?? '',
        );
    }
    return $entries;
}

function rb_ls_cache_trim()
{
    $dirs = glob(rb_cache_dir() . '/ls/*', GLOB_ONLYDIR) ?: array();
    $sizes = array();
    $total = 0;
    foreach ($dirs as $d) {
        $s = 0;
        foreach (glob("$d/*.json") ?: array() as $f) {
            $s += filesize($f);
        }
        $sizes[$d] = array(filemtime($d), $s);
        $total += $s;
    }
    if ($total < 64 * 1048576) {
        return;
    }
    uasort($sizes, function ($a, $b) {
        return $a[0] - $b[0];
    });
    foreach ($sizes as $d => $info) {
        if ($total < 48 * 1048576) {
            break;
        }
        exec('rm -rf ' . escapeshellarg($d));
        $total -= $info[1];
    }
}

function rb_api_find(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'repo'));
    $pattern = (string)rb_p($p, 'pattern');
    if ($pattern === '' || strpbrk($pattern, "\0\n\r") !== false) {
        throw new RbApiError('Enter a file name or a pattern such as *.jpg');
    }
    $args = array('find', '--json', '--no-lock');
    if (rb_p($p, 'snapshot') !== '') {
        array_push($args, '--snapshot', rb_need_snapshot_id(rb_p($p, 'snapshot')));
    }
    if (rb_bool(rb_p($p, 'ignore_case', true))) {
        $args[] = '--ignore-case';
    }
    $args[] = '--';
    $args[] = $pattern;
    $out = rb_api_restic($cfg, $repo, $args);
    $raw = json_decode(implode("\n", $out), true);
    $results = array();
    $count = 0;
    foreach (is_array($raw) ? $raw : array() as $group) {
        $matches = array();
        foreach ($group['matches'] ?? array() as $m) {
            if ($count++ >= 2000) {
                break;
            }
            $matches[] = array('path' => $m['path'], 'type' => $m['type'], 'size' => $m['size'] ?? null,
                               'mtime' => isset($m['mtime']) ? strtotime($m['mtime']) : null);
        }
        $results[] = array('snapshot' => $group['snapshot'], 'hits' => $group['hits'] ?? count($matches), 'matches' => $matches);
    }
    return array('results' => $results, 'truncated' => $count > 2000);
}

function rb_api_diff(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'repo'));
    $a = rb_need_snapshot_id(rb_p($p, 'from'));
    $b = rb_need_snapshot_id(rb_p($p, 'to'));
    $out = rb_api_restic($cfg, $repo, array('diff', '--json', '--no-lock', $a, $b));
    $changes = array();
    $stats = null;
    $total = 0;
    foreach (rb_json_lines($out) as $j) {
        if (($j['message_type'] ?? '') === 'change') {
            if ($total++ < 5000) {
                $changes[] = array('path' => $j['path'], 'modifier' => $j['modifier']);
            }
        } elseif (($j['message_type'] ?? '') === 'statistics') {
            $stats = $j;
        }
    }
    return array('changes' => $changes, 'total' => $total, 'statistics' => $stats);
}

function rb_api_snapshot_forget(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'repo'));
    if ($repo['append_only']) {
        throw new RbApiError('The repository is append-only; snapshots can only be removed on the server.');
    }
    $ids = array_map('rb_need_snapshot_id', rb_pj($p, 'ids'));
    if (!$ids) {
        throw new RbApiError('No snapshots chosen.');
    }
    $lock = rb_api_lock($repo);
    rb_api_restic($cfg, $repo, array_merge(array('forget', '--retry-lock', '1m'), $ids));
    flock($lock, LOCK_UN);
    @unlink(rb_cache_dir() . '/snapshots-' . $repo['id'] . '.json');
    return array('removed' => count($ids));
}

function rb_api_restore_start(array $p)
{
    $cfg = rb_config_load();
    $repo = rb_need_repo($cfg, rb_p($p, 'repo'));
    rb_restic_ready($cfg, $repo);
    $snap = rb_need_snapshot_id(rb_p($p, 'snapshot'));
    $items = array();
    foreach (rb_pj($p, 'items') as $item) {
        $path = rb_need_snapshot_path($item);
        if (!in_array($path, $items, true)) {
            $items[] = $path;
        }
    }
    if (!$items || count($items) > 1000) {
        throw new RbApiError($items ? 'Choose at most 1000 items at a time.' : 'Choose what to restore.');
    }
    $target = rb_p($p, 'target') === 'original' ? 'original' : 'path';
    $targetPath = '';
    if ($target === 'path') {
        $targetPath = rb_abs_path(rb_p($p, 'target_path'));
        if ($targetPath === '' || !rb_under($targetPath, '/mnt') || preg_match('#^/mnt(/(user|disks|remotes|[^/]+))?$#', $targetPath)) {
            throw new RbApiError('Restore into a folder on a share, a disk or a pool, such as /mnt/user/restore.');
        }
    } else {
        foreach ($items as $path) {
            if (!(rb_under($path, '/mnt') || rb_under($path, '/boot')) && $path !== '/') {
                throw new RbApiError("$path did not come from /mnt or /boot; restore it into a folder instead.");
            }
        }
    }
    if (!rb_array_started()) {
        throw new RbApiError('The array is not started.');
    }
    $overwrite = (string)rb_p($p, 'overwrite', $target === 'original' ? 'if-changed' : 'always');
    if (!in_array($overwrite, array('always', 'if-changed', 'if-newer', 'never'), true)) {
        throw new RbApiError('Unknown overwrite setting');
    }
    $op = rb_op_running_on($repo['id'], array('prune', 'check', 'forget'));
    if ($op) {
        throw new RbApiError(ucfirst($op['kind']) . ' is running on this repository; restore when it has finished.');
    }
    $dir = RB_RUN_DIR . '/requests';
    if (!is_dir($dir)) {
        @mkdir($dir, 0700, true);
    }
    $file = $dir . '/' . bin2hex(random_bytes(8)) . '.json';
    rb_json_write($file, array('snapshot' => $snap, 'items' => $items, 'target' => $target, 'target_path' => $targetPath,
                               'overwrite' => $overwrite, 'delete' => rb_bool(rb_p($p, 'delete', false))), 0600);
    rb_spawn(array('restore', $repo['id'], $file));
    return array('started' => true);
}

function rb_api_consistency_detect(array $p)
{
    $sources = array();
    foreach (rb_pj($p, 'sources') as $src) {
        $path = rb_abs_path($src);
        if ($path !== '' && (rb_under($path, '/mnt') || rb_under($path, '/boot'))) {
            $sources[] = $path;
        }
    }
    if (!$sources) {
        return array('docker' => true, 'libvirt' => true, 'containers' => array(), 'vms' => array(),
                     'zfs' => array('ok' => false, 'error' => 'Add folders to back up first.', 'datasets' => array()));
    }
    return rb_consistency_detect($sources);
}

function rb_api_mkdir(array $p)
{
    $parent = rb_abs_path(rb_p($p, 'path', ''));
    $name = trim((string)rb_p($p, 'name', ''));
    if ($name === '' || $name === '.' || $name === '..' || strpos($name, '/') !== false || strlen($name) > 255
        || preg_match('/[\x00-\x1f\x7f]/', $name)) {
        throw new RbApiError('A folder name cannot contain a slash, and cannot be . or ..');
    }
    if (!rb_under($parent, '/mnt') || $parent === '/mnt' || !is_dir($parent)) {
        throw new RbApiError('New folders can be made in a share, pool or disk under /mnt.');
    }
    if (in_array(rb_fs_type($parent), array('', 'rootfs', 'tmpfs', 'ramfs', 'devtmpfs'), true)) {
        throw new RbApiError("$parent is in memory, not on a disk: a folder made there would be gone after a reboot.");
    }
    $path = rtrim($parent, '/') . '/' . $name;
    if (file_exists($path)) {
        throw new RbApiError("$path exists already.");
    }
    if (!rb_mkdir_like_parent($path)) {
        throw new RbApiError("Could not make $path.");
    }
    return array('path' => $path);
}

function rb_api_browse(array $p)
{
    $path = rb_abs_path(rb_p($p, 'path', '/mnt'));
    if ($path === '' || $path === '/') {
        return array('path' => '/', 'entries' => array(
            array('name' => 'mnt', 'path' => '/mnt', 'dir' => true),
            array('name' => 'boot', 'path' => '/boot', 'dir' => true),
        ));
    }
    if (!(rb_under($path, '/mnt') || rb_under($path, '/boot'))) {
        throw new RbApiError('Only folders under /mnt and /boot can be chosen.');
    }
    while (!is_dir($path) && $path !== '/mnt' && $path !== '/boot') {
        $path = dirname($path);
    }
    if (!is_dir($path)) {
        return rb_api_browse(array('path' => '/'));
    }
    $files = rb_bool(rb_p($p, 'files', false));
    $dh = @opendir($path);
    if (!$dh) {
        throw new RbApiError("Cannot open $path");
    }
    $entries = array();
    while (($e = readdir($dh)) !== false && count($entries) < 5000) {
        if ($e === '.' || $e === '..') {
            continue;
        }
        $full = "$path/$e";
        $isDir = is_dir($full);
        if ($isDir || $files) {
            $entries[] = array('name' => $e, 'path' => $full, 'dir' => $isDir);
        }
    }
    closedir($dh);
    usort($entries, function ($a, $b) {
        return $a['dir'] !== $b['dir'] ? ($a['dir'] ? -1 : 1) : strnatcasecmp($a['name'], $b['name']);
    });
    return array('path' => $path, 'entries' => $entries);
}
