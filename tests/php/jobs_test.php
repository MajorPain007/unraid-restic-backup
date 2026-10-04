<?php
require __DIR__ . '/harness.php';
require RB_SRC . '/include/lib/jobs.php';

function job(array $sources, array $extra = array())
{
    return $extra + array('id' => 'j00000001', 'tags' => array(), 'sources' => $sources, 'excludes' => array(),
        'iexcludes' => array(), 'exclude_caches' => false, 'exclude_larger_than' => '', 'one_file_system' => false,
        'advanced' => array('host' => 'Tower', 'read_concurrency' => 0, 'skip_if_unchanged' => false, 'extra_flags' => array()));
}

function repo($type = 'sftp', $path = '')
{
    return array('type' => $type, 'local' => array('path' => $path));
}

$settings = array('data_dir' => '/mnt/user/appdata/restic.backup', 'host' => '');

t_group("The plugin keeps its own files out of a backup");

t_case('written as the source is: the pool path of a data folder set on the share', function () use ($settings) {
    t_eq(array('/mnt/cache/appdata/restic.backup/cache', '/mnt/cache/appdata/restic.backup/tmp'),
         rb_own_excludes(job(array('/boot', '/mnt/user/Daten', '/mnt/cache/appdata')), repo(), $settings));
});

t_case('and the share path of a data folder set on the pool', function () {
    t_eq(array('/mnt/user/appdata/restic.backup/cache', '/mnt/user/appdata/restic.backup/tmp'),
         rb_own_excludes(job(array('/mnt/user/appdata')), repo(), array('data_dir' => '/mnt/cache/appdata/restic.backup')));
});

t_case('once for each way a source writes it, and nothing for sources it is not in', function () use ($settings) {
    t_eq(array('/mnt/user/appdata/restic.backup/cache', '/mnt/cache/appdata/restic.backup/cache',
               '/mnt/user/appdata/restic.backup/tmp', '/mnt/cache/appdata/restic.backup/tmp'),
         rb_own_excludes(job(array('/mnt/user/appdata', '/mnt/cache/appdata', '/mnt/user/Daten')), repo(), $settings));
    t_eq(array(), rb_own_excludes(job(array('/mnt/user/Daten', '/mnt/cache/appdata/plex', '/mnt/disks/usb')), repo(), $settings));
});

t_case('a local repository inside a source, through its share too', function () use ($settings) {
    t_eq(array('/mnt/disk3/Backup/restic'),
         rb_own_excludes(job(array('/mnt/disk3/Backup')), repo('local', '/mnt/user/Backup/restic'), $settings));
    t_eq(array(), rb_own_excludes(job(array('/mnt/user/Daten')), repo('local', '/mnt/disks/usb/restic'), $settings));
    t_eq(array('/mnt/disks/usb/restic'),
         rb_own_excludes(job(array('/mnt/disks/usb')), repo('local', '/mnt/disks/usb/restic'), $settings));
});

t_case('the backup gets them in its exclude file', function () use ($settings) {
    $tmp = t_tmpdir() . '/op';
    $args = rb_backup_args(job(array('/mnt/cache/appdata'), array('excludes' => array('*.tmp'))), repo(), $settings, $tmp);
    t_true(in_array("$tmp.exclude", $args, true), 'the exclude file is passed');
    t_eq("*.tmp\n.zfs/snapshot\n/mnt/cache/appdata/restic.backup/cache\n/mnt/cache/appdata/restic.backup/tmp\n",
         file_get_contents("$tmp.exclude"));
});

t_done();
