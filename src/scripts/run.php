<?php

if (PHP_SAPI !== 'cli') {
    exit(1);
}

require_once dirname(__DIR__) . '/include/lib/ops.php';
require_once dirname(__DIR__) . '/include/lib/restic.php';
require_once dirname(__DIR__) . '/include/lib/unraid.php';
require_once dirname(__DIR__) . '/include/lib/schedule.php';
require_once dirname(__DIR__) . '/include/lib/jobs.php';
require_once dirname(__DIR__) . '/include/lib/consistency.php';
require_once dirname(__DIR__) . '/include/lib/browse.php';

rb_use_local_timezone();

class RbRun
{
    public $op;
    public $cfg;
    public $settings;
    public $repo;
    public $job;
    public $errors = array();
    private $lock;
    private $logFile;
    private $lastWrite = 0.0;
    private $cleanups = array();
    private $finished = false;

    public function __construct($kind, array $cfg, array $repo, $job, $trigger, $lock)
    {
        $this->lock = $lock;
        $this->cfg = $cfg;
        $this->settings = $cfg['settings'];
        $this->repo = $repo;
        $this->job = $job;
        $id = rb_new_op_id();
        $logDir = (rb_array_started() && rb_prepare_data_dir($this->settings) === '')
            ? $this->settings['data_dir'] . '/logs' : RB_RUN_DIR . '/logs';
        $this->logFile = $logDir . '/' . $id . '.log';
        $this->op = array(
            'id'         => $id,
            'kind'       => $kind,
            'job'        => $job ? $job['id'] : '',
            'job_name'   => $job ? $job['name'] : '',
            'repo'       => $repo['id'],
            'repo_name'  => $repo['name'],
            'trigger'    => $trigger,
            'started'    => time(),
            'ended'      => 0,
            'status'     => 'running',
            'phase'      => 'Starting',
            'progress'   => null,
            'runner_pid' => getmypid(),
            'pid'        => 0,
            'log'        => $this->logFile,
            'error'      => '',
            'summary'    => null,
        );
        register_shutdown_function(array($this, 'shutdown'));
    }

    public function id()
    {
        return $this->op['id'];
    }

    public function log($msg)
    {
        rb_log_append($this->logFile, $msg, 16 * 1048576);
    }

    public function phase($name)
    {
        $this->op['phase'] = $name;
        $this->op['progress'] = null;
        $this->write(true);
        $this->log("== $name");
    }

    public function write($force = false)
    {
        $now = microtime(true);
        if ($force || $now - $this->lastWrite >= 1.0) {
            $this->lastWrite = $now;
            rb_op_write($this->op);
        }
    }

    public function cancelled()
    {
        return rb_op_cancel_requested($this->op['id']);
    }

    public function onCleanup($label, $fn)
    {
        $this->cleanups[] = array($label, $fn);
    }

    public function runCleanups()
    {
        while ($c = array_pop($this->cleanups)) {
            try {
                $this->log(ucfirst($c[0]));
                $c[1]();
            } catch (Throwable $e) {
                $this->errors[] = "{$c[0]} failed: " . $e->getMessage();
                $this->log("{$c[0]} failed: " . $e->getMessage());
            }
        }
    }

    public function restic(array $args, $onJson = null, $wrap = null)
    {
        $errLines = array();
        $self = $this;
        $cmd = rb_restic_cmd($this->repo, $this->settings, $args);
        if ($wrap) {
            $cmd = $wrap($cmd);
        }
        $this->log('$ restic ' . implode(' ', array_map(function ($a) {
            return preg_match('/^[A-Za-z0-9_\/.:=@%+-]+$/', $a) ? $a : "'" . str_replace("'", "'\\''", $a) . "'";
        }, $args)));
        $code = rb_proc_stream(
            $cmd,
            rb_restic_env($this->repo, $this->settings),
            function ($stream, $line) use ($self, $onJson, &$errLines) {
                if ($stream === 'out') {
                    $j = json_decode($line, true);
                    if (is_array($j) && $onJson) {
                        $onJson($j);
                    } elseif ($line !== '') {
                        $self->log($line);
                    }
                    return;
                }
                if (count($errLines) < 500) {
                    $errLines[] = $line;
                }
                $j = json_decode($line, true);
                if (is_array($j) && ($j['message_type'] ?? '') === 'error') {
                    $self->log('error: ' . ($j['error']['message'] ?? $line));
                } elseif (is_array($j) && ($j['message_type'] ?? '') === 'exit_error') {
                    $self->log('restic: ' . ($j['message'] ?? $line));
                } elseif ($line !== '') {
                    $self->log($line);
                }
            },
            function () use ($self) {
                $self->write(false);
                return $self->cancelled();
            },
            function ($pid) use ($self) {
                $self->op['pid'] = $pid;
                $self->write(true);
            }
        );
        $this->op['pid'] = 0;
        $this->log("exit code $code");
        return array($code, $errLines);
    }

    public function finish($status, $error = '', $summary = null)
    {
        if ($this->finished) {
            return;
        }
        $this->runCleanups();
        if ($status === 'success' && $this->errors) {
            $status = 'warning';
        }
        if ($error === '' && $this->errors) {
            $error = implode('; ', array_slice($this->errors, 0, 3));
        }
        $this->finished = true;
        if ($this->op['kind'] !== 'restore') {
            rb_browse_stop($this->repo['id']);
        }
        $this->op['status'] = $status;
        $this->op['error'] = $error;
        $this->op['summary'] = $summary;
        $this->op['ended'] = time();
        $this->op['phase'] = 'Finished';
        $this->op['progress'] = null;
        $this->write(true);
        $this->log("Finished: $status" . ($error !== '' ? " - $error" : ''));

        $record = $this->op;
        unset($record['progress'], $record['phase'], $record['runner_pid'], $record['pid'], $record['updated']);
        rb_history_append($this->settings, $record);

        $op = $this->op;
        $counted = $op['kind'] === 'backup' && in_array($op['status'], array('success', 'warning'), true) && is_array($op['summary']);
        $backfill = $counted && empty(rb_state_read($this->settings)['jobs'][$op['job']]['sizes'])
            ? rb_job_sizes_from_history($this->settings, $op['job']) : array();
        rb_state_update($this->settings, function (&$state) use ($op, $counted, $backfill) {
            if ($counted) {
                $sizes = $state['jobs'][$op['job']]['sizes'] ?? $backfill;
                if (!$sizes || end($sizes)[0] !== (int)$op['ended']) {
                    $sizes[] = rb_size_point($op);
                }
                $state['jobs'][$op['job']]['sizes'] = array_slice($sizes, -RB_SIZES_KEEP);
            }
            if ($op['job'] !== '' && $op['kind'] === 'backup') {
                $state['jobs'][$op['job']]['last'] = array(
                    'id' => $op['id'], 'status' => $op['status'], 'started' => $op['started'],
                    'ended' => $op['ended'], 'error' => $op['error'], 'summary' => $op['summary'],
                );
                if ($op['status'] === 'success' || $op['status'] === 'warning') {
                    $state['jobs'][$op['job']]['last_success'] = $op['ended'];
                }
                if (isset($state['jobs'][$op['job']]['sched'])) {
                    $state['jobs'][$op['job']]['sched']['last_start'] = $op['started'];
                }
                unset($state['jobs'][$op['job']]['queued']);
            } elseif (in_array($op['kind'], array('prune', 'check'), true)) {
                $state['repos'][$op['repo']][$op['kind']]['last'] = array(
                    'id' => $op['id'], 'status' => $op['status'], 'ended' => $op['ended'], 'error' => $op['error'],
                );
            }
        });
        @unlink(RB_RUN_DIR . '/cache/snapshots-' . $this->repo['id'] . '.json');
        $this->notify();
        if ($this->lock) {
            flock($this->lock, LOCK_UN);
            fclose($this->lock);
            $this->lock = null;
        }
        rb_ops_prune();
    }

    private function notify()
    {
        $op = $this->op;
        $what = $op['kind'] === 'backup' ? 'Backup "' . $op['job_name'] . '"'
              : ucfirst($op['kind']) . ' of "' . $op['repo_name'] . '"';
        $n = $this->job ? $this->job['notify'] : array('success' => false, 'warning' => true, 'error' => true, 'healthcheck' => '');
        $took = rb_human_duration($op['ended'] - $op['started']);
        switch ($op['status']) {
            case 'success':
                $detail = $this->summaryText();
                if ($n['success']) {
                    rb_notify("$what finished", "Finished in $took" . ($detail !== '' ? ". $detail" : ''), 'normal');
                }
                break;
            case 'warning':
                if ($n['warning']) {
                    rb_notify("$what finished with warnings", $op['error'], 'warning');
                }
                break;
            case 'cancelled':
                break;
            default:
                if ($n['error']) {
                    rb_notify("$what failed", $op['error'], 'alert');
                }
        }
        if ($this->job && $n['healthcheck'] !== '' && $op['kind'] === 'backup' && $op['status'] !== 'cancelled') {
            rb_healthcheck_ping($n['healthcheck'], $op['status'] === 'error' ? 'fail' : 'success',
                                $op['error'] !== '' ? $op['error'] : $this->summaryText());
        }
    }

    public function summaryText($s = null)
    {
        $s = $s === null ? $this->op['summary'] : $s;
        if (!is_array($s) || !isset($s['data_added'])) {
            return '';
        }
        return sprintf('%d new, %d changed files; %s added', $s['files_new'] ?? 0, $s['files_changed'] ?? 0,
                       rb_human_bytes($s['data_added_packed'] ?? $s['data_added']));
    }

    public function shutdown()
    {
        if (!$this->finished) {
            $e = error_get_last();
            $this->finish('error', 'The operation ended unexpectedly' . ($e ? ': ' . $e['message'] : '.'));
        }
    }
}

function rb_run_fail_early($msg)
{
    fwrite(STDERR, "$msg\n");
    exit(1);
}

function rb_preflight(RbRun $r)
{
    if (!rb_array_started()) {
        return 'The array is not started.';
    }
    if (!is_executable(RB_RESTIC)) {
        return 'restic is not installed (' . RB_RESTIC . '). Reinstall the plugin.';
    }
    $err = rb_prepare_data_dir($r->settings);
    if ($err !== '') {
        return $err . ' - is the data folder in Settings on a disk that is mounted?';
    }
    if (!rb_secret_has($r->repo['id'], 'password')) {
        return 'The repository has no password set.';
    }
    if ($r->repo['type'] === 'local' && !is_file($r->repo['local']['path'] . '/config')) {
        return 'No repository at ' . $r->repo['local']['path'] . ' - is the disk mounted?';
    }
    if ($r->repo['type'] === 'sftp' && !is_file(rb_ssh_key_file($r->repo['id']))) {
        return 'The repository has no SSH key yet.';
    }
    $r->phase('Reaching the repository');
    return rb_repo_reachable($r->repo, 3, 20, function () use ($r) {
        return $r->cancelled();
    }, function ($msg) use ($r) {
        $r->log($msg);
    });
}

function rb_run_hook(RbRun $r, $name, $script, array $env)
{
    $r->phase("Running the $name command");
    $code = rb_proc_stream(
        array('/bin/bash', '-c', $script),
        $env + array('PATH' => '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin', 'HOME' => '/root'),
        function ($stream, $line) use ($r) {
            $r->log("[$stream] $line");
        },
        function () use ($r) {
            return $r->cancelled();
        }
    );
    $r->log("$name command exited with $code");
    return $code;
}

function rb_hook_env(RbRun $r, $status = '', $snapshot = '')
{
    return array(
        'RB_OPERATION' => $r->op['id'],
        'RB_JOB_ID'    => $r->job['id'],
        'RB_JOB_NAME'  => $r->job['name'],
        'RB_REPO_NAME' => $r->repo['name'],
        'RB_SOURCES'   => implode("\n", $r->job['sources']),
        'RB_STATUS'    => $status,
        'RB_SNAPSHOT'  => $snapshot,
        'RB_LOG'       => $r->op['log'],
    );
}

function rb_restic_unlocking(RbRun $r, array $args, $onJson = null, $wrap = null)
{
    list($code, $err) = $r->restic($args, $onJson, $wrap);
    if ($code === 11 && $r->repo['unlock_stale'] && !$r->cancelled()) {
        $r->log('The repository is locked; removing stale locks and trying again.');
        $r->restic(array('unlock'));
        list($code, $err) = $r->restic($args, $onJson, $wrap);
    }
    return array($code, $err);
}

function rb_prepare_consistency(RbRun $r, array $job)
{
    $c = $job['consistency'];
    $mode = $c['mode'];
    $opId = $r->id();
    $log = function ($m) use ($r) {
        $r->log($m);
    };
    $j = (object)array('containers' => array(), 'vms' => array(), 'snapshots' => array(), 'snap' => '',
                       'log' => $r->op['log']);
    $save = function () use ($opId, $j) {
        rb_journal_write($opId, (array)$j);
    };
    $r->onCleanup('clearing the journal', function () use ($opId, $j, $save) {
        if ($j->snapshots) {
            $j->ended = true;
            $save();
        } else {
            rb_journal_clear($opId);
        }
    });
    $startAgain = function () use ($r, $j, $log, $save) {
        if ($j->containers) {
            foreach (rb_docker_start($j->containers, $log) as $f) {
                $r->errors[] = "Container $f did not start again";
            }
            $j->containers = array();
        }
        if ($j->vms) {
            foreach (rb_vm_start($j->vms, $log) as $f) {
                $r->errors[] = "VM $f did not start again";
            }
            $j->vms = array();
        }
        $save();
    };

    $plan = null;
    if ($mode === 'zfs' || $mode === 'zfs-stop') {
        try {
            $plan = rb_zfs_plan($job['sources']);
        } catch (Throwable $e) {
            $r->finish('error', $e->getMessage());
            return array(false, null);
        }
        if ($plan['live']) {
            $r->log('Not on ZFS, read as they are: ' . implode(', ', array_map(function ($l) {
                return "{$l[0]} ({$l[1]})";
            }, $plan['live'])));
        }
        list($code) = rb_cmd(array('unshare', '--help'), 10);
        if ($code !== 0) {
            $r->finish('error', 'unshare is not available on this server, so the ZFS snapshot cannot be mounted for restic.');
            return array(false, null);
        }
    }

    if ($mode === 'stop' || $mode === 'zfs-stop') {
        $r->phase('Stopping what uses the data');
        $detected = rb_consistency_detect($job['sources']);
        $running = array();
        foreach (rb_docker_containers() ?: array() as $ct) {
            if ($ct['running']) {
                $running[$ct['name']] = true;
            }
        }
        $runningVms = array();
        foreach (rb_vms() ?: array() as $vm) {
            if ($vm['running']) {
                $runningVms[$vm['name']] = true;
            }
        }
        $stopC = array_values(array_filter($c['containers'], function ($n) use ($running) {
            return isset($running[$n]);
        }));
        $stopV = array_values(array_filter($c['vms'], function ($n) use ($runningVms) {
            return isset($runningVms[$n]);
        }));
        foreach (array_merge($detected['containers'], $detected['vms']) as $x) {
            if ($x['running'] && !in_array($x['name'], $c['containers'], true) && !in_array($x['name'], $c['vms'], true)
                && !in_array($x['name'], $c['skip'], true)) {
                $r->errors[] = "{$x['name']} uses {$x['path']} but is not in the job's list, so it kept running";
            }
        }
        $r->onCleanup('starting containers and VMs again', $startAgain);
        try {
            if ($stopC) {
                $j->containers = $stopC;
                $save();
                rb_docker_stop($stopC, $log);
            }
            if ($stopV) {
                $stopped = array();
                $j->vms = $stopV;
                $save();
                rb_vm_stop($stopV, $c['timeout'], $log, $stopped, function () use ($r) {
                    return $r->cancelled();
                });
            }
        } catch (Throwable $e) {
            $r->finish($r->cancelled() ? 'cancelled' : 'error', $e->getMessage());
            return array(false, null);
        }
        if (!$stopC && !$stopV) {
            $r->log('Nothing to stop: none of the listed containers or VMs is running.');
        }
    }

    if (!$plan) {
        return array(true, null);
    }
    $r->phase('Taking ZFS snapshots');
    $snap = 'restic-backup-' . $opId;
    $j->snap = $snap;
    $j->snapshots = $plan['datasets'];
    $save();
    $stage = RB_RUN_DIR . '/ns/' . $opId;
    $r->onCleanup('removing the ZFS snapshots', function () use ($r, $j, $plan, $snap, $stage, $save, $log) {
        rb_zfs_unstage($stage);
        $left = rb_zfs_destroy($plan['datasets'], $snap, $log);
        foreach ($left as $name => $why) {
            $r->errors[] = "Snapshot $name could not be removed yet ($why); it is tried again every minute";
        }
        $j->snapshots = array_values(array_filter($plan['datasets'], function ($d) use ($left, $snap) {
            return isset($left["$d@$snap"]);
        }));
        $save();
    });
    try {
        rb_zfs_snapshot($plan['datasets'], $snap);
        $r->log('Snapshots taken: ' . implode(', ', array_map(function ($d) use ($snap) {
            return "$d@$snap";
        }, $plan['datasets'])));
    } catch (Throwable $e) {
        $r->finish('error', $e->getMessage());
        return array(false, null);
    }
    if ($mode === 'zfs-stop') {
        $r->phase('Starting containers and VMs again');
        $startAgain();
    }
    return array(true, function (array $cmd) use ($plan, $snap, $stage) {
        return rb_zfs_wrap($cmd, $plan, $snap, $stage);
    });
}

function rb_apply_retention(RbRun $r, array $job)
{
    $r->phase('Applying the retention policy');
    $groups = null;
    list($code, $err) = rb_restic_unlocking($r, rb_forget_args($job), function ($j) use (&$groups) {
        if (!isset($j['message_type'])) {
            $groups = $j;
        }
    });
    if ($code !== 0) {
        $msg = rb_restic_error($err, $code);
        if (stripos(implode("\n", $err), 'unable to remove') !== false) {
            $msg = 'The server keeps this repository append-only, so old snapshots cannot be removed from here. '
                 . 'Clean it up on the server, and mark the repository append-only in its settings.';
        }
        $r->errors[] = "Retention: $msg";
        return null;
    }
    $removed = 0;
    $kept = 0;
    foreach (is_array($groups) ? $groups : array() as $g) {
        $removed += count($g['remove'] ?? array());
        $kept += count($g['keep'] ?? array());
    }
    $r->log("Retention: kept $kept, removed $removed snapshot(s)");
    return array($removed, $kept);
}

function rb_record_stats(RbRun $r)
{
    $stats = null;
    list($code) = $r->restic(array('stats', '--json', '--mode', 'raw-data', '--no-lock'), function ($j) use (&$stats) {
        if (isset($j['total_size'])) {
            $stats = $j;
        }
    });
    if ($code === 0 && $stats) {
        $stats['at'] = time();
        $repoId = $r->repo['id'];
        rb_state_update($r->settings, function (&$state) use ($repoId, $stats) {
            $state['repos'][$repoId]['stats'] = $stats;
            rb_state_add_repo_size($state, $repoId, $stats);
        });
    }
    return $stats;
}

function rb_run_backup(RbRun $r)
{
    $job = $r->job;
    $tmp = RB_OPS_DIR . '/' . $r->id();
    $r->onCleanup('removing temporary files', function () use ($tmp) {
        @unlink("$tmp.exclude");
        @unlink("$tmp.iexclude");
    });
    rb_healthcheck_ping($job['notify']['healthcheck'], 'start');

    $r->phase('Checking the sources');
    foreach ($job['sources'] as $src) {
        if (!file_exists($src)) {
            return $r->finish('error', "Source $src does not exist.");
        }
        if (is_dir($src) && !rb_dir_has_entries($src)) {
            return $r->finish('error', "Source $src is empty - is the disk mounted? Nothing was backed up.");
        }
    }

    if ($job['hooks']['before'] !== '') {
        $code = rb_run_hook($r, 'before-backup', $job['hooks']['before'], rb_hook_env($r));
        if ($r->cancelled()) {
            return $r->finish('cancelled', 'Stopped.');
        }
        if ($code !== 0) {
            return $r->finish('error', "The before-backup command failed (exit code $code); nothing was backed up.");
        }
    }

    $wrap = null;
    if ($job['consistency']['mode'] !== 'none') {
        list($ok, $wrap) = rb_prepare_consistency($r, $job);
        if (!$ok) {
            return null;
        }
        if ($r->cancelled()) {
            return $r->finish('cancelled', 'Stopped.');
        }
    }

    $summary = null;
    $errorCount = 0;
    $r->phase('Backing up');
    list($code, $errLines) = rb_restic_unlocking($r, rb_backup_args($job, $r->repo, $r->settings, $tmp),
        function ($j) use ($r, &$summary, &$errorCount) {
            $type = $j['message_type'] ?? '';
            if ($type === 'status') {
                $r->op['progress'] = array(
                    'percent'     => (float)($j['percent_done'] ?? 0),
                    'files_done'  => (int)($j['files_done'] ?? 0),
                    'total_files' => (int)($j['total_files'] ?? 0),
                    'bytes_done'  => (int)($j['bytes_done'] ?? 0),
                    'total_bytes' => (int)($j['total_bytes'] ?? 0),
                    'errors'      => (int)($j['error_count'] ?? 0),
                    'current'     => (string)($j['current_files'][0] ?? ''),
                    'eta'         => isset($j['seconds_remaining']) ? (int)$j['seconds_remaining'] : null,
                    'at'          => time(),
                );
                $errorCount = (int)($j['error_count'] ?? $errorCount);
            } elseif ($type === 'summary') {
                unset($j['message_type']);
                $summary = $j;
            }
        }, $wrap);
    $snapshot = (string)($summary['snapshot_id'] ?? '');
    if ($summary) {
        $r->log('Snapshot ' . substr($snapshot, 0, 8) . ': ' . $r->summaryText($summary));
    }
    if ($job['consistency']['mode'] !== 'none') {
        $r->phase('Cleaning up');
    }
    $r->runCleanups();

    if ($r->cancelled() || $code === 130) {
        return $r->finish('cancelled', 'Stopped - no snapshot was saved.', $summary);
    }
    if ($code !== 0 && $code !== 3) {
        return $r->finish('error', rb_restic_error($errLines, $code), $summary);
    }
    $status = 'success';
    $warning = '';
    if ($code === 3) {
        $status = 'warning';
        $warning = 'Some files could not be read and are missing from the snapshot - see the log.';
    }

    if ($job['retention']['enabled'] && !$r->repo['append_only'] && !$r->cancelled()) {
        $result = rb_apply_retention($r, $job);
        if ($result && is_array($summary)) {
            list($summary['forget_removed'], $summary['forget_kept']) = $result;
        }
    }

    $state = rb_state_read($r->settings);
    if (($state['repos'][$r->repo['id']]['stats']['at'] ?? 0) < time() - 6 * 3600 && !$r->cancelled()) {
        $r->phase('Measuring the repository');
        rb_record_stats($r);
    }

    if ($job['hooks']['after'] !== '') {
        $code = rb_run_hook($r, 'after-backup', $job['hooks']['after'], rb_hook_env($r, $status, $snapshot));
        if ($code !== 0) {
            $r->errors[] = "The after-backup command failed (exit code $code)";
        }
    }
    return $r->finish($status, $warning, $summary);
}

function rb_run_forget(RbRun $r)
{
    if ($r->repo['append_only']) {
        return $r->finish('error', 'The repository is append-only; snapshots can only be removed on the server.');
    }
    $result = rb_apply_retention($r, $r->job);
    if ($result === null) {
        return $r->finish('error', implode('; ', $r->errors));
    }
    $r->errors = array();
    return $r->finish('success', '', array('forget_removed' => $result[0], 'forget_kept' => $result[1]));
}

function rb_run_prune(RbRun $r)
{
    if ($r->repo['append_only']) {
        return $r->finish('error', 'The repository is append-only; prune has to run on the server.');
    }
    $r->phase('Removing data no snapshot needs');
    list($code, $err) = rb_restic_unlocking($r, array(
        'prune', '--max-unused', $r->repo['maintenance']['prune']['max_unused'], '--retry-lock', '5m'));
    if ($r->cancelled() || $code === 130) {
        return $r->finish('cancelled', 'Stopped. Prune can be stopped safely; it continues next time.');
    }
    if ($code !== 0) {
        return $r->finish('error', rb_restic_error($err, $code));
    }
    $r->phase('Measuring the repository');
    return $r->finish('success', '', rb_record_stats($r));
}

function rb_run_check(RbRun $r)
{
    $c = $r->repo['maintenance']['check'];
    $args = array('check', '--json', '--retry-lock', '5m');
    if ($c['read_data'] === 'subset') {
        array_push($args, '--read-data-subset', $c['subset']);
    } elseif ($c['read_data'] === 'all') {
        $args[] = '--read-data';
    }
    $r->phase($c['read_data'] === 'none' ? 'Checking the repository structure'
        : 'Checking the repository and reading ' . ($c['read_data'] === 'all' ? 'all data' : $c['subset'] . ' of the data'));
    $summary = null;
    list($code, $err) = rb_restic_unlocking($r, $args, function ($j) use ($r, &$summary) {
        $type = $j['message_type'] ?? '';
        if ($type === 'summary') {
            unset($j['message_type']);
            $summary = $j;
        } elseif ($type === 'status' && isset($j['percent_done'])) {
            $r->op['progress'] = array('percent' => (float)$j['percent_done'], 'at' => time());
        }
    });
    if ($r->cancelled() || $code === 130) {
        return $r->finish('cancelled', 'Stopped.');
    }
    $problems = (int)($summary['num_errors'] ?? 0);
    if ($code !== 0 || $problems > 0) {
        $msg = $problems > 0 ? "The check found $problems problem(s)" : rb_restic_error($err, $code);
        if (!empty($summary['suggest_repair_index'])) {
            $msg .= ' - restic suggests "repair index"';
        } elseif (!empty($summary['suggest_prune'])) {
            $msg .= ' - restic suggests running prune';
        }
        return $r->finish('error', $msg . '. See the log.', $summary);
    }
    return $r->finish('success', '', $summary);
}

function rb_glob_escape($name)
{
    return addcslashes($name, '\\*?[]');
}

function rb_run_restore(RbRun $r, array $req)
{
    $snap = (string)$req['snapshot'];
    $items = $req['items'];
    $target = $req['target'];
    $overwrite = $req['overwrite'];
    $sum = array('files_restored' => 0, 'files_skipped' => 0, 'files_deleted' => 0,
                 'bytes_restored' => 0, 'bytes_skipped' => 0, 'items' => count($items));
    foreach ($items as $i => $path) {
        if ($r->cancelled()) {
            return $r->finish('cancelled', 'Stopped. What was restored until then stays.', $sum);
        }
        $whole = $path === '/';
        $parent = $whole ? '/' : dirname($path);
        $dest = $target === 'original' ? $parent : $req['target_path'];
        if (!rb_mkdir_like_parent($dest)) {
            return $r->finish('error', "Cannot create $dest", $sum);
        }
        $args = array('restore', '--json', $whole ? $snap : "$snap:$parent", '--target', $dest,
                      '--overwrite', $overwrite, '--retry-lock', '10m');
        if (!$whole) {
            array_push($args, '--include', '/' . rb_glob_escape(basename($path)));
        }
        if (!empty($req['delete'])) {
            $args[] = '--delete';
        }
        $r->phase(count($items) > 1 ? 'Restoring ' . ($i + 1) . ' of ' . count($items) . ": $path" : "Restoring $path");
        $result = null;
        list($code, $err) = $r->restic($args, function ($j) use ($r, &$result) {
            $type = $j['message_type'] ?? '';
            if ($type === 'status') {
                $r->op['progress'] = array(
                    'percent'     => (float)($j['percent_done'] ?? 0),
                    'files_done'  => (int)($j['files_restored'] ?? 0) + (int)($j['files_skipped'] ?? 0),
                    'total_files' => (int)($j['total_files'] ?? 0),
                    'bytes_done'  => (int)($j['bytes_restored'] ?? 0) + (int)($j['bytes_skipped'] ?? 0),
                    'total_bytes' => (int)($j['total_bytes'] ?? 0),
                    'at'          => time(),
                );
            } elseif ($type === 'summary') {
                $result = $j;
            }
        });
        if ($r->cancelled() || $code === 130) {
            return $r->finish('cancelled', 'Stopped. What was restored until then stays.', $sum);
        }
        if ($code !== 0) {
            return $r->finish('error', rb_restic_error($err, $code), $sum);
        }
        foreach (array('files_restored', 'files_skipped', 'files_deleted', 'bytes_restored', 'bytes_skipped') as $k) {
            $sum[$k] += (int)($result[$k] ?? 0);
        }
    }
    $sum['target'] = $target === 'original' ? 'original location' : $req['target_path'];
    return $r->finish('success', '', $sum);
}

$argv = $_SERVER['argv'];
$kind = $argv[1] ?? '';
$target = $argv[2] ?? '';
$trigger = in_array($argv[3] ?? '', array('manual', 'schedule'), true) ? $argv[3] : 'manual';

$cfg = rb_config_load();
$job = null;
$request = null;
if ($kind === 'restore') {
    $file = $argv[3] ?? '';
    if (strpos($file, RB_RUN_DIR . '/requests/') !== 0 || !is_file($file)) {
        rb_run_fail_early('No restore request');
    }
    $request = rb_json_read($file);
    @unlink($file);
    $trigger = 'manual';
    if (!is_array($request) || empty($request['items'])) {
        rb_run_fail_early('Malformed restore request');
    }
}
if (in_array($kind, array('backup', 'forget'), true)) {
    $job = rb_job($cfg, $target);
    if (!$job) {
        rb_run_fail_early("No job $target");
    }
    $repo = rb_repo($cfg, $job['repo']);
} elseif (in_array($kind, array('prune', 'check', 'restore'), true)) {
    $repo = rb_repo($cfg, $target);
} else {
    rb_run_fail_early('Usage: run.php backup|forget|prune|check|restore <id> [manual|schedule]');
}
if (!$repo) {
    rb_run_fail_early("No repository for $target");
}

$lock = $kind === 'restore' ? false : rb_repo_lock($repo['id']);
if ($lock === null) {
    if ($job && $kind === 'backup') {
        rb_state_update($cfg['settings'], function (&$state) use ($job) {
            $state['jobs'][$job['id']]['queued'] = time();
        });
    }
    exit(75);
}
$r = new RbRun($kind, $cfg, $repo, $job, $trigger, $lock);
$r->write(true);
if ($kind !== 'restore') {
    rb_browse_stop($repo['id']);
}
$problem = rb_preflight($r);
if ($problem !== '') {
    $r->finish('error', $problem);
    exit(1);
}

switch ($kind) {
    case 'backup':
        rb_run_backup($r);
        break;
    case 'forget':
        rb_run_forget($r);
        break;
    case 'prune':
        rb_run_prune($r);
        break;
    case 'check':
        rb_run_check($r);
        break;
    case 'restore':
        rb_run_restore($r, $request);
        break;
    default:
        $r->finish('error', "$kind is not implemented yet");
}
exit($r->op['status'] === 'error' ? 1 : 0);
