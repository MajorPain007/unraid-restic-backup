<?php
require_once dirname(__DIR__, 2) . '/src/include/lib/api.php';
$params = json_decode($argv[2] ?? '{}', true) ?: array();
foreach ($params as $k => $v) {
    if (is_array($v)) {
        $params[$k] = json_encode($v);
    }
}
echo json_encode(rb_api_dispatch($argv[1] ?? '', $params), JSON_UNESCAPED_SLASHES | JSON_PRETTY_PRINT), "\n";
