<?php

require_once __DIR__ . '/lib/api.php';

if (($_SERVER['REQUEST_METHOD'] ?? '') !== 'POST') {
    header('Content-Type: application/json');
    echo json_encode(array('ok' => false, 'error' => 'POST only'));
    exit;
}

$action = (string)($_POST['action'] ?? '');
$params = $_POST;
unset($params['action'], $params['csrf_token']);

if ($action === 'download') {
    rb_download($params);
    exit;
}

header('Content-Type: application/json');
header('Cache-Control: no-store');
echo json_encode(rb_api_dispatch($action, $params), JSON_UNESCAPED_SLASHES | JSON_INVALID_UTF8_SUBSTITUTE);
exit;

function rb_download(array $p)
{
    rb_use_local_timezone();
    try {
        $cfg = rb_config_load();
        $repo = rb_need_repo($cfg, rb_p($p, 'repo'));
        $snap = rb_need_snapshot_id(rb_p($p, 'snapshot'));
        $path = rb_need_snapshot_path(rb_p($p, 'path'));
        rb_restic_ready($cfg, $repo);
        $parent = rb_api_ls(array('repo' => $repo['id'], 'snapshot' => $snap, 'path' => dirname($path)));
        $node = null;
        foreach ($parent['entries'] as $e) {
            if ($e['path'] === $path) {
                $node = $e;
            }
        }
        if (!$node || !in_array($node['type'], array('file', 'dir'), true)) {
            throw new RbApiError('That file is not in the snapshot.');
        }
    } catch (Throwable $e) {
        header('Content-Type: text/plain; charset=utf-8');
        http_response_code(404);
        echo 'Download failed: ' . $e->getMessage();
        return;
    }
    $isDir = $node['type'] === 'dir';
    $name = basename($path) . ($isDir ? '.zip' : '');
    $ascii = preg_replace('/[^A-Za-z0-9._ -]/', '_', $name);
    @set_time_limit(0);
    while (ob_get_level() > 0) {
        ob_end_clean();
    }
    header('Content-Type: ' . ($isDir ? 'application/zip' : 'application/octet-stream'));
    header("Content-Disposition: attachment; filename=\"$ascii\"; filename*=UTF-8''" . rawurlencode($name));
    header('X-Accel-Buffering: no');
    header('Cache-Control: no-store');
    if (!$isDir && $node['size'] !== null) {
        header('Content-Length: ' . $node['size']);
    }
    $args = $isDir ? array('dump', '--no-lock', '--archive', 'zip', $snap, $path) : array('dump', '--no-lock', $snap, $path);
    $proc = proc_open(rb_restic_cmd($repo, $cfg['settings'], $args, false),
        array(0 => array('file', '/dev/null', 'r'), 1 => array('pipe', 'w'), 2 => array('file', '/dev/null', 'w')),
        $pipes, '/', rb_restic_env($repo, $cfg['settings']));
    if (!is_resource($proc)) {
        return;
    }
    $outFh = fopen('php://output', 'wb');
    while (!feof($pipes[1])) {
        $chunk = fread($pipes[1], 1048576);
        if ($chunk === false || $chunk === '') {
            if (connection_aborted()) {
                break;
            }
            continue;
        }
        fwrite($outFh, $chunk);
        flush();
        if (connection_aborted()) {
            break;
        }
    }
    fclose($pipes[1]);
    proc_terminate($proc);
    proc_close($proc);
}
