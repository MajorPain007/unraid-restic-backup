<?php

require_once dirname(__DIR__, 2) . '/src/include/lib/config.php';

$cmd = $argv[1] ?? '';
$cfg = rb_config_load();
$errors = array();
switch ($cmd) {
    case 'settings':
        $cfg['settings'] = rb_clean_settings(json_decode($argv[2], true), $errors);
        break;
    case 'repo':
        $repo = rb_clean_repo(json_decode($argv[2], true), $errors);
        $i = rb_find($cfg['repos'], $repo['id']);
        if ($i === null) {
            $cfg['repos'][] = $repo;
        } else {
            $cfg['repos'][$i] = $repo;
        }
        break;
    case 'job':
        $job = rb_clean_job(json_decode($argv[2], true), array_column($cfg['repos'], 'id'), $errors);
        $errors = array_merge($errors, rb_check_job_against_repos($job, $cfg));
        $i = rb_find($cfg['jobs'], $job['id']);
        if ($i === null) {
            $cfg['jobs'][] = $job;
        } else {
            $cfg['jobs'][$i] = $job;
        }
        break;
    case 'secret':
        rb_secret_set($argv[2], $argv[3], $argv[4]);
        exit(0);
    case 'get':
        echo json_encode($cfg, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES), "\n";
        exit(0);
    default:
        fwrite(STDERR, "unknown command\n");
        exit(2);
}
if ($errors) {
    fwrite(STDERR, implode("\n", $errors) . "\n");
    exit(1);
}
rb_config_save($cfg);
