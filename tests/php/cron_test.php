<?php
require __DIR__ . '/harness.php';
require RB_SRC . '/include/lib/cron.php';

date_default_timezone_set('Europe/Berlin');

function at($text)
{
    $t = strtotime($text);
    if ($t === false) {
        throw new Exception("bad time $text");
    }
    return $t;
}

function fmt($ts)
{
    return $ts === null ? null : date('Y-m-d H:i D', $ts);
}

t_group('Parsing');

t_case('mistakes are named, field by field', function () {
    $cases = array(
        ''                => 'five fields',
        '* * * *'         => 'five fields',
        '60 * * * *'      => 'minute: 60 is outside 0-59',
        '* 24 * * *'      => 'hour: 24 is outside 0-23',
        '* * 0 * *'       => 'day of month: 0 is outside 1-31',
        '* * 32 * *'      => 'day of month: 32',
        '* * * 13 *'      => 'month: 13',
        '* * * * 8'       => 'day of week: 8',
        '5-1 * * * *'     => 'runs backwards',
        '*/0 * * * *'     => 'step "0"',
        'x * * * *'       => 'minute: "x" is not a number',
        '* * * foo *'     => 'month: "foo" is not a number or a name',
        '1,,2 * * * *'    => 'empty list entry',
        '@reboot'         => 'five fields',
    );
    foreach ($cases as $expr => $needle) {
        t_true(RbCron::problem($expr) !== '', "\"$expr\" was accepted");
        t_true(stripos(RbCron::problem($expr), $needle) !== false,
            "\"$expr\": \"" . RbCron::problem($expr) . "\" lacks \"$needle\"");
    }
});

t_case('the usual forms are accepted', function () {
    foreach (array('*/15 * * * *', '0 3 * * *', '30 2 * * 1-5', '0 0 1 */3 *', '0 9 * * mon-fri',
                   '0 4 * jan,jul sun', '5/15 * * * *', '  0  3  *  *  *  ', '@daily', '@HOURLY', '0 0 * * 7') as $expr) {
        t_eq('', RbCron::problem($expr), $expr);
    }
});

t_group('Matching and run times');

t_case('a step runs on the multiples', function () {
    $c = RbCron::parse('*/15 * * * *');
    t_true($c->matches(at('2026-10-01 10:15')), '10:15');
    t_true(!$c->matches(at('2026-10-01 10:16')), '10:16');
    t_eq('2026-10-01 10:15 Thu', fmt($c->prev(at('2026-10-01 10:29:59'))));
    t_eq('2026-10-01 10:30 Thu', fmt($c->next(at('2026-10-01 10:15:00'))));
});

t_case('"a/n" starts at a', function () {
    $c = RbCron::parse('5/15 * * * *');
    $mins = array();
    foreach ($c->upcoming(at('2026-10-01 09:59'), 4) as $t) {
        $mins[] = date('i', $t);
    }
    t_eq(array('05', '20', '35', '50'), $mins);
});

t_case('the latest run is today if it has passed, else yesterday', function () {
    $c = RbCron::parse('0 3 * * *');
    t_eq('2026-10-01 03:00 Thu', fmt($c->prev(at('2026-10-01 10:00'))));
    t_eq('2026-09-30 03:00 Wed', fmt($c->prev(at('2026-10-01 02:59'))));
    t_eq('2026-10-01 03:00 Thu', fmt($c->prev(at('2026-10-01 03:00:30'))), 'within the minute');
    t_eq('2026-10-02 03:00 Fri', fmt($c->next(at('2026-10-01 03:00:00'))), 'next is strictly later');
});

t_case('a run at the turn of a month and a year is found', function () {
    $c = RbCron::parse('30 23 31 12 *');
    t_eq('2025-12-31 23:30 Wed', fmt($c->prev(at('2026-10-01 12:00'))));
    t_eq('2026-12-31 23:30 Thu', fmt($c->next(at('2026-10-01 12:00'))));
});

t_case('day of month and day of week: either one when both are given', function () {
    $c = RbCron::parse('0 3 1 * mon');
    $days = array_map(function ($t) { return date('D j', $t); }, $c->upcoming(at('2026-10-01 04:00'), 4));
    t_eq(array('Mon 5', 'Mon 12', 'Mon 19', 'Mon 26'), $days);
    t_eq('Sun 1', date('D j', $c->next(at('2026-10-31 12:00'))), 'November 1st is a Sunday');
});

t_case('a field starting with * makes it both, as in cronie', function () {
    $c = RbCron::parse('0 0 */2 * 1');
    foreach ($c->upcoming(at('2026-10-01'), 6) as $t) {
        t_eq('Mon', date('D', $t), date('Y-m-d', $t));
        t_eq(1, (int)date('j', $t) % 2, date('Y-m-d', $t) . ' is an odd day');
    }
});

t_case('Sunday is 0 and 7, and names work', function () {
    t_eq(fmt(RbCron::parse('0 0 * * 0')->next(at('2026-10-01'))),
         fmt(RbCron::parse('0 0 * * 7')->next(at('2026-10-01'))));
    t_eq('2026-10-04 00:00 Sun', fmt(RbCron::parse('0 0 * * sun')->next(at('2026-10-01'))));
    t_eq('2026-10-02 09:00 Fri', fmt(RbCron::parse('0 9 * * mon-fri')->next(at('2026-10-01 09:00'))));
    t_eq('2026-10-05 09:00 Mon', fmt(RbCron::parse('0 9 * * mon-fri')->next(at('2026-10-02 09:00'))));
});

t_case('February 29th is found years away', function () {
    $c = RbCron::parse('0 0 29 2 *');
    t_eq('2024-02-29 00:00 Thu', fmt($c->prev(at('2026-10-01'))));
    t_eq('2028-02-29 00:00 Tue', fmt($c->next(at('2026-10-01'))));
});

t_case('an expression that never fires gives no time', function () {
    $c = RbCron::parse('0 0 31 2 *');
    t_eq(null, $c->prev(at('2026-10-01')));
    t_eq(null, $c->next(at('2026-10-01')));
});

t_case('the skipped hour of a DST change runs an hour late, once', function () {
    $c = RbCron::parse('30 2 * * *');
    t_eq('2026-03-29 03:30 Sun', fmt($c->next(at('2026-03-28 12:00'))));
    t_eq('2026-03-28 02:30 Sat', fmt($c->prev(at('2026-03-29 03:10'))), 'not yet at 03:10');
    t_eq('2026-03-29 03:30 Sun', fmt($c->prev(at('2026-03-29 03:40'))));
    t_eq('2026-03-30 02:30 Mon', fmt($c->next(at('2026-03-29 03:30'))));
});

t_case('the macros mean what cron means', function () {
    t_eq('2026-10-02 00:00 Fri', fmt(RbCron::parse('@daily')->next(at('2026-10-01 00:00'))));
    t_eq('2026-10-04 00:00 Sun', fmt(RbCron::parse('@weekly')->next(at('2026-10-01'))));
    t_eq('2026-11-01 00:00 Sun', fmt(RbCron::parse('@monthly')->next(at('2026-10-01'))));
    t_eq('2027-01-01 00:00 Fri', fmt(RbCron::parse('@yearly')->next(at('2026-10-01'))));
    t_eq('2026-10-01 11:00 Thu', fmt(RbCron::parse('@hourly')->next(at('2026-10-01 10:00'))));
});

t_case('a search is quick even for a rare schedule', function () {
    $c = RbCron::parse('0 0 29 2 1');
    $t0 = microtime(true);
    for ($i = 0; $i < 50; $i++) {
        RbCron::parse('0 0 29 2 *')->prev(at('2026-10-01'));
    }
    t_true(microtime(true) - $t0 < 2.0, 'fifty look-ups took ' . round(microtime(true) - $t0, 2) . ' s');
    t_eq('2027-02-01 00:00 Mon', fmt($c->next(at('2026-10-01'))), 'the first Monday in February');
});

t_done();
