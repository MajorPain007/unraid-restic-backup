<?php

if (PHP_SAPI !== 'cli') {
    exit(1);
}

require_once dirname(__DIR__) . '/include/lib/ops.php';
require_once dirname(__DIR__) . '/include/lib/schedule.php';
require_once dirname(__DIR__) . '/include/lib/unraid.php';
require_once dirname(__DIR__) . '/include/lib/consistency.php';
require_once dirname(__DIR__) . '/include/lib/browse.php';

rb_use_local_timezone();
@mkdir(RB_RUN_DIR, 0755, true);
@touch(RB_RUN_DIR . '/scheduler.tick');
foreach (rb_journal_recover() as $msg) {
    rb_notify($msg[0], $msg[1], 'warning');
}
rb_browse_reap();
if (!rb_array_started()) {
    exit(0);
}
$cfg = rb_config_load();
$settings = $cfg['settings'];
if (!$cfg['jobs'] && !$cfg['repos']) {
    exit(0);
}
if (rb_prepare_data_dir($settings) !== '') {
    exit(0);
}

$now = time();
$startedAt = rb_array_started_at();
$holdoff = ($startedAt > 0 && $now - $startedAt < $settings['start_delay'] * 60)
        || ($settings['parity_pause'] && rb_parity_running());

$state = rb_state_read($settings);
$started = array();
$record = array('jobs' => array(), 'repos' => array());

foreach ($cfg['jobs'] as $job) {
    if (!$job['enabled'] || !rb_repo($cfg, $job['repo'])) {
        continue;
    }
    $js = $state['jobs'][$job['id']] ?? array();
    $entry = $js['sched'] ?? array();
    $d = rb_sched_decide($job['schedule'], $entry, $now);
    $queued = !empty($js['queued']);
    if (!$d['due'] && !$queued) {
        if ($d['entry'] !== $entry) {
            $record['jobs'][$job['id']] = $d['entry'];
        }
        continue;
    }
    if (($holdoff && !$queued) || isset($started[$job['repo']]) || rb_repo_busy($job['repo'])) {
        continue;
    }
    rb_spawn(array('backup', $job['id'], $d['due'] ? 'schedule' : 'manual'));
    $started[$job['repo']] = true;
    if ($d['due']) {
        $record['jobs'][$job['id']] = $d['entry'];
    }
}

foreach ($cfg['repos'] as $repo) {
    foreach (array('prune', 'check') as $kind) {
        $m = $repo['maintenance'][$kind];
        if (!$m['enabled'] || ($kind === 'prune' && $repo['append_only'])) {
            continue;
        }
        $entry = $state['repos'][$repo['id']][$kind]['sched'] ?? array();
        $d = rb_sched_decide($m['schedule'], $entry, $now);
        if (!$d['due']) {
            if ($d['entry'] !== $entry) {
                $record['repos'][$repo['id']][$kind] = $d['entry'];
            }
            continue;
        }
        if ($holdoff || isset($started[$repo['id']]) || rb_repo_busy($repo['id'])
            || rb_op_running_on($repo['id'], array('restore'))) {
            continue;
        }
        rb_spawn(array($kind, $repo['id'], 'schedule'));
        $started[$repo['id']] = true;
        $record['repos'][$repo['id']][$kind] = $d['entry'];
    }
}

if ($record['jobs'] || $record['repos']) {
    rb_state_update($settings, function (&$s) use ($record) {
        foreach ($record['jobs'] as $id => $entry) {
            $s['jobs'][$id]['sched'] = $entry;
        }
        foreach ($record['repos'] as $id => $kinds) {
            foreach ($kinds as $kind => $entry) {
                $s['repos'][$id][$kind]['sched'] = $entry;
            }
        }
    });
}
