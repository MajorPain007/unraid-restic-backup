<?php

require_once dirname(__DIR__, 2) . '/src/include/lib/preview.php';

$errors = array();
$job = rb_clean_job(json_decode($argv[1], true), array('r00000001'), $errors);
$repo = array('id' => 'r00000001', 'type' => 'local', 'local' => array('path' => $argv[2]));
$settings = rb_config_load()['settings'];

$preview = rb_preview_run($job, $repo, $settings, null, null, true);

$tmp = sys_get_temp_dir() . '/rb-compare-' . getmypid();
$args = rb_backup_args($job, $repo, $settings, $tmp);
array_splice($args, 1, 0, array('--dry-run', '-vv'));
$cmd = escapeshellarg(RB_RESTIC) . ' -r ' . escapeshellarg($argv[2]);
foreach ($args as $a) {
    $cmd .= ' ' . escapeshellarg($a);
}
exec($cmd . ' 2>/dev/null', $out, $code);
@unlink("$tmp.exclude");
@unlink("$tmp.iexclude");

$files = array();
$bytes = null;
$count = null;
foreach ($out as $line) {
    $j = json_decode($line, true);
    if (($j['message_type'] ?? '') === 'verbose_status' && in_array($j['action'] ?? '', array('new', 'unchanged', 'modified'), true)
        && substr($j['item'], -1) !== '/') {
        $files[] = $j['item'];
    } elseif (($j['message_type'] ?? '') === 'summary') {
        $bytes = $j['total_bytes_processed'];
        $count = $j['total_files_processed'];
    }
}
sort($files, SORT_STRING);
$list = $preview['files'];
sort($list, SORT_STRING);
echo json_encode(array('preview' => $list, 'restic' => $files, 'preview_bytes' => $preview['total']['bytes'],
                       'restic_bytes' => $bytes, 'restic_count' => $count, 'preview_count' => $preview['total']['files'], 'restic_exit' => $code, 'rules' => $preview['rules']),
                 JSON_UNESCAPED_SLASHES | JSON_PRETTY_PRINT), "\n";
