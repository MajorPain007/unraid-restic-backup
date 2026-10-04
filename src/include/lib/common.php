<?php

define('RB_NAME', 'restic.backup');
define('RB_CONFIG_DIR', getenv('RB_CONFIG_DIR') ?: '/boot/config/plugins/' . RB_NAME);
define('RB_RUN_DIR', getenv('RB_RUN_DIR') ?: '/tmp/' . RB_NAME);
define('RB_PLUGIN_DIR', getenv('RB_PLUGIN_DIR') ?: '/usr/local/emhttp/plugins/' . RB_NAME);
define('RB_RESTIC', getenv('RB_RESTIC') ?: '/usr/local/lib/' . RB_NAME . '/restic');
define('RB_VAR_INI', getenv('RB_VAR_INI') ?: '/var/local/emhttp/var.ini');
define('RB_PHP', getenv('RB_PHP') ?: '/usr/bin/php');

function rb_json_read($path, $default = null)
{
    $text = @file_get_contents($path);
    if ($text === false) {
        return $default;
    }
    $data = json_decode($text, true);
    return is_array($data) ? $data : $default;
}

function rb_json_write($path, $data, $mode = 0644)
{
    $text = json_encode($data, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE) . "\n";
    return rb_file_write($path, $text, $mode);
}

function rb_file_write($path, $text, $mode = 0644)
{
    if (@file_get_contents($path) === $text) {
        return true;
    }
    $dir = dirname($path);
    if (!is_dir($dir) && !@mkdir($dir, 0755, true)) {
        return false;
    }
    $tmp = $path . '.tmp' . getmypid();
    if (@file_put_contents($tmp, $text) !== strlen($text)) {
        @unlink($tmp);
        return false;
    }
    @chmod($tmp, $mode);
    if (!@rename($tmp, $path)) {
        @unlink($tmp);
        return false;
    }
    return true;
}

function rb_new_id($prefix)
{
    return $prefix . bin2hex(random_bytes(4));
}

function rb_var_ini()
{
    $ini = @parse_ini_file(RB_VAR_INI);
    return is_array($ini) ? $ini : array();
}

function rb_use_local_timezone()
{
    $tz = '';
    $ini = rb_var_ini();
    if (!empty($ini['timeZone'])) {
        $tz = $ini['timeZone'];
    } elseif (is_link('/etc/localtime')) {
        $target = readlink('/etc/localtime');
        $pos = strpos($target, 'zoneinfo/');
        if ($pos !== false) {
            $tz = substr($target, $pos + 9);
        }
    }
    if ($tz !== '' && in_array($tz, timezone_identifiers_list(), true)) {
        date_default_timezone_set($tz);
    } elseif (!ini_get('date.timezone')) {
        date_default_timezone_set('UTC');
    }
}

function rb_array_started()
{
    $ini = rb_var_ini();
    if (!$ini) {
        return getenv('RB_ASSUME_ARRAY') !== '0';
    }
    return ($ini['mdState'] ?? '') === 'STARTED' && ($ini['fsState'] ?? '') === 'Started'
        && ($ini['startMode'] ?? '') !== 'Maintenance';
}

function rb_server_name()
{
    $ini = rb_var_ini();
    $name = $ini['NAME'] ?? '';
    return $name !== '' ? $name : (gethostname() ?: 'unraid');
}

function rb_human_bytes($n)
{
    $n = (float)$n;
    foreach (array('B', 'KB', 'MB', 'GB', 'TB', 'PB') as $i => $unit) {
        if ($n < 1024 || $unit === 'PB') {
            return ($i === 0 ? (string)(int)$n : number_format($n, $n < 10 ? 1 : 0)) . ' ' . $unit;
        }
        $n /= 1024;
    }
    return '';
}

function rb_human_duration($seconds)
{
    $s = (int)round($seconds);
    if ($s < 60) {
        return $s . ' s';
    }
    if ($s < 3600) {
        return intdiv($s, 60) . ' min';
    }
    $h = intdiv($s, 3600);
    $m = intdiv($s % 3600, 60);
    return $h . ' h' . ($m ? ' ' . $m . ' min' : '');
}

function rb_log_append($path, $line, $max = 1048576)
{
    $dir = dirname($path);
    if (!is_dir($dir)) {
        @mkdir($dir, 0755, true);
    }
    @file_put_contents($path, date('Y-m-d H:i:s') . ' ' . rtrim($line, "\n") . "\n", FILE_APPEND | LOCK_EX);
    clearstatcache(true, $path);
    if (@filesize($path) > $max) {
        rb_log_trim($path, intdiv($max, 2));
    }
}

function rb_log_trim($path, $keep)
{
    $fh = @fopen($path, 'r+');
    if (!$fh) {
        return;
    }
    if (flock($fh, LOCK_EX)) {
        $size = fstat($fh)['size'];
        if ($size > $keep) {
            fseek($fh, $size - $keep);
            $tail = stream_get_contents($fh);
            $nl = strpos($tail, "\n");
            $tail = $nl === false ? $tail : substr($tail, $nl + 1);
            ftruncate($fh, 0);
            rewind($fh);
            fwrite($fh, $tail);
        }
        flock($fh, LOCK_UN);
    }
    fclose($fh);
}

function rb_tail($path, $lines = 200, $maxBytes = 262144)
{
    $fh = @fopen($path, 'r');
    if (!$fh) {
        return '';
    }
    $size = fstat($fh)['size'];
    $start = max(0, $size - $maxBytes);
    fseek($fh, $start);
    $data = stream_get_contents($fh);
    fclose($fh);
    if ($start > 0) {
        $nl = strpos($data, "\n");
        $data = $nl === false ? '' : substr($data, $nl + 1);
    }
    $all = explode("\n", rtrim($data, "\n"));
    return implode("\n", array_slice($all, -$lines));
}

function rb_mkdir_like_parent($dir)
{
    if (is_dir($dir)) {
        return true;
    }
    $parent = dirname($dir);
    if (!rb_mkdir_like_parent($parent) || !@mkdir($dir)) {
        return false;
    }
    $st = @stat($parent);
    if ($st) {
        @chmod($dir, $st['mode'] & 07777);
        @chown($dir, $st['uid']);
        @chgrp($dir, $st['gid']);
    }
    return true;
}

function rb_fs_type($path)
{
    $path = realpath($path);
    if ($path === false) {
        return '';
    }
    $best = '';
    $type = '';
    foreach (@file('/proc/self/mountinfo', FILE_IGNORE_NEW_LINES) ?: array() as $line) {
        $parts = explode(' - ', $line, 2);
        $f = explode(' ', $parts[0]);
        if (count($parts) !== 2 || !isset($f[4])) {
            continue;
        }
        $mp = preg_replace_callback('/\\\\([0-7]{3})/', function ($m) {
            return chr(octdec($m[1]));
        }, $f[4]);
        if (($mp === '/' || $path === $mp || strpos($path, rtrim($mp, '/') . '/') === 0) && strlen($mp) >= strlen($best)) {
            $best = $mp;
            $type = strtok($parts[1], ' ');
        }
    }
    return $type;
}

function rb_dir_has_entries($dir)
{
    $dh = @opendir($dir);
    if (!$dh) {
        return false;
    }
    while (($e = readdir($dh)) !== false) {
        if ($e !== '.' && $e !== '..') {
            closedir($dh);
            return true;
        }
    }
    closedir($dh);
    return false;
}
