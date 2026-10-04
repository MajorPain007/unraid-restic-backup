<?php

$src = dirname(__DIR__, 2) . '/src';
$entries = array_merge(glob("$src/scripts/*.php"), array("$src/include/api.php", "$src/include/page.php"));

function loads($file, array &$seen = array())
{
    $file = realpath($file);
    if ($file === false || isset($seen[$file])) {
        return $seen;
    }
    $seen[$file] = true;
    $code = file_get_contents($file);
    preg_match_all('/require(?:_once)?\s*\(?\s*(dirname\(__DIR__\)|__DIR__)\s*\.\s*\'([^\']+)\'/', $code, $m, PREG_SET_ORDER);
    foreach ($m as $req) {
        $base = $req[1] === '__DIR__' ? dirname($file) : dirname(dirname($file));
        loads($base . $req[2], $seen);
    }
    return $seen;
}

function functions($file)
{
    $defined = array();
    $called = array();
    $tokens = token_get_all(file_get_contents($file));
    $n = count($tokens);
    for ($i = 0; $i < $n; $i++) {
        $t = $tokens[$i];
        if (!is_array($t) || $t[0] !== T_STRING || strpos($t[1], 'rb_') !== 0) {
            continue;
        }
        $prev = $i;
        do {
            $prev--;
        } while ($prev >= 0 && is_array($tokens[$prev]) && $tokens[$prev][0] === T_WHITESPACE);
        $next = $i;
        do {
            $next++;
        } while ($next < $n && is_array($tokens[$next]) && $tokens[$next][0] === T_WHITESPACE);
        $isFunctionKeyword = $prev >= 0 && is_array($tokens[$prev]) && $tokens[$prev][0] === T_FUNCTION;
        $isMethod = $prev >= 0 && is_array($tokens[$prev]) && in_array($tokens[$prev][0], array(T_OBJECT_OPERATOR, T_DOUBLE_COLON), true);
        if ($isFunctionKeyword) {
            $defined[$t[1]] = true;
        } elseif (!$isMethod && $next < $n && $tokens[$next] === '(') {
            $called[$t[1]][] = $t[2];
        }
    }
    return array($defined, $called);
}

$problems = array();
$checked = 0;
foreach ($entries as $entry) {
    $files = array_keys(loads($entry));
    $defined = array();
    $calls = array();
    foreach ($files as $f) {
        list($d, $c) = functions($f);
        $defined += $d;
        foreach ($c as $name => $lines) {
            foreach ($lines as $line) {
                $calls[] = array($name, $f, $line);
            }
        }
    }
    foreach ($calls as $c) {
        $checked++;
        if (!isset($defined[$c[0]])) {
            $problems[] = basename($entry) . ': ' . $c[0] . '() called in ' . str_replace("$src/", '', $c[1]) .
                          ':' . $c[2] . ' is not defined in anything it loads';
        }
    }
}
$problems = array_unique($problems);
foreach ($problems as $p) {
    fwrite(STDOUT, "  \033[31mFAIL\033[0m $p\n");
}
fwrite(STDOUT, ($problems ? '' : "  \033[32mPASS\033[0m ") . count($entries) . " entry points, $checked calls" .
       ($problems ? '' : ': every rb_ function they call is loaded') . "\n");
exit($problems ? 1 : 0);
