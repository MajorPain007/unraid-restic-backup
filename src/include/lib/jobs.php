<?php

require_once __DIR__ . '/config.php';
require_once __DIR__ . '/consistency.php';

function rb_job_host(array $job, array $settings)
{
    if ($job['advanced']['host'] !== '') {
        return $job['advanced']['host'];
    }
    return $settings['host'] !== '' ? $settings['host'] : rb_server_name();
}

function rb_pattern_file(array $patterns)
{
    return implode("\n", array_map(function ($p) {
        return str_replace('$', '$$', $p);
    }, $patterns)) . "\n";
}

function rb_own_excludes(array $job, array $repo, array $settings)
{
    $own = array($settings['data_dir'] . '/cache');
    if (($repo['type'] ?? '') === 'local') {
        $own[] = $repo['local']['path'];
    }
    $out = array();
    foreach ($own as $path) {
        foreach ($job['sources'] as $src) {
            $as = rb_path_as($path, $src);
            if ($as !== null && !in_array($as, $out, true)) {
                $out[] = $as;
            }
        }
    }
    return $out;
}

function rb_path_as($path, $dir)
{
    if (rb_under($path, $dir)) {
        return $path;
    }
    $kp = rb_share_key($path);
    $kd = rb_share_key($dir);
    if (!$kp || !$kd || !rb_path_within($path, $dir)) {
        return null;
    }
    $top = explode('/', trim($dir, '/'))[1];
    return rtrim(RB_MNT, '/') . "/$top/$kp[0]" . ($kp[1] === '' ? '' : "/$kp[1]");
}

function rb_backup_args(array $job, array $repo, array $settings, $tmp)
{
    $args = array('backup', '--json', '--host', rb_job_host($job, $settings), '--tag', 'job:' . $job['id'],
                  '--group-by', 'host,tags');
    foreach ($job['tags'] as $tag) {
        array_push($args, '--tag', $tag);
    }
    $excludes = $job['excludes'];
    $excludes[] = '.zfs/snapshot';
    foreach (rb_own_excludes($job, $repo, $settings) as $own) {
        $excludes[] = $own;
    }
    if ($excludes) {
        file_put_contents("$tmp.exclude", rb_pattern_file($excludes));
        array_push($args, '--exclude-file', "$tmp.exclude");
    }
    if ($job['iexcludes']) {
        file_put_contents("$tmp.iexclude", rb_pattern_file($job['iexcludes']));
        array_push($args, '--iexclude-file', "$tmp.iexclude");
    }
    if ($job['exclude_caches']) {
        $args[] = '--exclude-caches';
    }
    if ($job['exclude_larger_than'] !== '') {
        array_push($args, '--exclude-larger-than', $job['exclude_larger_than']);
    }
    if ($job['one_file_system']) {
        $args[] = '--one-file-system';
    }
    if ($job['advanced']['read_concurrency'] > 0) {
        array_push($args, '--read-concurrency', (string)$job['advanced']['read_concurrency']);
    }
    if ($job['advanced']['skip_if_unchanged']) {
        $args[] = '--skip-if-unchanged';
    }
    foreach ($job['advanced']['extra_flags'] as $flag) {
        $args[] = $flag;
    }
    array_push($args, '--retry-lock', '2m', '--');
    foreach ($job['sources'] as $src) {
        $args[] = $src;
    }
    return $args;
}

function rb_forget_args(array $job, $dryRun = false)
{
    $args = array('forget', '--json', '--tag', 'job:' . $job['id'], '--group-by', '');
    $ret = $job['retention'];
    foreach (array('last', 'hourly', 'daily', 'weekly', 'monthly', 'yearly') as $k) {
        if ($ret[$k] > 0) {
            array_push($args, "--keep-$k", (string)$ret[$k]);
        }
    }
    if ($ret['within'] !== '') {
        array_push($args, '--keep-within', $ret['within']);
    }
    if ($dryRun) {
        $args[] = '--dry-run';
    } else {
        array_push($args, '--retry-lock', '2m');
    }
    return $args;
}
