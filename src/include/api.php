<?php
$rbBin = getenv('RB_BIN') ?: dirname(__DIR__) . '/bin/restic-backup';

function rb_proc(array $cmd, $input)
{
    $proc = proc_open($cmd, array(0 => array('pipe', 'r'), 1 => array('pipe', 'w'), 2 => array('file', '/dev/null', 'w')), $pipes);
    if (!is_resource($proc)) {
        return array(null, null);
    }
    fwrite($pipes[0], $input);
    fclose($pipes[0]);
    return array($proc, $pipes[1]);
}

if (($_SERVER['REQUEST_METHOD'] ?? '') !== 'POST') {
    header('Content-Type: application/json');
    echo json_encode(array('ok' => false, 'error' => 'POST only'));
    exit;
}
$action = (string)($_POST['action'] ?? '');
$params = $_POST;
unset($params['action'], $params['csrf_token']);
$input = json_encode($params, JSON_UNESCAPED_SLASHES | JSON_INVALID_UTF8_SUBSTITUTE);

if ($action === 'download') {
    list($proc, $out) = rb_proc(array($rbBin, 'download-info'), $input);
    $info = $proc ? json_decode(stream_get_contents($out), true) : null;
    if ($proc) {
        fclose($out);
        proc_close($proc);
    }
    if (!is_array($info) || empty($info['ok'])) {
        header('Content-Type: text/plain; charset=utf-8');
        http_response_code(404);
        echo 'Download failed: ' . (is_array($info) ? $info['error'] : 'the plugin did not answer.');
        exit;
    }
    @set_time_limit(0);
    while (ob_get_level() > 0) {
        ob_end_clean();
    }
    $name = $info['name'];
    $ascii = preg_replace('/[^A-Za-z0-9._ -]/', '_', $name);
    header('Content-Type: ' . ($info['dir'] ? 'application/zip' : 'application/octet-stream'));
    header("Content-Disposition: attachment; filename=\"$ascii\"; filename*=UTF-8''" . rawurlencode($name));
    header('X-Accel-Buffering: no');
    header('Cache-Control: no-store');
    if (!$info['dir'] && $info['size'] !== null) {
        header('Content-Length: ' . $info['size']);
    }
    list($proc, $out) = rb_proc(array($rbBin, 'download'), $input);
    if (!$proc) {
        exit;
    }
    $sink = fopen('php://output', 'wb');
    while (!feof($out)) {
        $chunk = fread($out, 1048576);
        if ($chunk === false || $chunk === '') {
            if (connection_aborted()) {
                break;
            }
            continue;
        }
        fwrite($sink, $chunk);
        flush();
        if (connection_aborted()) {
            break;
        }
    }
    fclose($out);
    proc_terminate($proc);
    proc_close($proc);
    exit;
}

header('Content-Type: application/json');
header('Cache-Control: no-store');
list($proc, $out) = rb_proc(array($rbBin, 'api', $action), $input);
if (!$proc) {
    echo json_encode(array('ok' => false, 'error' => 'The plugin program could not be started.'));
    exit;
}
$json = stream_get_contents($out);
fclose($out);
proc_close($proc);
echo $json !== '' ? $json : json_encode(array('ok' => false, 'error' => 'The plugin program gave no answer.'));
