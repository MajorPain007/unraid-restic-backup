<?php

require_once __DIR__ . '/cron.php';

define('RB_SCHED_GRACE', 600);

function rb_sched_key(array $s)
{
    if ($s['mode'] === 'cron') {
        return 'cron:' . $s['cron'];
    }
    if ($s['mode'] === 'interval') {
        return 'interval:' . $s['every'] . $s['unit'][0];
    }
    return 'off';
}

function rb_sched_interval(array $s)
{
    $unit = array('minutes' => 60, 'hours' => 3600, 'days' => 86400);
    return $s['every'] * $unit[$s['unit']];
}

function rb_sched_decide(array $s, array $entry, $now)
{
    $key = rb_sched_key($s);
    $out = array('due' => false, 'skip' => false, 'entry' => $entry);
    if ($key === 'off') {
        return $out;
    }
    if (($entry['key'] ?? '') !== $key) {
        $out['entry'] = array('key' => $key, 'since' => $now, 'slot' => 0);
        if ($s['mode'] === 'cron') {
            $out['entry']['slot'] = (int)RbCron::parse($s['cron'])->prev($now);
        }
        return $out;
    }
    if ($s['mode'] === 'cron') {
        $slot = RbCron::parse($s['cron'])->prev($now);
        if ($slot === null || $slot <= ($entry['slot'] ?? 0)) {
            return $out;
        }
        $out['entry']['slot'] = $slot;
        if (!$s['catch_up'] && $now - $slot > RB_SCHED_GRACE) {
            $out['skip'] = true;
        } else {
            $out['due'] = true;
        }
        return $out;
    }
    $from = max((int)($entry['since'] ?? 0), (int)($entry['last_start'] ?? 0));
    if ($now >= $from + rb_sched_interval($s)) {
        $out['due'] = true;
        $out['entry']['last_start'] = $now;
    }
    return $out;
}

function rb_sched_next(array $s, array $entry, $now)
{
    $key = rb_sched_key($s);
    if ($key === 'off') {
        return null;
    }
    if ($s['mode'] === 'cron') {
        $c = RbCron::parse($s['cron']);
        if (($entry['key'] ?? '') === $key) {
            $slot = $c->prev($now);
            if ($slot !== null && $slot > ($entry['slot'] ?? 0) && ($s['catch_up'] || $now - $slot <= RB_SCHED_GRACE)) {
                return $slot;
            }
        }
        return $c->next($now);
    }
    $from = ($entry['key'] ?? '') === $key
        ? max((int)($entry['since'] ?? 0), (int)($entry['last_start'] ?? 0))
        : $now;
    return $from + rb_sched_interval($s);
}

function rb_sched_describe(array $s)
{
    if ($s['mode'] === 'off') {
        return 'Manual only';
    }
    if ($s['mode'] === 'interval') {
        $unit = $s['every'] === 1 ? rtrim($s['unit'], 's') : $s['unit'];
        return 'Every ' . ($s['every'] === 1 ? '' : $s['every'] . ' ') . $unit;
    }
    $f = explode(' ', $s['cron']);
    if (count($f) === 5 && ctype_digit($f[0]) && ctype_digit($f[1])) {
        $time = sprintf('%02d:%02d', $f[1], $f[0]);
        $days = array('Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday');
        if ($f[2] === '*' && $f[3] === '*' && $f[4] === '*') {
            return "Daily at $time";
        }
        if ($f[2] === '*' && $f[3] === '*' && ctype_digit($f[4]) && (int)$f[4] <= 7) {
            return $days[(int)$f[4]] . "s at $time";
        }
        if (ctype_digit($f[2]) && $f[3] === '*' && $f[4] === '*') {
            $n = (int)$f[2];
            $suffix = ($n % 10 === 1 && $n !== 11) ? 'st' : (($n % 10 === 2 && $n !== 12) ? 'nd' : (($n % 10 === 3 && $n !== 13) ? 'rd' : 'th'));
            return "Monthly on the $n$suffix at $time";
        }
    }
    if (count($f) === 5 && ctype_digit($f[0]) && $f[1] === '*' && $f[2] === '*' && $f[3] === '*' && $f[4] === '*') {
        return sprintf('Hourly at :%02d', $f[0]);
    }
    return 'Cron ' . $s['cron'];
}
