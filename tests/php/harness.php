<?php

error_reporting(E_ALL);
ini_set('display_errors', 'stderr');
set_error_handler(function ($no, $msg, $file, $line) {
    throw new ErrorException($msg, 0, $no, $file, $line);
});

$GLOBALS['t_pass'] = 0;
$GLOBALS['t_fail'] = 0;

define('RB_SRC', dirname(__DIR__, 2) . '/src');

class TFailure extends Exception {}

function t_group($name)
{
    fwrite(STDOUT, "\n\033[1m$name\033[0m\n");
}

function t_case($name, $fn)
{
    try {
        $fn();
        $GLOBALS['t_pass']++;
        fwrite(STDOUT, "  \033[32mPASS\033[0m $name\n");
    } catch (Throwable $e) {
        $GLOBALS['t_fail']++;
        $where = $e instanceof TFailure ? '' : ' (' . basename($e->getFile()) . ':' . $e->getLine() . ')';
        fwrite(STDOUT, "  \033[31mFAIL\033[0m $name\n       " . get_class($e) . ': ' . $e->getMessage() . "$where\n");
    }
}

function t_eq($expected, $actual, $what = '')
{
    if ($expected !== $actual) {
        throw new TFailure(($what !== '' ? "$what: " : '') . 'expected ' . t_show($expected) . ', got ' . t_show($actual));
    }
}

function t_true($cond, $what)
{
    if (!$cond) {
        throw new TFailure($what);
    }
}

function t_throws($fn, $needle, $what = '')
{
    try {
        $fn();
    } catch (Throwable $e) {
        if ($needle !== '' && stripos($e->getMessage(), $needle) === false) {
            throw new TFailure(($what !== '' ? "$what: " : '') . "message \"{$e->getMessage()}\" lacks \"$needle\"");
        }
        return $e;
    }
    throw new TFailure(($what !== '' ? "$what: " : '') . 'nothing was thrown');
}

function t_show($v)
{
    return is_string($v) ? '"' . $v . '"' : json_encode($v, JSON_UNESCAPED_SLASHES);
}

function t_tmpdir()
{
    $dir = sys_get_temp_dir() . '/rbtest.' . bin2hex(random_bytes(4));
    mkdir($dir, 0700, true);
    register_shutdown_function(function () use ($dir) {
        exec('rm -rf ' . escapeshellarg($dir));
    });
    return $dir;
}

function t_done()
{
    $p = $GLOBALS['t_pass'];
    $f = $GLOBALS['t_fail'];
    fwrite(STDOUT, "\n\033[1mResult:\033[0m $p passed, $f failed\n");
    exit($f ? 1 : 0);
}
