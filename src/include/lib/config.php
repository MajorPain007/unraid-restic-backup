<?php

require_once __DIR__ . '/common.php';
require_once __DIR__ . '/cron.php';

define('RB_CONFIG_FILE', RB_CONFIG_DIR . '/config.json');
define('RB_SECRETS_DIR', RB_CONFIG_DIR . '/secrets');
define('RB_SSH_DIR', RB_CONFIG_DIR . '/ssh');
define('RB_CONFIG_VERSION', 1);

function rb_default_settings()
{
    return array(
        'data_dir'     => '/mnt/user/appdata/' . RB_NAME,
        'host'         => '',
        'nice'         => 10,
        'io_priority'  => 'low',
        'start_delay'  => 5,
        'parity_pause' => true,
        'history_days' => 90,
        'show_in_menu' => false,
    );
}

function rb_default_schedule($cron = '0 3 * * *')
{
    return array('mode' => 'cron', 'cron' => $cron, 'every' => 24, 'unit' => 'hours', 'catch_up' => true);
}

function rb_default_repo()
{
    return array(
        'id'      => '',
        'name'    => '',
        'type'    => 'sftp',
        'sftp'    => array('user' => '', 'host' => '', 'port' => 22, 'path' => ''),
        'local'   => array('path' => ''),
        'rest'    => array('url' => '', 'user' => '', 'cacert' => ''),
        'options' => array('compression' => 'auto', 'limit_upload' => 0, 'limit_download' => 0,
                           'connections' => 0, 'extra_flags' => array()),
        'maintenance' => array(
            'prune' => array('enabled' => true, 'schedule' => rb_default_schedule('0 4 * * 0'), 'max_unused' => '10%'),
            'check' => array('enabled' => true, 'schedule' => rb_default_schedule('0 5 1 * *'),
                             'read_data' => 'subset', 'subset' => '5%'),
        ),
        'unlock_stale' => true,
        'append_only'  => false,
    );
}

function rb_default_job()
{
    return array(
        'id'       => '',
        'name'     => '',
        'enabled'  => true,
        'repo'     => '',
        'sources'  => array(),
        'excludes' => array(),
        'iexcludes' => array(),
        'exclude_caches' => true,
        'exclude_larger_than' => '',
        'one_file_system' => false,
        'tags'     => array(),
        'schedule' => rb_default_schedule(),
        'retention' => array('enabled' => true, 'last' => 0, 'hourly' => 0, 'daily' => 7, 'weekly' => 4,
                             'monthly' => 12, 'yearly' => 0, 'within' => ''),
        'consistency' => array('mode' => 'none', 'containers' => array(), 'vms' => array(), 'skip' => array(),
                               'timeout' => 120),
        'hooks'    => array('before' => '', 'after' => ''),
        'notify'   => array('success' => false, 'warning' => true, 'error' => true, 'healthcheck' => ''),
        'advanced' => array('host' => '', 'read_concurrency' => 0, 'skip_if_unchanged' => false, 'extra_flags' => array()),
    );
}

function rb_config_load()
{
    $raw = rb_json_read(RB_CONFIG_FILE, array());
    $cfg = array(
        'version'  => RB_CONFIG_VERSION,
        'settings' => rb_clean_settings(isset($raw['settings']) ? $raw['settings'] : array(), $ignored),
        'repos'    => array(),
        'jobs'     => array(),
    );
    foreach ((isset($raw['repos']) && is_array($raw['repos'])) ? $raw['repos'] : array() as $r) {
        $errors = array();
        $repo = rb_clean_repo(is_array($r) ? $r : array(), $errors);
        if ($repo['id'] !== '') {
            $cfg['repos'][] = $repo;
        }
    }
    $repoIds = array_column($cfg['repos'], 'id');
    foreach ((isset($raw['jobs']) && is_array($raw['jobs'])) ? $raw['jobs'] : array() as $j) {
        $errors = array();
        $job = rb_clean_job(is_array($j) ? $j : array(), $repoIds, $errors);
        if ($job['id'] !== '') {
            $cfg['jobs'][] = $job;
        }
    }
    return $cfg;
}

function rb_config_save(array $cfg)
{
    $cfg['version'] = RB_CONFIG_VERSION;
    return rb_json_write(RB_CONFIG_FILE, $cfg);
}

function rb_config_lock()
{
    if (!is_dir(RB_RUN_DIR)) {
        @mkdir(RB_RUN_DIR, 0755, true);
    }
    $fh = fopen(RB_RUN_DIR . '/config.lock', 'c');
    if ($fh) {
        flock($fh, LOCK_EX);
    }
    return $fh;
}

function rb_find(array $list, $id)
{
    foreach ($list as $i => $item) {
        if ($item['id'] === $id) {
            return $i;
        }
    }
    return null;
}

function rb_repo(array $cfg, $id)
{
    $i = rb_find($cfg['repos'], $id);
    return $i === null ? null : $cfg['repos'][$i];
}

function rb_job(array $cfg, $id)
{
    $i = rb_find($cfg['jobs'], $id);
    return $i === null ? null : $cfg['jobs'][$i];
}

function rb_secret_file($repoId, $kind)
{
    return RB_SECRETS_DIR . '/' . $repoId . '.' . $kind;
}

function rb_secret_get($repoId, $kind)
{
    $v = @file_get_contents(rb_secret_file($repoId, $kind));
    return $v === false ? '' : rtrim($v, "\r\n");
}

function rb_secret_set($repoId, $kind, $value)
{
    rb_private_dir(RB_SECRETS_DIR);
    return rb_file_write(rb_secret_file($repoId, $kind), $value . "\n", 0600);
}

function rb_private_dir($dir)
{
    if (!is_dir($dir)) {
        @mkdir($dir, 0700, true);
    }
    if (!is_file("$dir/.gitignore")) {
        @file_put_contents("$dir/.gitignore", "*\n");
    }
}

function rb_secret_has($repoId, $kind)
{
    return @filesize(rb_secret_file($repoId, $kind)) > 1;
}

function rb_ssh_key_file($repoId)
{
    return RB_SSH_DIR . '/' . $repoId;
}

function rb_known_hosts_file()
{
    return RB_SSH_DIR . '/known_hosts';
}

function rb_generate_password()
{
    $alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    $out = '';
    for ($i = 0; $i < 32; $i++) {
        $out .= $alphabet[random_int(0, 61)];
    }
    return $out;
}

function rb_has_control($s)
{
    return preg_match('/[\x00-\x1f\x7f]/', $s) === 1;
}

function rb_text($v, $max = 255)
{
    if (!is_scalar($v)) {
        return '';
    }
    $s = trim((string)$v);
    return function_exists('mb_substr') ? mb_substr($s, 0, $max) : substr($s, 0, $max);
}

function rb_bool($v)
{
    return $v === true || $v === 1 || $v === '1' || $v === 'true' || $v === 'yes' || $v === 'on';
}

function rb_count($v, $max = 100000)
{
    return (is_numeric($v) && (int)$v > 0) ? min((int)$v, $max) : 0;
}

function rb_abs_path($p)
{
    $p = is_scalar($p) ? trim((string)$p) : '';
    if ($p === '' || $p[0] !== '/' || rb_has_control($p)) {
        return '';
    }
    $parts = array();
    foreach (explode('/', $p) as $seg) {
        if ($seg === '' || $seg === '.') {
            continue;
        }
        if ($seg === '..') {
            return '';
        }
        $parts[] = $seg;
    }
    return '/' . implode('/', $parts);
}

function rb_under($path, $dir)
{
    return $path === $dir || strpos($path, rtrim($dir, '/') . '/') === 0;
}

function rb_list($v, $max = 500)
{
    if (is_string($v)) {
        $v = preg_split('/\r?\n/', $v);
    }
    if (!is_array($v)) {
        return array();
    }
    $out = array();
    foreach ($v as $item) {
        $s = rb_text($item, 4096);
        if ($s !== '' && !in_array($s, $out, true)) {
            $out[] = $s;
        }
    }
    return array_slice($out, 0, $max);
}

function rb_clean_schedule($in, $label, array &$errors)
{
    $s = rb_default_schedule();
    if (!is_array($in)) {
        return $s;
    }
    $mode = rb_text($in['mode'] ?? 'cron');
    $s['mode'] = in_array($mode, array('off', 'cron', 'interval'), true) ? $mode : 'cron';
    $s['cron'] = preg_replace('/\s+/', ' ', rb_text($in['cron'] ?? $s['cron'], 120));
    $s['every'] = max(1, rb_count($in['every'] ?? $s['every'], 100000));
    $unit = rb_text($in['unit'] ?? 'hours');
    $s['unit'] = in_array($unit, array('minutes', 'hours', 'days'), true) ? $unit : 'hours';
    $s['catch_up'] = array_key_exists('catch_up', $in) ? rb_bool($in['catch_up']) : true;
    if ($s['mode'] === 'cron') {
        $problem = RbCron::problem($s['cron']);
        if ($problem !== '') {
            $errors[] = "$label: $problem";
        }
    } elseif ($s['mode'] === 'interval' && $s['unit'] === 'minutes' && $s['every'] < 5) {
        $errors[] = "$label: run at most every 5 minutes";
    }
    return $s;
}

function rb_clean_settings($in, &$errors)
{
    $errors = is_array($errors) ? $errors : array();
    $d = rb_default_settings();
    if (!is_array($in)) {
        return $d;
    }
    $dir = rb_abs_path($in['data_dir'] ?? $d['data_dir']);
    if ($dir === '' || !rb_under($dir, '/mnt') || in_array($dir, array('/mnt', '/mnt/user', '/mnt/disks'), true)) {
        $errors[] = 'Data folder: choose a folder on a disk or pool, such as /mnt/user/appdata/' . RB_NAME;
        $dir = $d['data_dir'];
    }
    $d['data_dir'] = $dir;
    $host = rb_text($in['host'] ?? '');
    if ($host !== '' && !preg_match('/^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$/', $host)) {
        $errors[] = 'Host name: letters, digits, dots, dashes and underscores only';
        $host = '';
    }
    $d['host'] = $host;
    $d['nice'] = min(19, rb_count($in['nice'] ?? $d['nice'], 19));
    $io = rb_text($in['io_priority'] ?? 'low');
    $d['io_priority'] = in_array($io, array('low', 'idle', 'normal'), true) ? $io : 'low';
    $d['start_delay'] = min(120, rb_count($in['start_delay'] ?? $d['start_delay']));
    $d['parity_pause'] = array_key_exists('parity_pause', $in) ? rb_bool($in['parity_pause']) : true;
    $d['history_days'] = max(7, min(3650, rb_count($in['history_days'] ?? 90) ?: 90));
    $d['show_in_menu'] = rb_bool($in['show_in_menu'] ?? false);
    return $d;
}

function rb_clean_flags($in, $label, array &$errors)
{
    $out = array();
    foreach (rb_list($in, 50) as $flag) {
        if (!preg_match('/^--[a-z][a-z0-9-]*(=.*)?$/', $flag)) {
            $errors[] = "$label: \"$flag\" is not a long option like --name or --name=value";
            continue;
        }
        $out[] = $flag;
    }
    return $out;
}

function rb_clean_repo($in, array &$errors)
{
    $r = rb_default_repo();
    $r['id'] = preg_match('/^r[0-9a-f]{8}$/', $in['id'] ?? '') ? $in['id'] : '';
    $r['name'] = rb_text($in['name'] ?? '', 60);
    if ($r['name'] === '') {
        $errors[] = 'Name: give the repository a name';
    }
    $type = rb_text($in['type'] ?? 'sftp');
    $r['type'] = in_array($type, array('sftp', 'local', 'rest'), true) ? $type : 'sftp';

    $sftp = is_array($in['sftp'] ?? null) ? $in['sftp'] : array();
    $r['sftp'] = array(
        'user' => rb_text($sftp['user'] ?? '', 64),
        'host' => rb_text($sftp['host'] ?? '', 253),
        'port' => (int)($sftp['port'] ?? 22),
        'path' => rb_text($sftp['path'] ?? '', 1024),
    );
    $local = is_array($in['local'] ?? null) ? $in['local'] : array();
    $r['local'] = array('path' => rb_abs_path($local['path'] ?? ''));
    $rest = is_array($in['rest'] ?? null) ? $in['rest'] : array();
    $r['rest'] = array(
        'url'    => rb_text($rest['url'] ?? '', 1024),
        'user'   => rb_text($rest['user'] ?? '', 128),
        'cacert' => rb_abs_path($rest['cacert'] ?? ''),
    );

    if ($r['type'] === 'sftp') {
        $s = $r['sftp'];
        if (!preg_match('/^[A-Za-z0-9._][A-Za-z0-9._-]{0,63}$/', $s['user'])) {
            $errors[] = 'SSH user: letters, digits, dots, dashes and underscores';
        }
        if (!preg_match('/^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$/', $s['host'])
            && !preg_match('/^[0-9A-Fa-f:]{2,39}$/', $s['host'])) {
            $errors[] = 'SSH host: a host name or an IP address';
        }
        if ($s['port'] < 1 || $s['port'] > 65535) {
            $errors[] = 'SSH port: 1-65535';
            $r['sftp']['port'] = 22;
        }
        if ($s['path'] === '' || rb_has_control($s['path'])) {
            $errors[] = 'Path on the server: where the repository lives, e.g. backups/tower';
        }
    } elseif ($r['type'] === 'local') {
        $p = $r['local']['path'];
        if ($p === '' || !rb_under($p, '/mnt') || in_array($p, array('/mnt', '/mnt/user', '/mnt/disks', '/mnt/remotes'), true)) {
            $errors[] = 'Folder: a folder on a disk, pool or unassigned device, such as /mnt/disks/backup/restic';
        } elseif (preg_match('#^/mnt/[^/]+$#', $p)) {
            $errors[] = 'Folder: use a folder inside ' . $p . ', not the disk itself';
        }
    } else {
        $u = parse_url($r['rest']['url']);
        if (!$u || !in_array($u['scheme'] ?? '', array('http', 'https'), true) || empty($u['host'])) {
            $errors[] = 'Server URL: http:// or https:// followed by the server, e.g. https://backup.lan:8000/tower';
        } elseif (isset($u['user']) || isset($u['pass'])) {
            $errors[] = 'Server URL: put the user name and password in their own fields, not in the URL';
        }
        if ($r['rest']['user'] !== '' && !preg_match('/^[^\s:]{1,128}$/', $r['rest']['user'])) {
            $errors[] = 'REST user: no spaces or colons';
        }
    }

    $o = is_array($in['options'] ?? null) ? $in['options'] : array();
    $comp = rb_text($o['compression'] ?? 'auto');
    $r['options'] = array(
        'compression'    => in_array($comp, array('auto', 'off', 'fastest', 'better', 'max'), true) ? $comp : 'auto',
        'limit_upload'   => rb_count($o['limit_upload'] ?? 0, 10000000),
        'limit_download' => rb_count($o['limit_download'] ?? 0, 10000000),
        'connections'    => rb_count($o['connections'] ?? 0, 64),
        'extra_flags'    => rb_clean_flags($o['extra_flags'] ?? array(), 'Extra options', $errors),
    );

    $m = is_array($in['maintenance'] ?? null) ? $in['maintenance'] : array();
    $p = is_array($m['prune'] ?? null) ? $m['prune'] : array();
    $c = is_array($m['check'] ?? null) ? $m['check'] : array();
    $defaults = rb_default_repo()['maintenance'];
    $maxUnused = rb_text($p['max_unused'] ?? '10%', 20);
    if (!preg_match('/^(\d{1,3}(\.\d+)?%|\d+[KMGT]?|unlimited)$/i', $maxUnused)) {
        $errors[] = 'Prune: "unused space" takes a percentage like 10%, a size like 5G, or unlimited';
        $maxUnused = '10%';
    }
    $readData = rb_text($c['read_data'] ?? 'subset');
    $subset = rb_text($c['subset'] ?? '5%', 20);
    if (!preg_match('/^\d{1,3}(\.\d+)?%$/', $subset) || (float)$subset <= 0 || (float)$subset > 100) {
        $errors[] = 'Check: the share of data to read is a percentage between 0 and 100';
        $subset = '5%';
    }
    $r['maintenance'] = array(
        'prune' => array(
            'enabled'    => array_key_exists('enabled', $p) ? rb_bool($p['enabled']) : true,
            'schedule'   => rb_clean_schedule($p['schedule'] ?? $defaults['prune']['schedule'], 'Prune schedule', $errors),
            'max_unused' => $maxUnused,
        ),
        'check' => array(
            'enabled'   => array_key_exists('enabled', $c) ? rb_bool($c['enabled']) : true,
            'schedule'  => rb_clean_schedule($c['schedule'] ?? $defaults['check']['schedule'], 'Check schedule', $errors),
            'read_data' => in_array($readData, array('none', 'subset', 'all'), true) ? $readData : 'subset',
            'subset'    => $subset,
        ),
    );
    $r['unlock_stale'] = array_key_exists('unlock_stale', $in) ? rb_bool($in['unlock_stale']) : true;
    $r['append_only'] = rb_bool($in['append_only'] ?? false);
    return $r;
}

function rb_clean_job($in, array $repoIds, array &$errors)
{
    $j = rb_default_job();
    $j['id'] = preg_match('/^j[0-9a-f]{8}$/', $in['id'] ?? '') ? $in['id'] : '';
    $j['name'] = rb_text($in['name'] ?? '', 60);
    if ($j['name'] === '') {
        $errors[] = 'Name: give the job a name';
    }
    $j['enabled'] = array_key_exists('enabled', $in) ? rb_bool($in['enabled']) : true;
    $j['repo'] = rb_text($in['repo'] ?? '', 20);
    if (!in_array($j['repo'], $repoIds, true)) {
        $errors[] = 'Repository: choose where the backup goes';
    }

    foreach (rb_list($in['sources'] ?? array(), 100) as $src) {
        $p = rb_abs_path($src);
        if ($p === '' || !(rb_under($p, '/mnt') || rb_under($p, '/boot')) || $p === '/mnt') {
            $errors[] = "Sources: \"$src\" - choose folders under /mnt or /boot";
            continue;
        }
        if (!in_array($p, $j['sources'], true)) {
            $j['sources'][] = $p;
        }
    }
    if (!$j['sources']) {
        $errors[] = 'Sources: add at least one folder to back up';
    }
    foreach (array('excludes', 'iexcludes') as $key) {
        $j[$key] = rb_list($in[$key] ?? array(), 1000);
    }
    $j['exclude_caches'] = array_key_exists('exclude_caches', $in) ? rb_bool($in['exclude_caches']) : true;
    $elt = rb_text($in['exclude_larger_than'] ?? '', 20);
    if ($elt !== '' && !preg_match('/^\d+(\.\d+)?[KMGT]?$/i', $elt)) {
        $errors[] = 'Skip files larger than: a size like 500M or 4G';
        $elt = '';
    }
    $j['exclude_larger_than'] = $elt;
    $j['one_file_system'] = rb_bool($in['one_file_system'] ?? false);
    foreach (rb_list($in['tags'] ?? array(), 20) as $tag) {
        if (!preg_match('/^[A-Za-z0-9._:@+=\/-]{1,64}$/', $tag)) {
            $errors[] = "Tags: \"$tag\" - letters, digits and . _ : @ + = / - only";
        } elseif (strpos($tag, 'job:') === 0) {
            $errors[] = "Tags: \"$tag\" - tags starting with job: are the plugin's own";
        } else {
            $j['tags'][] = $tag;
        }
    }

    $j['schedule'] = rb_clean_schedule($in['schedule'] ?? null, 'Schedule', $errors);

    $ret = is_array($in['retention'] ?? null) ? $in['retention'] : array();
    $j['retention']['enabled'] = array_key_exists('enabled', $ret) ? rb_bool($ret['enabled']) : true;
    foreach (array('last', 'hourly', 'daily', 'weekly', 'monthly', 'yearly') as $k) {
        $j['retention'][$k] = rb_count($ret[$k] ?? $j['retention'][$k]);
    }
    $within = strtolower(rb_text($ret['within'] ?? '', 20));
    if ($within !== '' && !preg_match('/^(\d+y)?(\d+m)?(\d+d)?(\d+h)?$/', $within)) {
        $errors[] = 'Keep everything from the last: a duration like 14d, 6m or 1y2m';
        $within = '';
    }
    $j['retention']['within'] = $within;
    if ($j['retention']['enabled'] && $within === ''
        && !array_sum(array_intersect_key($j['retention'], array_flip(array('last', 'hourly', 'daily', 'weekly', 'monthly', 'yearly'))))) {
        $errors[] = 'Retention: keep at least one snapshot, or switch clean-up off';
    }

    $cons = is_array($in['consistency'] ?? null) ? $in['consistency'] : array();
    $mode = rb_text($cons['mode'] ?? 'none');
    $j['consistency']['mode'] = in_array($mode, array('none', 'zfs', 'zfs-stop', 'stop'), true) ? $mode : 'none';
    foreach (rb_list($cons['containers'] ?? array(), 200) as $name) {
        if (preg_match('/^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/', $name)) {
            $j['consistency']['containers'][] = $name;
        } else {
            $errors[] = "Containers: \"$name\" is not a container name";
        }
    }
    foreach (rb_list($cons['vms'] ?? array(), 100) as $name) {
        if (!rb_has_control($name)) {
            $j['consistency']['vms'][] = $name;
        }
    }
    foreach (rb_list($cons['skip'] ?? array(), 300) as $name) {
        if (!rb_has_control($name)) {
            $j['consistency']['skip'][] = $name;
        }
    }
    $j['consistency']['timeout'] = max(10, min(3600, rb_count($cons['timeout'] ?? 120) ?: 120));

    $hooks = is_array($in['hooks'] ?? null) ? $in['hooks'] : array();
    foreach (array('before', 'after') as $k) {
        $script = is_string($hooks[$k] ?? null) ? str_replace("\r\n", "\n", $hooks[$k]) : '';
        $j['hooks'][$k] = strlen($script) > 65536 ? substr($script, 0, 65536) : rtrim($script);
    }

    $n = is_array($in['notify'] ?? null) ? $in['notify'] : array();
    $j['notify'] = array(
        'success'     => rb_bool($n['success'] ?? false),
        'warning'     => array_key_exists('warning', $n) ? rb_bool($n['warning']) : true,
        'error'       => array_key_exists('error', $n) ? rb_bool($n['error']) : true,
        'healthcheck' => rb_text($n['healthcheck'] ?? '', 512),
    );
    if ($j['notify']['healthcheck'] !== '' && !preg_match('#^https?://[^\s/]+#', $j['notify']['healthcheck'])) {
        $errors[] = 'Healthcheck URL: an http:// or https:// address';
        $j['notify']['healthcheck'] = '';
    }

    $a = is_array($in['advanced'] ?? null) ? $in['advanced'] : array();
    $host = rb_text($a['host'] ?? '');
    if ($host !== '' && !preg_match('/^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$/', $host)) {
        $errors[] = 'Host name: letters, digits, dots, dashes and underscores only';
        $host = '';
    }
    $j['advanced'] = array(
        'host'              => $host,
        'read_concurrency'  => rb_count($a['read_concurrency'] ?? 0, 64),
        'skip_if_unchanged' => rb_bool($a['skip_if_unchanged'] ?? false),
        'extra_flags'       => rb_clean_flags($a['extra_flags'] ?? array(), 'Extra options', $errors),
    );
    return $j;
}

function rb_check_job_against_repos(array $job, array $cfg)
{
    $errors = array();
    $repo = rb_repo($cfg, $job['repo']);
    if ($repo && $repo['type'] === 'local' && $repo['local']['path'] !== '') {
        foreach ($job['sources'] as $src) {
            if (rb_under($repo['local']['path'], $src)) {
                $errors[] = "Sources: $src contains the repository itself ({$repo['local']['path']}) - " .
                            'exclude it or keep the repository elsewhere';
            }
        }
    }
    return $errors;
}

function rb_config_public(array $cfg)
{
    foreach ($cfg['repos'] as &$r) {
        $r['has_password'] = rb_secret_has($r['id'], 'password');
        $r['has_rest_password'] = rb_secret_has($r['id'], 'rest');
        $r['ssh_public_key'] = (string)@file_get_contents(rb_ssh_key_file($r['id']) . '.pub');
    }
    unset($r);
    return $cfg;
}
