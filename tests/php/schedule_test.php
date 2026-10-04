<?php
require __DIR__ . '/harness.php';
require RB_SRC . '/include/lib/schedule.php';

date_default_timezone_set('Europe/Berlin');

function at($text)
{
    return strtotime($text);
}

function cron($expr, $catchUp = true)
{
    return array('mode' => 'cron', 'cron' => $expr, 'every' => 24, 'unit' => 'hours', 'catch_up' => $catchUp);
}

function every($n, $unit)
{
    return array('mode' => 'interval', 'cron' => '', 'every' => $n, 'unit' => $unit, 'catch_up' => true);
}

function simulate(array $s, $from, $to, array &$entry, $up = null)
{
    $runs = array();
    for ($t = at($from); $t <= at($to); $t += 60) {
        if ($up !== null && !$up($t)) {
            continue;
        }
        $d = rb_sched_decide($s, $entry, $t);
        $entry = $d['entry'];
        if ($d['due']) {
            $runs[] = date('m-d H:i', $t);
        }
    }
    return $runs;
}

t_group('Cron schedules');

t_case('a daily job runs once a day at its time', function () {
    $e = array();
    t_eq(array('10-02 03:00', '10-03 03:00'), simulate(cron('0 3 * * *'), '2026-10-01 10:00', '2026-10-03 23:59', $e));
});

t_case('a new job does not fire for a time that passed before it was saved', function () {
    $e = array();
    $d = rb_sched_decide(cron('0 3 * * *'), $e, at('2026-10-01 10:00'));
    t_eq(false, $d['due']);
    t_eq(at('2026-10-01 03:00'), $d['entry']['slot']);
});

t_case('a slot missed while the server was off runs when it is back - once', function () {
    $e = array();
    $off = function ($t) { return $t < at('2026-10-02 00:00') || $t >= at('2026-10-05 09:00'); };
    t_eq(array('10-05 09:00'), simulate(cron('0 3 * * *'), '2026-10-01 10:00', '2026-10-05 12:00', $e, $off),
         'three missed nights, one catch-up run');
});

t_case('with catching up off, a missed slot is passed over', function () {
    $e = array();
    $off = function ($t) { return $t < at('2026-10-02 00:00') || $t >= at('2026-10-02 09:00'); };
    t_eq(array('10-03 03:00'), simulate(cron('0 3 * * *', false), '2026-10-01 10:00', '2026-10-03 12:00', $e, $off));
});

t_case('...but a slot only minutes late still runs', function () {
    $e = array();
    $late = function ($t) { return $t < at('2026-10-02 03:00') || $t >= at('2026-10-02 03:04'); };
    t_eq(array('10-02 03:04'), simulate(cron('0 3 * * *', false), '2026-10-01 10:00', '2026-10-02 12:00', $e, $late));
});

t_case('a slot that could not start stays due until it can', function () {
    $s = cron('0 3 * * *');
    $e = rb_sched_decide($s, array(), at('2026-10-01 10:00'))['entry'];
    $busyUntil = at('2026-10-02 03:47');
    $started = null;
    for ($t = at('2026-10-02 02:58'); $t <= at('2026-10-02 05:00') && $started === null; $t += 60) {
        $d = rb_sched_decide($s, $e, $t);
        if ($d['due'] && $t >= $busyUntil) {
            $started = date('H:i', $t);
            $e = $d['entry'];
        }
    }
    t_eq('03:47', $started);
});

t_case('changing the schedule starts it afresh', function () {
    $e = array();
    simulate(cron('0 3 * * *'), '2026-10-01 10:00', '2026-10-01 10:05', $e);
    $d = rb_sched_decide(cron('0 9 * * *'), $e, at('2026-10-01 10:06'));
    t_eq(false, $d['due'], 'the 09:00 slot that passed today does not count');
    t_eq('cron:0 9 * * *', $d['entry']['key']);
});

t_case('every 15 minutes runs at the quarter hours', function () {
    $e = array();
    t_eq(array('10-01 10:15', '10-01 10:30', '10-01 10:45', '10-01 11:00'),
         simulate(cron('*/15 * * * *'), '2026-10-01 10:01', '2026-10-01 11:05', $e));
});

t_group('Interval schedules');

t_case('every 6 hours counts from when the schedule was set', function () {
    $e = array();
    t_eq(array('10-01 16:00', '10-01 22:00', '10-02 04:00'),
         simulate(every(6, 'hours'), '2026-10-01 10:00', '2026-10-02 05:00', $e));
});

t_case('a run started by hand moves the next one', function () {
    $e = rb_sched_decide(every(6, 'hours'), array(), at('2026-10-01 10:00'))['entry'];
    $e['last_start'] = at('2026-10-01 14:00');
    t_eq(at('2026-10-01 20:00'), rb_sched_next(every(6, 'hours'), $e, at('2026-10-01 14:01')));
});

t_case('a long outage gives one run, not one per interval', function () {
    $e = array();
    $off = function ($t) { return $t < at('2026-10-01 11:00') || $t >= at('2026-10-03 08:00'); };
    t_eq(array('10-03 08:00', '10-03 09:00'),
         simulate(every(1, 'hours'), '2026-10-01 10:00', '2026-10-03 09:30', $e, $off));
});

t_group('Next run and description');

t_case('the next run of a cron job', function () {
    $s = cron('0 3 * * *');
    $e = rb_sched_decide($s, array(), at('2026-10-01 10:00'))['entry'];
    t_eq(at('2026-10-02 03:00'), rb_sched_next($s, $e, at('2026-10-01 10:00')));
    t_eq(at('2026-10-02 03:00'), rb_sched_next($s, $e, at('2026-10-02 03:30')),
         'a due slot not yet started is still the next run');
    t_eq(null, rb_sched_next(array('mode' => 'off') + $s, $e, at('2026-10-01 10:00')));
});

t_case('schedules read as words', function () {
    t_eq('Daily at 03:00', rb_sched_describe(cron('0 3 * * *')));
    t_eq('Sundays at 04:30', rb_sched_describe(cron('30 4 * * 0')));
    t_eq('Monthly on the 1st at 05:00', rb_sched_describe(cron('0 5 1 * *')));
    t_eq('Monthly on the 22nd at 05:00', rb_sched_describe(cron('0 5 22 * *')));
    t_eq('Hourly at :15', rb_sched_describe(cron('15 * * * *')));
    t_eq('Cron */15 * * * *', rb_sched_describe(cron('*/15 * * * *')));
    t_eq('Every 6 hours', rb_sched_describe(every(6, 'hours')));
    t_eq('Every day', rb_sched_describe(every(1, 'days')));
    t_eq('Manual only', rb_sched_describe(array('mode' => 'off')));
});

t_done();
