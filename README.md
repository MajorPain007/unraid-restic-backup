# Restic Backup for Unraid

An Unraid plugin that makes backups with [restic](https://restic.net):
encrypted, deduplicated snapshots of your shares, appdata or flash drive,
kept on another disk, on a server reached over SSH, or on a restic REST
server - set up, scheduled and restored from the Unraid web interface.

## Features

| | |
|---|---|
| **Repositories** | A folder on a local disk or pool, an SFTP server (another Unraid, a NAS, a Hetzner Storage Box, any Linux server), or a REST server. SFTP logs in with an SSH key the plugin makes; no passwords are stored for it. |
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
