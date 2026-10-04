<?php

require_once __DIR__ . '/config.php';

define('RB_DOCKER', getenv('RB_DOCKER') ?: 'docker');
define('RB_VIRSH', getenv('RB_VIRSH') ?: 'virsh');
define('RB_ZFS', getenv('RB_ZFS') ?: 'zfs');
define('RB_MNT', getenv('RB_MNT') ?: '/mnt');

function rb_cmd(array $cmd, $timeout = 60)
{
    $env = array('PATH' => '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin', 'HOME' => '/root', 'LANG' => 'C');
    $proc = @proc_open($cmd, array(0 => array('file', '/dev/null', 'r'), 1 => array('pipe', 'w'), 2 => array('pipe', 'w')),
                       $pipes, '/', $env);
    if (!is_resource($proc)) {
        return array(-1, array(), 'cannot run ' . $cmd[0]);
    }
    stream_set_blocking($pipes[1], false);
    stream_set_blocking($pipes[2], false);
    $out = '';
    $err = '';
    $deadline = microtime(true) + $timeout;
    while (true) {
        $r = array($pipes[1], $pipes[2]);
        $w = $e = null;
        @stream_select($r, $w, $e, 0, 200000);
        $out .= (string)stream_get_contents($pipes[1]);
        $err .= (string)stream_get_contents($pipes[2]);
        $st = proc_get_status($proc);
        if (!$st['running']) {
            $out .= (string)stream_get_contents($pipes[1]);
            $err .= (string)stream_get_contents($pipes[2]);
            $code = $st['exitcode'];
            break;
        }
        if (microtime(true) > $deadline) {
            proc_terminate($proc, 9);
            $code = -2;
            $err .= "\ntimed out after {$timeout}s";
            break;
        }
    }
    fclose($pipes[1]);
    fclose($pipes[2]);
    proc_close($proc);
    $lines = $out === '' ? array() : explode("\n", rtrim($out, "\n"));
    return array($code, $lines, trim($err));
}

function rb_share_key($path)
{
    $parts = explode('/', trim($path, '/'));
    if (count($parts) < 3 || $parts[0] !== trim(RB_MNT, '/')) {
        return null;
    }
    if (in_array($parts[1], array('disks', 'remotes', 'addons', 'rootshare'), true)) {
        return null;
    }
    return array($parts[2], implode('/', array_slice($parts, 3)));
}

function rb_path_within($path, $dir)
{
    $kp = rb_share_key($path);
    $kd = rb_share_key($dir);
    if ($kp && $kd) {
        if ($kp[0] !== $kd[0]) {
            return false;
        }
        $path = '/' . $kp[1];
        $dir = '/' . $kd[1];
    }
    $path = rtrim($path, '/') ?: '/';
    $dir = rtrim($dir, '/') ?: '/';
    return $dir === '/' || rb_under($path, $dir);
}

function rb_user_path_location($path)
{
    $mnt = rtrim(RB_MNT, '/');
    if (!preg_match('#^' . preg_quote($mnt, '#') . '/(user0?)/([^/]+)(/.*)?$#', $path, $m)) {
        return array($path, '');
    }
    $share = $m[2];
    $rest = $m[3] ?? '';
    $found = array();
    $mntDev = @stat($mnt)['dev'];
    foreach (glob("$mnt/*", GLOB_ONLYDIR) ?: array() as $top) {
        $name = basename($top);
        if (in_array($name, array('user', 'user0', 'disks', 'remotes', 'addons', 'rootshare'), true)) {
            continue;
        }
        if (@stat($top)['dev'] === $mntDev) {
            continue;
        }
        $here = "$top/$share$rest";
        if (is_file($here) || (is_dir($here) && rb_dir_has_entries($here))) {
            $found[] = "$top/$share";
        }
    }
    if (count($found) === 1) {
        return array($found[0] . $rest, '');
    }
    if (!$found) {
        return array('', "the share $share was not found on any pool or disk");
    }
    return array('', "the share $share is spread over " . implode(', ', array_map('dirname', $found)) .
                     ' - a ZFS snapshot can only cover one of them');
}

function rb_zfs_mounts()
{
    list($code, $lines) = rb_cmd(array(RB_ZFS, 'list', '-H', '-p', '-t', 'filesystem', '-o', 'name,mountpoint,mounted'), 30);
    if ($code !== 0) {
        return array();
    }
    $out = array();
    foreach ($lines as $line) {
        $f = explode("\t", $line);
        if (count($f) === 3 && $f[2] === 'yes' && $f[1] !== '' && $f[1][0] === '/') {
            $out[] = array($f[0], rtrim($f[1], '/') ?: '/');
        }
    }
    usort($out, function ($a, $b) {
        return strlen($b[1]) - strlen($a[1]);
    });
    return $out;
}

function rb_zfs_plan(array $sources, $mounts = null)
{
    $mounts = $mounts === null ? rb_zfs_mounts() : $mounts;
    if (!$mounts) {
        throw new RuntimeException('No ZFS file systems are mounted on this server.');
    }
    $datasets = array();
    $binds = array();
    $live = array();
    foreach ($sources as $src) {
        list($real, $why) = rb_user_path_location($src);
        $owner = null;
        foreach ($real === '' ? array() : $mounts as $m) {
            if (rb_under($real, $m[1])) {
                $owner = $m;
                break;
            }
        }
        if (!$owner) {
            $live[] = array($src, $real === '' ? $why : rb_zfs_why_not($real));
            continue;
        }
        $datasets[$owner[0]] = true;
        $binds[] = array($owner[0], $owner[1], substr($real, strlen(rtrim($owner[1], '/'))), $src);
        foreach ($mounts as $m) {
            if ($m[1] !== $real && rb_under($m[1], $real) && $m[1] !== $owner[1]) {
                $datasets[$m[0]] = true;
                $binds[] = array($m[0], $m[1], '', rtrim($src, '/') . substr($m[1], strlen(rtrim($real, '/'))));
            }
        }
    }
    if (!$datasets) {
        throw new RuntimeException(count($sources) === 1
            ? "{$live[0][0]} is not on a ZFS dataset ({$live[0][1]}). Choose another consistency setting for this job."
            : 'None of these folders is on a ZFS dataset. Choose another consistency setting for this job.');
    }
    usort($binds, function ($a, $b) {
        return substr_count($a[3], '/') - substr_count($b[3], '/') ?: strcmp($a[3], $b[3]);
    });
    return array('datasets' => array_keys($datasets), 'binds' => $binds, 'live' => $live);
}

function rb_zfs_why_not($real)
{
    $type = rb_fs_type($real);
    if (rb_under($real, '/boot')) {
        $where = 'flash drive';
    } elseif (preg_match('#^' . preg_quote(rtrim(RB_MNT, '/'), '#') . '/([^/]+)/#', $real . '/', $m)) {
        $where = $m[1];
    } else {
        $where = '';
    }
    return implode(', ', array_filter(array($where, $type !== '' ? $type : 'not ZFS')));
}

function rb_zfs_snapshot(array $datasets, $snap)
{
    $byPool = array();
    foreach ($datasets as $d) {
        $byPool[strtok($d, '/')][] = "$d@$snap";
    }
    foreach ($byPool as $names) {
        list($code, , $err) = rb_cmd(array_merge(array(RB_ZFS, 'snapshot'), $names), 120);
        if ($code !== 0) {
            throw new RuntimeException('zfs snapshot failed: ' . ($err !== '' ? $err : "exit code $code"));
        }
    }
}

function rb_zfs_destroy(array $datasets, $snap, $log = null)
{
    $left = array();
    foreach ($datasets as $d) {
        $name = "$d@$snap";
        for ($try = 1; ; $try++) {
            list($code, , $err) = rb_cmd(array(RB_ZFS, 'destroy', $name), 120);
            if ($code === 0 || stripos($err, 'could not find') !== false || stripos($err, 'does not exist') !== false) {
                continue 2;
            }
            if (stripos($err, 'busy') === false || $try === 4) {
                break;
            }
            if ($try === 2) {
                rb_zfs_release($name, $log);
            }
            sleep(1);
        }
        $why = $err !== '' ? str_replace("cannot destroy snapshot $name: ", '', $err) : "exit code $code";
        foreach (rb_zfs_holders($name) as $h) {
            if ($h['container'] !== '') {
                $why .= "; container {$h['container']} has it mounted";
            } else {
                $why .= '; mounted at ' . implode(', ', $h['points']) . ($h['own'] ? ' and in use' : " for {$h['comm']} (pid {$h['pid']})");
            }
        }
        $left[$name] = $why;
    }
    return $left;
}

function rb_zfs_holders($name)
{
    $needle = ' - zfs ' . strtr($name, array(' ' => '\040', "\t" => '\011', "\n" => '\012', '\\' => '\134')) . ' ';
    $own = @readlink('/proc/self/ns/mnt');
    $seen = array();
    $found = array();
    foreach (glob('/proc/[0-9]*', GLOB_ONLYDIR) ?: array() as $dir) {
        $ns = @readlink("$dir/ns/mnt");
        $info = ($ns === false || isset($seen[$ns])) ? false : @file_get_contents("$dir/mountinfo");
        if ($info === false) {
            continue;
        }
        $seen[$ns] = true;
        if (strpos($info, $needle) === false) {
            continue;
        }
        $points = array();
        foreach (explode("\n", $info) as $line) {
            if (strpos($line, $needle) !== false) {
                $points[] = preg_replace_callback('/\\\\([0-7]{3})/', function ($m) {
                    return chr(octdec($m[1]));
                }, explode(' ', $line)[4]);
            }
        }
        $cgroup = (string)@file_get_contents("$dir/cgroup");
        $found[] = array('pid' => (int)basename($dir), 'points' => $points,
                         'container' => preg_match('#/docker[/-]([0-9a-f]{64})#', $cgroup, $m) ? $m[1] : '',
                         'comm' => trim((string)@file_get_contents("$dir/comm")), 'own' => $ns === $own);
    }
    if ($found) {
        $names = array();
        foreach (rb_docker_containers() ?: array() as $c) {
            $names[$c['id']] = $c['name'];
        }
        foreach ($found as $k => $h) {
            if ($h['container'] !== '') {
                $found[$k]['container'] = $names[$h['container']] ?? substr($h['container'], 0, 12);
            }
        }
    }
    return $found;
}

function rb_zfs_release($name, $log = null)
{
    if (strpos($name, '@restic-backup-') === false) {
        return;
    }
    foreach (rb_zfs_holders($name) as $h) {
        if ($h['container'] === '') {
            continue;
        }
        foreach ($h['points'] as $point) {
            list($code, , $err) = rb_cmd(array('nsenter', '-t', (string)$h['pid'], '-p', '--',
                                               'umount', '-N', "/proc/{$h['pid']}/ns/mnt", '-l', $point), 30);
            if ($log) {
                $log($code === 0 ? "Unmounted the copy of $name that container {$h['container']} held"
                                 : "Could not unmount the copy of $name that container {$h['container']} holds: $err");
            }
        }
    }
}

function rb_zfs_wrap(array $cmd, array $plan, $snap, $stageDir)
{
    $script = "set -e\n";
    $dirs = array();
    foreach ($plan['binds'] as $b) {
        if (!isset($dirs[$b[0]])) {
            $dirs[$b[0]] = "$stageDir/" . count($dirs);
            @mkdir($dirs[$b[0]], 0700, true);
            $script .= 'mount -t zfs -o ro ' . escapeshellarg("{$b[0]}@$snap") . ' ' . escapeshellarg($dirs[$b[0]]) . "\n";
        }
    }
    foreach ($plan['binds'] as $b) {
        $from = $dirs[$b[0]] . $b[2];
        $missing = "The snapshot of {$b[0]} has no " . ($b[2] !== '' ? $b[2] : '/') . '.';
        $script .= '[ -e ' . escapeshellarg($from) . ' ] || { echo ' . escapeshellarg($missing) . " >&2; exit 1; }\n";
        $script .= 'mount --bind ' . escapeshellarg($from) . ' ' . escapeshellarg($b[3]) . "\n";
    }
    $script .= 'exec "$@"' . "\n";
    return array_merge(array('unshare', '--mount', '--propagation', 'private', '--', '/bin/sh', '-c', $script, 'sh'), $cmd);
}

function rb_zfs_unstage($stageDir)
{
    foreach (glob("$stageDir/*", GLOB_ONLYDIR) ?: array() as $d) {
        @rmdir($d);
    }
    @rmdir($stageDir);
}

function rb_docker_containers()
{
    list($code, $ids) = rb_cmd(array(RB_DOCKER, 'ps', '-aq', '--no-trunc'), 30);
    if ($code !== 0) {
        return null;
    }
    $ids = array_values(array_filter($ids));
    if (!$ids) {
        return array();
    }
    list($code, $lines) = rb_cmd(array_merge(array(RB_DOCKER, 'inspect'), $ids), 60);
    $data = json_decode(implode("\n", $lines), true);
    if ($code !== 0 || !is_array($data)) {
        return null;
    }
    $out = array();
    foreach ($data as $c) {
        $mounts = array();
        foreach ($c['Mounts'] ?? array() as $m) {
            if (($m['Type'] ?? '') === 'bind' && !empty($m['RW']) && !empty($m['Source'])) {
                $mounts[] = $m['Source'];
            }
        }
        $out[] = array('id' => (string)($c['Id'] ?? ''), 'name' => ltrim((string)($c['Name'] ?? ''), '/'),
                       'running' => !empty($c['State']['Running']), 'mounts' => $mounts);
    }
    return $out;
}

function rb_vms()
{
    list($code, $names) = rb_cmd(array(RB_VIRSH, 'list', '--all', '--name'), 30);
    if ($code !== 0) {
        return null;
    }
    $out = array();
    foreach (array_filter(array_map('trim', $names)) as $name) {
        list(, $lines) = rb_cmd(array(RB_VIRSH, 'domblklist', $name, '--details'), 30);
        $disks = array();
        foreach ($lines as $line) {
            if (preg_match('/^\s*file\s+\S+\s+\S+\s+(\/.+?)\s*$/', $line, $m)) {
                $disks[] = $m[1];
            }
        }
        list(, $state) = rb_cmd(array(RB_VIRSH, 'domstate', $name), 30);
        $out[] = array('name' => $name, 'state' => trim(implode(' ', $state)), 'running' => trim(implode(' ', $state)) !== 'shut off',
                       'disks' => $disks);
    }
    return $out;
}

function rb_consistency_detect(array $sources)
{
    $containers = rb_docker_containers();
    $vms = rb_vms();
    $hitC = array();
    $broad = array();
    foreach ($containers ?: array() as $c) {
        foreach ($c['mounts'] as $m) {
            foreach ($sources as $src) {
                if (rb_path_within($m, $src)) {
                    $hitC[$c['name']] = array('name' => $c['name'], 'running' => $c['running'], 'path' => $m);
                    unset($broad[$c['name']]);
                    break 2;
                }
                if (rb_path_within($src, $m) && !isset($broad[$c['name']])) {
                    $broad[$c['name']] = array('name' => $c['name'], 'running' => $c['running'], 'path' => $m);
                }
            }
        }
    }
    $hitV = array();
    foreach ($vms ?: array() as $v) {
        foreach ($v['disks'] as $d) {
            foreach ($sources as $src) {
                if (rb_path_within($d, $src)) {
                    $hitV[$v['name']] = array('name' => $v['name'], 'running' => $v['running'], 'path' => $d);
                    break 2;
                }
            }
        }
    }
    $zfs = array('ok' => true, 'error' => '', 'datasets' => array(), 'live' => array());
    try {
        $plan = rb_zfs_plan($sources);
        $zfs['datasets'] = $plan['datasets'];
        foreach ($plan['live'] as $l) {
            $zfs['live'][] = array('path' => $l[0], 'why' => $l[1]);
        }
    } catch (Throwable $e) {
        $zfs = array('ok' => false, 'error' => $e->getMessage(), 'datasets' => array(), 'live' => array());
    }
    return array(
        'docker'     => $containers !== null,
        'libvirt'    => $vms !== null,
        'containers' => array_values($hitC),
        'broad'      => array_values($broad),
        'vms'        => array_values($hitV),
        'zfs'        => $zfs,
    );
}

function rb_docker_autostart_order()
{
    $order = array();
    foreach (@file('/var/lib/docker/unraid-autostart', FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES) ?: array() as $line) {
        $f = preg_split('/\s+/', trim($line));
        $order[$f[0]] = isset($f[1]) ? (int)$f[1] : 0;
    }
    return $order;
}

function rb_docker_stop_timeout()
{
    $cfg = @parse_ini_file(getenv('RB_DOCKER_CFG') ?: '/boot/config/docker.cfg');
    $t = is_array($cfg) && isset($cfg['DOCKER_TIMEOUT']) ? (int)$cfg['DOCKER_TIMEOUT'] : 10;
    return max(1, min(600, $t ?: 10));
}

function rb_docker_stop(array $names, $log)
{
    if (!$names) {
        return;
    }
    $timeout = rb_docker_stop_timeout();
    $log('Stopping containers: ' . implode(', ', $names));
    list($code, , $err) = rb_cmd(array_merge(array(RB_DOCKER, 'stop', '-t', (string)$timeout), $names), $timeout + 60);
    if ($code !== 0) {
        throw new RuntimeException('docker stop failed: ' . ($err !== '' ? $err : "exit code $code"));
    }
}

function rb_docker_start(array $names, $log)
{
    $order = rb_docker_autostart_order();
    usort($names, function ($a, $b) use ($order) {
        $pa = array_search($a, array_keys($order), true);
        $pb = array_search($b, array_keys($order), true);
        return ($pa === false ? PHP_INT_MAX : $pa) <=> ($pb === false ? PHP_INT_MAX : $pb);
    });
    $failed = array();
    foreach ($names as $i => $name) {
        $log("Starting container $name");
        list($code, , $err) = rb_cmd(array(RB_DOCKER, 'start', $name), 120);
        if ($code !== 0) {
            $failed[] = "$name ($err)";
        } elseif (!empty($order[$name]) && $i < count($names) - 1) {
            sleep(min(120, $order[$name]));
        }
    }
    return $failed;
}

function rb_vm_stop(array $names, $timeout, $log, array &$stopped, $cancelled = null)
{
    foreach ($names as $name) {
        list(, $state) = rb_cmd(array(RB_VIRSH, 'domstate', $name), 30);
        if (trim(implode(' ', $state)) === 'paused') {
            rb_cmd(array(RB_VIRSH, 'resume', $name), 30);
        }
        $log("Shutting down VM $name");
        rb_cmd(array(RB_VIRSH, 'shutdown', $name), 60);
    }
    $deadline = time() + $timeout;
    $pending = $names;
    while ($pending) {
        foreach ($pending as $k => $name) {
            list(, $state) = rb_cmd(array(RB_VIRSH, 'domstate', $name), 30);
            if (trim(implode(' ', $state)) === 'shut off') {
                $stopped[] = $name;
                unset($pending[$k]);
            }
        }
        if (!$pending) {
            break;
        }
        if (time() > $deadline) {
            throw new RuntimeException('VM ' . implode(', ', $pending) . " did not shut down within {$timeout}s; nothing was backed up.");
        }
        if ($cancelled && $cancelled()) {
            throw new RuntimeException('Stopped.');
        }
        sleep(2);
    }
}

function rb_vm_start(array $names, $log)
{
    $failed = array();
    foreach ($names as $name) {
        $log("Starting VM $name");
        list($code, , $err) = rb_cmd(array(RB_VIRSH, 'start', $name), 120);
        if ($code !== 0 && stripos($err, 'already active') === false) {
            $failed[] = "$name ($err)";
        }
    }
    return $failed;
}

function rb_journal_file($opId)
{
    return RB_RUN_DIR . '/journal/' . $opId . '.json';
}

function rb_journal_write($opId, array $j)
{
    $j['runner_pid'] = getmypid();
    return rb_json_write(rb_journal_file($opId), $j);
}

function rb_journal_clear($opId)
{
    @unlink(rb_journal_file($opId));
}

function rb_journal_recover()
{
    $done = array();
    foreach (glob(RB_RUN_DIR . '/journal/*.json') ?: array() as $file) {
        $j = rb_json_read($file);
        if (!$j || (function_exists('posix_kill') ? posix_kill((int)$j['runner_pid'], 0) : file_exists('/proc/' . (int)$j['runner_pid']))) {
            continue;
        }
        $log = function ($msg) use ($j) {
            if (!empty($j['log'])) {
                rb_log_append($j['log'], $msg, 16 * 1048576);
            }
        };
        $failed = array();
        $j['done'] = $j['done'] ?? array();
        if (!empty($j['snapshots']) && !empty($j['snap'])) {
            $left = rb_zfs_destroy($j['snapshots'], $j['snap'], $log);
            if ($left) {
                foreach ($left as $name => $why) {
                    $failed[] = "$name ($why)";
                }
                $j['snapshots'] = array_values(array_filter($j['snapshots'], function ($d) use ($left, $j) {
                    return isset($left[$d . '@' . $j['snap']]);
                }));
            } else {
                $j['done'][] = 'removed its ZFS snapshots';
                $j['snapshots'] = array();
            }
        }
        if (!empty($j['containers'])) {
            $bad = rb_docker_start($j['containers'], $log);
            if ($bad) {
                $failed = array_merge($failed, $bad);
            } else {
                $j['done'][] = 'started ' . implode(', ', $j['containers']);
                $j['containers'] = array();
            }
        }
        if (!empty($j['vms'])) {
            $bad = rb_vm_start($j['vms'], $log);
            if ($bad) {
                $failed = array_merge($failed, $bad);
            } else {
                $j['done'][] = 'started VM ' . implode(', ', $j['vms']);
                $j['vms'] = array();
            }
        }
        $j['attempts'] = ($j['attempts'] ?? 0) + 1;
        if ($failed && $j['attempts'] < 30) {
            rb_json_write($file, $j);
            continue;
        }
        if (!empty($j['ended'])) {
            if ($failed) {
                $log('Gave up removing the ZFS snapshot after 30 minutes: ' . implode('; ', $failed));
                $done[] = array('ZFS snapshot left behind', 'A backup could not remove its ZFS snapshot, not in 30 minutes '
                    . 'of trying: ' . implode('; ', $failed) . '. Remove it with zfs destroy once nothing has it mounted.');
            } else {
                $log('Removed the ZFS snapshot that was still busy at the end.');
            }
        } else {
            $what = $j['done'] ? implode(', ', $j['done']) : 'nothing needed putting back';
            $log('The runner was gone; the scheduler ' . $what . ($failed ? ', but not: ' . implode('; ', $failed) : '') . '.');
            $done[] = array('Backup interrupted', 'A backup ended unexpectedly; ' . $what .
                      ($failed ? '. Could not, after 30 minutes of trying: ' . implode('; ', $failed) : '.'));
        }
        @unlink($file);
    }
    return $done;
}
