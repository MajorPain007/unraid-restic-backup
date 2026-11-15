# Restic Backup for Unraid

An Unraid plugin that makes backups with [restic](https://restic.net):
encrypted, deduplicated snapshots of your shares, appdata or flash drive,
kept on another disk, on a server reached over SSH, or on a restic REST
server - set up, scheduled and restored from the Unraid web interface.

## Features

| | |
|---|---|
| **Repositories** | A folder on a local disk or pool, an SFTP server (another Unraid, a NAS, a Hetzner Storage Box, any Linux server), or a REST server. SFTP logs in with an SSH key the plugin makes; no passwords are stored for it. Repository passwords and keys can be kept encrypted on the flash drive, unlocked with the array or a master password. |
| **Jobs** | Folders, exclusions, a schedule and a retention policy per job. A run missed while the server was off happens when it is back. |
| **Data in use** | Per job: back up as it is, from a ZFS snapshot, from a ZFS snapshot with the containers and VMs using the data stopped for a moment, or with them stopped for the whole backup. |
| **Restore** | Browse every snapshot, restore files and folders in place or into another folder, download them, search across snapshots, compare two. |
| **Maintenance** | Prune and check on their own schedules, Unraid notifications, an optional Healthchecks ping, commands before and after a backup. |

## Installation

**Plugins → Install Plugin**, paste:

```
https://raw.githubusercontent.com/MajorPain007/unraid-restic-backup/main/src/restic.backup.plg
```

Requires Unraid 6.12 or newer. restic 0.19.1 is downloaded from its GitHub
release during installation and checked against its published SHA-256.

The plugin appears under **Settings → Utilities → Restic Backup**; a **Backup**
entry next to Docker and VMs can be switched on in its settings.

Keep each repository's password somewhere other than the server - the plugin
shows it when the repository is created. Without it, the backups cannot be
read.

## Data and privacy

Settings, repository passwords, the SSH keys for SFTP repositories and their
known hosts are kept in `/boot/config/plugins/restic.backup/` on the flash
drive. Passwords and keys are stored unencrypted there unless you choose
encryption in the settings. History, run logs and restic's cache are kept in
the data folder you set (on a pool or share); status and running operations
live in `/tmp/restic.backup/` (RAM). The plugin contacts only your
repositories, the restic download on GitHub during installation, and a
Healthchecks URL if you set one.

**Uninstalling** stops running backups and removes the plugin, restic and its
package from the flash drive. Settings, passwords and keys stay in
`/boot/config/plugins/restic.backup/`, and the data folder stays, so a
reinstall picks them up; delete them yourself if you no longer need the
backups. Repositories are never touched.
