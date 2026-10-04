<?php

require_once __DIR__ . '/jobs.php';

define('RB_PREVIEW_DIR', RB_RUN_DIR . '/preview');
define('RB_PREVIEW_TREE_DEPTH', 3);
define('RB_PREVIEW_TREE_KEEP', 12);

function rb_path_clean($p)
{
    if ($p === '') {
        return '.';
    }
    $rooted = $p[0] === '/';
    $out = array();
    foreach (explode('/', $p) as $seg) {
        if ($seg === '' || $seg === '.') {
            continue;
        }
        if ($seg === '..') {
            if ($out && end($out) !== '..') {
                array_pop($out);
            } elseif (!$rooted) {
                $out[] = '..';
            }
            continue;
        }
        $out[] = $seg;
    }
    $s = implode('/', $out);
    return $rooted ? '/' . $s : ($s === '' ? '.' : $s);
}

function rb_filter_split($p)
{
    $parts = explode('/', $p);
    if ($parts[0] === '') {
        $parts[0] = '/';
    }
    return $parts;
}

function rb_filter_parse($pattern, $lower = false)
{
    $neg = $pattern !== '' && $pattern[0] === '!';
    $body = $neg ? substr($pattern, 1) : $pattern;
    if ($lower) {
        $body = strtolower($body);
    }
    $parts = array();
    foreach (rb_filter_split(rb_path_clean($body)) as $part) {
        $parts[] = array('p' => $part === '**' ? '' : $part, 'simple' => strpbrk($part, '\\[]*?') === false);
    }
    return array('neg' => $neg, 'parts' => $parts, 'lower' => $lower);
}

function rb_filter_match(array $parts, array $strs)
{
    foreach ($parts as $pos => $part) {
        if ($part['p'] === '') {
            $prefix = array_slice($parts, 0, $pos);
            $suffix = array_slice($parts, $pos + 1);
            for ($i = 0; $i <= count($strs) - count($parts) + 1; $i++) {
                $stars = $i > 0 ? array_fill(0, $i, array('p' => '*', 'simple' => false)) : array();
                if (rb_filter_match(array_merge($prefix, $stars, $suffix), $strs)) {
                    return true;
                }
            }
            return false;
        }
    }
    if (!$parts) {
        return !$strs;
    }
    if (count($parts) > count($strs)) {
        return false;
    }
    $min = 0;
    $max = count($strs) - count($parts);
    if ($parts[0]['p'] === '/') {
        $max = 0;
    } elseif ($strs[0] === '/') {
        $min = 1;
    }
    for ($offset = $max; $offset >= $min; $offset--) {
        for ($i = count($parts) - 1; $i >= 0; $i--) {
            $s = $strs[$offset + $i];
            if ($parts[$i]['simple'] ? $parts[$i]['p'] !== $s : !fnmatch($parts[$i]['p'], $s)) {
                continue 2;
            }
        }
        return true;
    }
    return false;
}

function rb_filter_list(array $patterns, $path)
{
    $strs = rb_filter_split($path);
    $lowerStrs = null;
    $by = null;
    foreach ($patterns as $i => $p) {
        if ($p['lower']) {
            $lowerStrs = $lowerStrs ?? rb_filter_split(strtolower($path));
        }
        if (!rb_filter_match($p['parts'], $p['lower'] ? $lowerStrs : $strs)) {
            continue;
        }
        if ($p['neg']) {
            $by = null;
        } elseif ($by === null) {
            $by = $i;
        }
    }
    return $by;
}

function rb_parse_bytes($s)
{
    if (!preg_match('/^(\d+)([bkmgt]?)$/i', trim($s), $m)) {
        return null;
    }
    $unit = array('' => 1, 'b' => 1, 'k' => 1024, 'm' => 1048576, 'g' => 1073741824, 't' => 1099511627776);
    return (int)$m[1] * $unit[strtolower($m[2])];
}

function rb_preview_rules(array $job, array $repo, array $settings)
{
    $rules = array();
    $patterns = array();
    foreach ($job['excludes'] as $p) {
        $rules[] = array('kind' => 'pattern', 'label' => $p, 'measure' => true);
        $patterns[] = rb_filter_parse($p) + array('rule' => count($rules) - 1);
    }
    $rules[] = array('kind' => 'zfs', 'label' => '.zfs/snapshot', 'measure' => false);
    $patterns[] = rb_filter_parse('.zfs/snapshot') + array('rule' => count($rules) - 1);
    foreach (rb_own_excludes($job, $repo, $settings) as $dir) {
        $rules[] = array('kind' => 'plugin', 'label' => $dir, 'measure' => true);
        $patterns[] = rb_filter_parse($dir) + array('rule' => count($rules) - 1);
    }
    foreach ($job['iexcludes'] as $p) {
        $rules[] = array('kind' => 'ipattern', 'label' => $p, 'measure' => true);
        $patterns[] = rb_filter_parse($p, true) + array('rule' => count($rules) - 1);
    }
    $ctx = array('rules' => $rules, 'patterns' => $patterns, 'caches' => null, 'size' => null, 'otherfs' => null,
                 'max_size' => null);
    if ($job['exclude_caches']) {
        $ctx['rules'][] = array('kind' => 'caches', 'label' => 'CACHEDIR.TAG', 'measure' => true);
        $ctx['caches'] = count($ctx['rules']) - 1;
    }
    if ($job['one_file_system']) {
        $ctx['rules'][] = array('kind' => 'otherfs', 'label' => 'other file systems', 'measure' => false);
        $ctx['otherfs'] = count($ctx['rules']) - 1;
    }
    if ($job['exclude_larger_than'] !== '' && ($max = rb_parse_bytes($job['exclude_larger_than'])) !== null) {
        $ctx['rules'][] = array('kind' => 'size', 'label' => $job['exclude_larger_than'], 'measure' => true);
        $ctx['size'] = count($ctx['rules']) - 1;
        $ctx['max_size'] = $max;
    }
    foreach ($ctx['rules'] as $i => $r) {
        $ctx['rules'][$i] += array('files' => 0, 'dirs' => 0, 'bytes' => 0, 'examples' => array());
    }
    return $ctx;
}

function rb_preview_run(array $job, array $repo, array $settings, $progress = null, $cancelled = null, $listFiles = false)
{
    $ctx = rb_preview_rules($job, $repo, $settings);
    if ($listFiles) {
        $ctx['list'] = array();
    }
    $ctx += array('files' => 0, 'dirs' => 0, 'bytes' => 0, 'largest' => array(), 'errors' => array(),
                  'seen' => 0, 'current' => '', 'last_report' => 0.0, 'progress' => $progress,
                  'cancelled' => $cancelled, 'stop' => false, 'tagged' => array(), 'source_dev' => null,
                  'started' => microtime(true));
    $sources = array();
    foreach ($job['sources'] as $src) {
        $src = rb_path_clean($src);
        $entry = array('path' => $src, 'files' => 0, 'dirs' => 0, 'bytes' => 0, 'tree' => null, 'missing' => false);
        $st = @lstat($src);
        if ($st === false) {
            $entry['missing'] = true;
            $sources[] = $entry;
            continue;
        }
        $ctx['source_dev'] = $st['dev'];
        $rule = rb_preview_by_name($src, $ctx);
        if ($rule !== null) {
            rb_preview_excluded($rule, $src, $st, $ctx);
        } elseif (($st['mode'] & 0170000) === 0040000) {
            $before = array($ctx['files'], $ctx['dirs'], $ctx['bytes']);
            $entry['tree'] = rb_preview_dir($src, basename($src) ?: $src, 0, $ctx);
            $entry['files'] = $ctx['files'] - $before[0];
            $entry['dirs'] = $ctx['dirs'] - $before[1];
            $entry['bytes'] = $ctx['bytes'] - $before[2];
        } else {
            $entry['bytes'] = rb_preview_file($src, $st, $ctx);
            $entry['files'] = ($st['mode'] & 0170000) === 0100000 ? 1 : 0;
        }
        $sources[] = $entry;
        if ($ctx['stop']) {
            break;
        }
    }
    usort($ctx['largest'], function ($a, $b) {
        return $b['bytes'] <=> $a['bytes'];
    });
    $rules = array();
    foreach ($ctx['rules'] as $r) {
        if ($r['kind'] === 'pattern' || $r['kind'] === 'ipattern' || $r['files'] || $r['dirs']) {
            $rules[] = $r;
        }
    }
    return array(
        'cancelled' => $ctx['stop'],
        'seconds'   => round(microtime(true) - $ctx['started'], 1),
        'total'     => array('files' => $ctx['files'], 'dirs' => $ctx['dirs'], 'bytes' => $ctx['bytes']),
        'sources'   => $sources,
        'rules'     => $rules,
        'largest'   => $ctx['largest'],
        'errors'    => array_slice($ctx['errors'], 0, 20),
        'error_count' => count($ctx['errors']),
    ) + (isset($ctx['list']) ? array('files' => $ctx['list']) : array());
}

function rb_preview_by_name($item, array &$ctx)
{
    $i = rb_filter_list($ctx['patterns'], $item);
    if ($i !== null) {
        return $ctx['patterns'][$i]['rule'];
    }
    if ($ctx['caches'] !== null && basename($item) !== 'CACHEDIR.TAG') {
        $dir = dirname($item);
        if (!isset($ctx['tagged'][$dir])) {
            $head = @file_get_contents("$dir/CACHEDIR.TAG", false, null, 0, 43);
            $ctx['tagged'][$dir] = $head === 'Signature: 8a477f597d28d172789f06886806bc55';
        }
        if ($ctx['tagged'][$dir]) {
            return $ctx['caches'];
        }
    }
    return null;
}

function rb_preview_by_stat($item, array $st, array &$ctx)
{
    $isDir = ($st['mode'] & 0170000) === 0040000;
    if ($ctx['otherfs'] !== null && $st['dev'] !== $ctx['source_dev']) {
        $parent = @lstat(dirname($item));
        return $isDir && $parent && $parent['dev'] === $ctx['source_dev'] ? 'mount' : $ctx['otherfs'];
    }
    if ($ctx['size'] !== null && !$isDir && $st['size'] > $ctx['max_size']) {
        return $ctx['size'];
    }
    return null;
}

function rb_preview_dir($path, $name, $depth, array &$ctx)
{
    $ctx['dirs']++;
    $ctx['current'] = $path;
    $node = array('name' => $name, 'bytes' => 0, 'files' => 0, 'children' => array(), 'more' => null);
    $dh = @opendir($path);
    if (!$dh) {
        $ctx['errors'][] = "$path: cannot be read";
        return $node;
    }
    $names = array();
    while (($e = readdir($dh)) !== false) {
        if ($e !== '.' && $e !== '..') {
            $names[] = $e;
        }
    }
    closedir($dh);
    sort($names, SORT_STRING);
    foreach ($names as $entry) {
        if ($ctx['stop']) {
            break;
        }
        $item = ($path === '/' ? '' : $path) . '/' . $entry;
        $st = @lstat($item);
        if ($st === false) {
            continue;
        }
        $rule = rb_preview_by_name($item, $ctx);
        $rule = $rule ?? rb_preview_by_stat($item, $st, $ctx);
        if ($rule === 'mount') {
            $ctx['dirs']++;
            continue;
        }
        if ($rule !== null) {
            rb_preview_excluded($rule, $item, $st, $ctx);
            continue;
        }
        if (($st['mode'] & 0170000) === 0040000) {
            $child = rb_preview_dir($item, $entry, $depth + 1, $ctx);
            $node['bytes'] += $child['bytes'];
            $node['files'] += $child['files'];
            if ($depth < RB_PREVIEW_TREE_DEPTH) {
                $node['children'][] = $child;
            }
        } elseif (($st['mode'] & 0170000) === 0100000) {
            $node['bytes'] += rb_preview_file($item, $st, $ctx);
            $node['files']++;
        }
    }
    usort($node['children'], function ($a, $b) {
        return $b['bytes'] <=> $a['bytes'];
    });
    if (count($node['children']) > RB_PREVIEW_TREE_KEEP) {
        $rest = array_splice($node['children'], RB_PREVIEW_TREE_KEEP);
        $node['more'] = array('count' => count($rest), 'bytes' => array_sum(array_column($rest, 'bytes')));
    }
    if ($depth >= RB_PREVIEW_TREE_DEPTH) {
        unset($node['children'], $node['more']);
    }
    return $node;
}

function rb_preview_file($item, array $st, array &$ctx)
{
    if (($st['mode'] & 0170000) !== 0100000) {
        return 0;
    }
    $size = $st['size'];
    $ctx['files']++;
    $ctx['bytes'] += $size;
    if (isset($ctx['list'])) {
        $ctx['list'][] = $item;
    }
    if ($size > 0 && (count($ctx['largest']) < 15 || $size > $ctx['largest'][14]['bytes'])) {
        $ctx['largest'][] = array('path' => $item, 'bytes' => $size);
        usort($ctx['largest'], function ($a, $b) {
            return $b['bytes'] <=> $a['bytes'];
        });
        $ctx['largest'] = array_slice($ctx['largest'], 0, 15);
    }
    rb_preview_tick($ctx);
    return $size;
}

function rb_preview_excluded($rule, $item, array $st, array &$ctx)
{
    $r = &$ctx['rules'][$rule];
    if (count($r['examples']) < 5) {
        $r['examples'][] = $item;
    }
    if (($st['mode'] & 0170000) === 0100000) {
        $r['files']++;
        $r['bytes'] += $st['size'];
    } elseif (($st['mode'] & 0170000) === 0040000) {
        $r['dirs']++;
        if ($r['measure']) {
            rb_preview_measure($item, $r, $ctx);
        }
    }
    rb_preview_tick($ctx);
}

function rb_preview_measure($dir, array &$r, array &$ctx)
{
    $dh = @opendir($dir);
    if (!$dh) {
        return;
    }
    while (($e = readdir($dh)) !== false && !$ctx['stop']) {
        if ($e === '.' || $e === '..' || $e === '.zfs') {
            continue;
        }
        $st = @lstat("$dir/$e");
        if ($st === false) {
            continue;
        }
        if (($st['mode'] & 0170000) === 0040000) {
            $r['dirs']++;
            rb_preview_measure("$dir/$e", $r, $ctx);
        } elseif (($st['mode'] & 0170000) === 0100000) {
            $r['files']++;
            $r['bytes'] += $st['size'];
            rb_preview_tick($ctx);
        }
    }
    closedir($dh);
}

function rb_preview_tick(array &$ctx)
{
    if (++$ctx['seen'] % 500 !== 0) {
        return;
    }
    $now = microtime(true);
    if ($now - $ctx['last_report'] < 1.0) {
        return;
    }
    $ctx['last_report'] = $now;
    if ($ctx['cancelled'] && ($ctx['cancelled'])()) {
        $ctx['stop'] = true;
    }
    if ($ctx['progress']) {
        ($ctx['progress'])(array('files' => $ctx['files'], 'dirs' => $ctx['dirs'], 'bytes' => $ctx['bytes'],
                                 'current' => $ctx['current']));
    }
}

function rb_preview_source_of(array $job, $path)
{
    $found = null;
    foreach ($job['sources'] as $src) {
        $src = rb_path_clean($src);
        if (rb_under($path, $src) && ($found === null || strlen($src) > strlen($found))) {
            $found = $src;
        }
    }
    return $found;
}

function rb_preview_folder(array $job, array $repo, array $settings, $src, $path, $limit = 500)
{
    $ctx = rb_preview_rules($job, $repo, $settings) + array('tagged' => array(), 'source_dev' => null);
    $out = function ($rule, $via = null) use (&$ctx) {
        $r = $ctx['rules'][$rule];
        return array('kind' => $r['kind'], 'label' => $r['label'], 'via' => $via);
    };
    $entry = function ($item, $name, $st) {
        $types = array(0040000 => 'dir', 0100000 => 'file', 0120000 => 'symlink');
        $type = $types[$st['mode'] & 0170000] ?? 'other';
        return array('name' => $name, 'path' => $item, 'type' => $type,
                     'size' => $type === 'file' ? $st['size'] : null,
                     'open' => $type === 'dir' && $name !== '.zfs', 'out' => null, 'mount' => false);
    };

    if ($path === '') {
        $entries = array();
        foreach ($job['sources'] as $s) {
            $s = rb_path_clean($s);
            $st = @lstat($s);
            if ($st === false) {
                $entries[] = array('name' => $s, 'path' => $s, 'type' => 'missing', 'size' => null, 'open' => false,
                                   'out' => null, 'mount' => false);
                continue;
            }
            $e = $entry($s, $s, $st);
            $rule = rb_preview_by_name($s, $ctx);
            $e['out'] = $rule !== null ? $out($rule) : null;
            $entries[] = $e;
        }
        return array('path' => '', 'entries' => $entries, 'more' => 0, 'more_out' => 0);
    }

    $st = @lstat($src);
    if ($st === false) {
        throw new RuntimeException("$src does not exist.");
    }
    $ctx['source_dev'] = $st['dev'];
    $rule = rb_preview_by_name($src, $ctx);
    $via = $rule !== null ? $src : null;
    $cur = $src;
    $rest = substr($path, strlen($src));
    foreach ($rest === '' ? array() : explode('/', ltrim($rest, '/')) as $name) {
        $cur = ($cur === '/' ? '' : $cur) . '/' . $name;
        $st = @lstat($cur);
        if ($st === false || ($st['mode'] & 0170000) !== 0040000) {
            throw new RuntimeException("$cur is not a folder (any more).");
        }
        if ($via !== null) {
            continue;
        }
        $rule = rb_preview_by_name($cur, $ctx) ?? rb_preview_by_stat($cur, $st, $ctx);
        if ($rule === 'mount') {
            $rule = $ctx['otherfs'];
        }
        if ($rule !== null) {
            $via = $cur;
        }
    }

    $names = @scandir($path);
    if ($names === false) {
        throw new RuntimeException("$path cannot be read.");
    }
    $entries = array();
    foreach ($names as $name) {
        if ($name === '.' || $name === '..') {
            continue;
        }
        $item = ($path === '/' ? '' : $path) . '/' . $name;
        $st = @lstat($item);
        if ($st === false) {
            continue;
        }
        $e = $entry($item, $name, $st);
        if ($via !== null) {
            $e['out'] = $out($rule, $via);
        } else {
            $r = rb_preview_by_name($item, $ctx) ?? rb_preview_by_stat($item, $st, $ctx);
            if ($r === 'mount') {
                $e['mount'] = true;
            } elseif ($r !== null) {
                $e['out'] = $out($r);
            }
        }
        $entries[] = $e;
    }
    usort($entries, function ($a, $b) {
        $da = $a['type'] === 'dir' ? 0 : 1;
        $db = $b['type'] === 'dir' ? 0 : 1;
        return $da !== $db ? $da - $db : strnatcasecmp($a['name'], $b['name']);
    });
    $rest = array_splice($entries, $limit);
    return array('path' => $path, 'entries' => $entries, 'more' => count($rest),
                 'more_out' => count(array_filter($rest, function ($e) {
                     return $e['out'] !== null;
                 })));
}

function rb_preview_file_path($id, $suffix = '.json')
{
    return RB_PREVIEW_DIR . '/' . $id . $suffix;
}

function rb_preview_prune()
{
    foreach (glob(RB_PREVIEW_DIR . '/*') ?: array() as $f) {
        if (@filemtime($f) < time() - 86400) {
            @unlink($f);
        }
    }
}
